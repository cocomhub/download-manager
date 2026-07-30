# pkg/download 第四轮审查经验总结

> 审查日期：2026-07-30 ~ 2026-07-31
> 审查范围：pkg/download/ 全包（55+ 文件）
> 修复统计：22 项修复 + 4 项最终修复，38 个变更文件

---

## 通用问题模式

### 1. 修复遗漏同一语义问题在不同文件的复现

**现象**：`transport_stdlib.go` 中的 `JoinPath` 转义查询参数已修复，但 `wget.go` 中完全相同的 `JoinPath(u.Host + u.RequestURI())` 模式未被修复。

**根因**：修复代理按文件分配，transport_stdlib.go 和 wget.go 不在同一修复批次中。批次的"连锁影响分析"只检查了**调用链**（函数调用的上下游），未检查**语义等价**（同一 bug 模式在不同文件的复现）。

**防御**：Phase 4 分析阶段增加"语义等价问题搜索"：`grep -rn "JoinPath"` 找到所有同类调用，一次性修复。

### 2. 安全问题误报 — 缺少防护验证

**现象**：WgetExtractor 的 `exec.CommandContext` 被多轮审查反复标注为"命令行注入风险"，但实际代码中已有 `validateWgetRequest` 覆盖了所有输入路径的检查。

**根因**：审查子代理看到 `exec.Command` 就触发 alert，不去读 `validateWgetRequest` 的检查范围。

**防御**：更新后的 skill 提示词已强制要求"先检查现有防护再判断"，本轮已生效 —— extractor 子包报告明确标注了"已防护，误报"。

### 3. 死测试 — 无有效断言

**现象**：`TestHTTPExtractorCancelNotFound` 只 `t.Logf` 打印结果，不做任何断言。自引入以来从未验证过任何行为。

**防御**：测试审查中已列入 T#1 死测试检测清单，需在 Phase 2 测试审查中严格执行。

### 4. Windows 专属的 `TerminateProcess` 死锁

**现象**：`hls.go` 的 `executeFFmpeg` 中，Windows 上 `Process.Kill` 使用 `TerminateProcess`，不保证 kill 子进程树，导致 stderr pipe 不关闭，`cmd.Wait()` 和 `bufio.Scanner` 互相阻塞形成死锁。

**防御**：任何使用 `exec.Command` + `StderrPipe` + goroutine 读取的模式，都需要在 `cmd.Wait()` 上加超时保护。

### 5. 测试断言过于宽松

**现象**：`bandwidth_test.go` 中 `bw > 0` 的断言，本地回环 1MB 数据至少应有 10+ MB/s，但测试只要求正数。这是"弱断言"变体。

**防御**：测试审查中应检查断言是否真正可失败，对带宽/时间类断言应设置合理下限而非仅 `> 0`。

---

## 代码审查流程改进

### 1. 安全检查前置：验证现有防护

**问题**：多轮审查重复报同一个安全问题（WgetExtractor 注入风险），但实际已修复。

**改进**：Phase 2 审查子代理提示词新增"安全问题必须先检查代码中已有防护措施"规则。本轮验证有效 —— extractor 子包报告明确标注了"已防护，误报"。

### 2. 修复批次的"语义等价"搜索

**问题**：`JoinPath` 转义修复在 transport_stdlib.go 中完成，但 wget.go 中相同模式被遗漏。

**改进**：Phase 4C 修复子代理的"列出所有受影响的位置"步骤，应增加语义等价搜索：
- `grep -rn "问题模式"` 找到所有同类调用
- 无论是否在同一文件，只要使用了相同模式，都一并修复

### 3. 最终审查的回归检测

**问题**：最终审查发现了 wget.go 的 JoinPath 回归，说明修复阶段没有覆盖所有同类模式。

**改进**：Phase 5 最终审查提示词已新增回归检测步骤，后续应强制执行。

### 4. 测试审查的断言强度分级

**问题**：`bw > 0` 这样的弱断言通过了测试审查。

**改进**：在测试审查中增加断言强度分级：
- **强断言**：验证具体值或合理范围（如 `bw > 1024`）
- **弱断言**：仅验证非零/非负（如 `bw > 0`）
- 弱断言应标注为 suggestion，建议增强

