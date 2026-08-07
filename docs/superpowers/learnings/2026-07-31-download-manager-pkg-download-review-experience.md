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

---

## 第二轮：独立功能可用性审查 + 全量修复（2026-07-31 19:00~23:55）

> 审查范围：pkg/download/ 全部 29 个实现文件 + 29 个测试文件
> 修复统计：三轮修复，15+10+13 个文件，共约 +700/-100 行

### 修复统计

| 类别 | 数量 | 说明 |
|------|------|------|
| 🔴 生产 Bug 修复 | 10 | `--` 分隔符、30s 超时可配置、Windows 路径遍历、`WithRuleSet(nil)` panic、`Release` slot 泄漏、`CheckBandwidth` 状态码缺失、`ErrNoDefaultDownloader` 清理、`DefaultProbe*` 清理、`LogKeyError,nil` 误导、`os.Remove` 静默错误 |
| 🟢 新增测试 | 12 | 取消重试、符号链接路径安全、白名单路径解析、ProgressReader 并发安全、代理全部不可达、`*` 通配匹配、RuleSet 并发安全、带宽失败路径、取消后重试 |
| 🔧 防御性修复 | 2 | 代理模式 `Host` header、`TestHookRetrySleep` 测试钩子 |
| 📝 文档/注释修正 | 2 | `download_test.go` 注释更新、日志信息修复 |

### 通用问题模式（本轮新增）

#### 6. 并行 agent 文件冲突

多个并行修复 agent 修改同一文件时，后完成的 agent 会覆盖前一个的修改。

**案例**：`transport_stdlib.go` 被 3 个并行 agent 同时修改（Host header + 废弃常量 + 编译错误修复），导致部分修复被覆盖出现编译错误。

**防御**：并行修复必须严格按文件隔离分组，确保每个文件只出现在一个批次中。修复完成后统一验证所有文件的完整性。

#### 7. 测试钩子模式

需要精确同步测试边界条件时，在生产代码中添加测试钩子：

```go
TestHookRetrySleep func() // 仅测试使用

// 生产代码中调用
if hook := e.TestHookRetrySleep; hook != nil {
    hook()
}
```

测试钩子通过 `sync.Once` 确保只触发一次，避免并发问题。

#### 8. 废弃代码清理

**案例**：`ErrNoDefaultDownloader` 和 `DefaultProbeTimeout`/`DefaultProbeBytes` 标记为 `Deprecated` 但未实际删除，作为文档残留存在。

**教训**：标记为 `Deprecated` 的常量/变量应在下一个版本中清理，不留残留。清理时使用 `grep` 确认无引用。

#### 9. 多个 agent 的编译错误累积

多个并行 agent 各自修复了生产代码，但每个 agent 只验证了自己的子包，导致其他子包的编译错误未被发现。

**案例**：`m3u8d` 子包在 `Option` 修复 agent 运行全量测试时出现 `undefined: path` 编译错误，因为 `path` import 被移除但 `path.Clean` 调用还没被替换。

**教训**：每个修复 agent 提交前应运行 `go build ./...` 而非仅 `go build ./<affected_packages>/...`。

### 测试最佳实践（本轮新增）

#### 4. 时序依赖消除

| 模式 | 推荐 | 不推荐 |
|------|------|--------|
| 异步同步 | 信号驱动（channel close + `sync.Once`） | `time.Sleep(500ms)` 固定等待 |
| 阻塞验证 | `time.Sleep` + `time.After` 超时兜底 | 仅 `time.Sleep` 无超时 |
| 取消验证 | 轮询 `MustEventually` + channel 检查 | 依赖 `Cancel()` 返回值 |

#### 5. 断言强度分级

| 级别 | 描述 | 示例 |
|------|------|------|
| 🟢 强 | 可验证精确值 | `finalProgress != 100.0` |
| 🟡 中 | 可验证存在性 | `strings.Contains(err.Error(), "already downloading")` |
| 🔴 弱 | 避免永真断言 | 不要用 `bw > 0`，改为 `bw > 1024` |

### 本轮关键修复

| 修复 | 文件 | 风险等级 |
|------|------|---------|
| `buildFFmpegArgs` SavePath 前加 `--` 分隔符 | `extractor/hls.go:181` | 🔴 高 |
| 30s 硬编码超时改为可配置（默认 5min） | `extractor/hls.go:261` | 🔴 高 |
| Windows 路径遍历 `path.Clean` → `filepath.Clean` | `m3u8d/engine.go:320` | 🔴 高 |
| `WithRuleSet(nil)` 增加 nil 保护 | `option.go:75` | 🔴 高 |
| `DomainLimiter.Release` URL 解析失败直接 return | `domainlimiter.go:138` | 🔴 高 |
| `CheckBandwidth` 增加 HTTP 状态码校验 | `bandwidth.go:35` | 🔴 中 |
| 代理模式显式设置 `hreq.Host = targetHost` | `transport_stdlib.go:98` | 🟡 中 |
| 测试 `time.Sleep` → `TestHookRetrySleep` 信号驱动 | `http_extractor_retry_cancel_test.go` | 🟡 中 |
| 新增取消后重试测试 | `http_extractor_cancel_test.go` | 🟢 低 |
| 新增符号链接 + 白名单路径解析测试 | `fs_test.go` | 🟢 低 |
| 新增 ProgressReader 并发安全测试 | `progress_test.go` | 🟢 低 |
| 新增代理全部不可达 + `*` 通配 + 并发安全测试 | `proxy_selector_test.go`, `rule_test.go` | 🟢 低 |

### 残留问题（建议改进项）

1. **缺少 `ErrAlreadyDownloading` sentinel error** — 当前用 `fmt.Errorf("already downloading: %s", req.URL)` 动态构造，测试用 `strings.Contains` 检测，建议定义 sentinel 便于 `errors.Is`
2. **`hls.go` 5s/3s grace period 硬编码** — cancel/timeout 场景的 goroutine 清理等待时间，当前值合理但极端场景下可能不够
3. **`CompositeExtractor`/`WgetExtractor`/`M3U8DEngine` 标记为 Deprecated** — 保留代码但标记为已废弃，未来可考虑清理