---

## 测试最佳实践

### 1. 异步测试超时保护

`hls.go` 的 `executeFFmpeg` 中，`cmd.Wait()` 在 Windows 上可能因 pipe 未关闭而无限阻塞。修复方案：将 `cmd.Wait()` 移入 goroutine，加 5s cancel 超时 + 30s 全局超时兜底。

```go
waitCh := make(chan error, 1)
go func() {
    waitCh <- cmd.Wait()
}()
select {
case err := <-waitCh:
    // 正常完成
case <-dlCtx.Done():
    select {
    case err := <-waitCh:
        // cancel 后正常退出
    case <-time.After(5 * time.Second):
        // 超时兜底
    }
case <-time.After(30 * time.Second):
    // 全局超时
}
```

### 2. 测试资源清理

`t.Cleanup(cancel)` 应在 `context.WithCancel` 后立即注册，确保 `t.Fatal` 等提前终止路径也能释放资源。

### 3. 测试断言增强

- 带宽测试：`bw > 0` → `bw > 1024`（1KB/s 本地回环下限）
- 缓存测试：除验证缓存文件存在外，还应验证 `Select()` 返回值
- Range 请求测试：在 server handler 中记录 `rangeRequested` 标志，结束后验证

---

## API 设计经验

### 1. `RuleSetSelector` hint nil guard

`ruleSetSelector.MatchExtractor` 中 `hint == nil` 的 guard 是必要的（`hint.Extractor = matched.Extractor` 在 hint==nil 时 panic），但注释错误地声称"caller guarantees non-nil"。应改为安全处理：hint==nil 时直接委托给 next selector。

### 2. `MetricRegistry.Snapshot` 锁类型

`Snapshot` 只读遍历 map，应使用 `RLock` 而非 `Lock`，避免不必要地阻塞并发 `Record()`/`Get()`。

### 3. `Request` 文档说明

`Request` 结构体在 `Download` 调用后字段会被修改，文档应明确说明此行为，禁止多 goroutine 复用同一实例。

---

## Changelog

### 修复统计

| 类别 | 数量 | 占比 |
|------|------|------|
| 并发安全 | 3 | 14% |
| 功能性 Bug | 5 | 23% |
| 安全加固 | 3 | 14% |
| API 设计改进 | 4 | 18% |
| 测试修复 | 7 | 32% |
| **总计** | **22** | 100% |

### 关键修复

| 修复 | 影响 | 风险等级 |
|------|------|---------|
| `StdlibTransport` 添加 HTTP 超时（5m） | 防止请求无限挂起 | 🔴 高 |
| `isSafeTargetURL` DNS 超时改用 `net.Resolver.LookupHost` | 防止恶意 DNS 阻塞 | 🔴 高 |
| `ProgressReader.downloaded` 改用 `atomic.Int64` | 消除 data race | 🔴 高 |
| `DomainLimiter.Set` 惊群效应修复 | CPU 峰值问题 | 🟡 中 |
| `hls.go` executeFFmpeg 超时保护 | Windows 死锁修复 | 🟡 中 |
| `wget.go` + `transport_stdlib.go` JoinPath 查询参数转义 | 代理下载 URL 错误 | 🟡 中 |
| `hls.go` 全量 header CR/LF/`-` 过滤 | 安全加固 | 🟡 中 |
| 7 个测试修复 | 断言增强 | 🟢 低 |

### 未修复（已文档化）

| 问题 | 文件 | 原因 |
|------|------|------|
| `M3U8DEngine.Config` 导出字段 | `m3u8d/engine.go` | 依赖包内使用 |
|  `pkg/download` 依赖顶层 `config` 包 | `proxy_selector.go` | 重构范围大 |
| 隧道密钥失败静默降级 | `transport/sproxy.go` | 设计决策 |
| CompositeExtractor 不实现 Canceller | `extractor/composite.go` | 功能未缺失 |
| CompositeExtractor 失败不清理 | `extractor/composite.go` | 调用方负责 |
| TransportResponse.Headers map[string]string | `request.go` | 兼容性 |