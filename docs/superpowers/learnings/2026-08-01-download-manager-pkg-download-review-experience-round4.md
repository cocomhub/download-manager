# 2026-08-01 cocomhub pkg/download 审查修复经验总结（第四轮）

## 概述
- 审查范围：`pkg/download` 全部 58 个 Go 文件（约 9,300 行代码）
- 审查模式：`code-review-repair-cycle` 技能，10 个独立子代理并行审查（6 个代码审查 + 4 个测试审查）
- 发现：2 个功能不可用 + 8 个功能受限 = 共计 10 个问题
- 修复：6 个并行修复批次，对应 6 个 commit，全部经由三轮验证（`go build` + `go test -race` + `golangci-lint run`）
- 分支：`fix/pkg-download-code-review`，累计对 master 有 59 个变更文件 +3,074/-659 行（含前几轮累积）

## 问题统计

| 类型 | 数量 | 说明 |
|------|------|------|
| 功能不可用 | 2 | `forceProxy` 缓存覆盖强制代理语义、`ApplyDomainLimits` 静默忽略域名限制 |
| 功能受限 | 8 | `DomainLimiter` 唤醒 TOCTOU 竞态、sproxy `Host header` 遗漏、`m3u8d` 递归无深度限制、`WgetExtractor`/`CompositeExtractor` 未填充 `Result`、`m3u8d grab.go` 空响应 `Request` 终止整体、`DomainLimiter` 惊群、`checkBandwidth` 无 HTTP 状态码校验、wget 软链接路径清理 |
| 修复批次 | 6 | 所有问题已修复 |

## 本轮 6 个修复批次

| 批次 | Commit | 文件 | 描述 |
|------|--------|------|------|
| Batch 1 | `f61db56` | `proxy_selector.go`, `proxy_selector_test.go` | `forceProxy` 忽略直连缓存 |
| Batch 2 | `94797fb` | `downloader/adapter.go` | `ApplyDomainLimits` 非 `StdlibTransport` 时打 warning 日志 |
| Batch 3 | `6cd4a60` | `domainlimiter.go`, `domainlimiter_test.go` | `DomainLimiter` atomic slot 重构消除 ctx 竞态 |
| Batch 4 | `a1b5207` | `extractor/composite.go`, `extractor/wget.go` | wget/composite 填充 `req.Result` |
| Batch 5 | `af45b36` | `transport/sproxy.go`, `transport/sproxy_test.go` | 修复 `Host header` + 启用隧道测试 |
| Batch 6 | `654ac7b` | `m3u8d/engine.go`, `m3u8d/grab.go` | 递归深度限制 + 空响应 `Request` 不终止批量 |

## 通用问题模式

### 1. 缓存导致配置失效
`forceProxy=true` 时缓存中的 "direct" 决策被信任并返回直连，完全跳过强制代理逻辑。
**教训**：缓存不能覆盖配置语义。缓存应存储"探测结果"，不应存储"配置意图"。读取缓存后应检查当前配置是否使缓存失效。

### 2. 静默忽略的配置路径
`ApplyDomainLimits` 在 transport 不是 `StdlibTransport` 时静默丢弃域名限制，没有任何日志。
**教训**：配置被绕过多半比错误更严重。任何静默忽略配置的路径，至少应该打 warning 日志告知用户。配置生效是可观测性的一部分。

### 3. 唤醒机制中的 TOCTOU 竞态
`DomainLimiter.Set()` 唤醒等待者后，被唤醒者获取 slot 前检查 ctx 是否取消，如果已取消则 passSlot 给下一个。但 passSlot 多唤醒了一个 `Set()` 未计在内的等待者。
**教训**：栈上 slot 传递（channel 唤醒后自动获得资源）与 context 取消天然冲突。被唤醒后不应自动获得资源，而应重新检查条件（重试循环 + atomic CAS）。用 channel 做通知，用 atomic 做计数。

### 4. Host header 遗漏的代理兼容性
sproxy HTTP 代理模式下没有设置 `Host header`，与 `StdlibTransport` 不一致。如果目标服务器是虚拟主机，会返回错误内容。
**教训**：不同 transport 实现应共享同一个"代理兼容性基线"。有多实现时，每个实现应用性检查清单（Host header、超时配置、连接池等）确保行为一致。

### 5. 递归处理缺少深度限制
`m3u8d` 的 `parseM3U8` 递归处理时 level 已传入但没有上限检查，恶意 m3u8 可导致栈溢出。
**教训**：任何递归处理都应加深度限制。level 参数传入但不检查 == 假安全感。始终在入口处检查 `level > maxDepth`。

### 6. 批量操作中的单一异常不应终止整体
`m3u8d/grab.go` 的 `recordFailure` 在 `resp.Request == nil` 时返回 error 终止整个批量下载。
**教训**：批量操作中单一条目的异常应跳过而非终止。单个分片异常需要权衡：在其不威胁整体结果时应跳过，记录日志后继续。

### 7. 提取器完成时未填充 Result
`WgetExtractor` 和 `CompositeExtractor` 完成下载后没有设置 `req.Result`，依赖 Result 的调用方得到 nil。
**教训**：Result 是 extractor-contract 的一部分。提取器不管是否 deprecated，如果 Result 是 Request 的公开字段，所有提取器都应该填充它。supplier 端的一致性比 consumer 端的容错更重要。

### 8. 从测试框架获取更多上下文
本轮审查中发现的部分"功能受限"问题（如带宽零值、缺少 wget 预检查等）已在第 3 轮中修复，但审查代理仍报告。原因在于 worktree 隔离的审查代理以 master 为 base ref，看不到分支上的修复。
**教训**：审查代理在检查"是否已修复"时应获取更丰富的上下文（如 `git log --oneline base..HEAD`），或审查 target 分支而非 base 分支。`code-review-repair-cycle` 的 Phase 2 提示词已建议在 base ref 策略说明中补充此点。

## 流程改进

### 1. Worktree 自动清理增强
本轮 10 个审查代理 + 6 个修复代理全部使用 worktree 隔离。修复代理的 worktree 在任务完成时自动清理，但 commit 不会自动 cherry-pick 回主分支。需要手动 `git cherry-pick`。
**建议**：修复子代理完成时自动 cherry-pick commit 到主分支并清理 worktree。目前 worktree 代理的 commit 只在 worktree 分支上，需要主侧显式 cherry-pick。

### 2. 冲突处理流程增强
多个 worktree 修复同一文件时（如 `domainlimiter.go` 和 `transport_stdlib.go` 被 Batch 3 原子重构修改），cherry-pick 会产生合并冲突。
**建议**：修复规划时识别文件重叠，确保同一文件不在多个并行批次中。如果无法避免，应串行处理这些批次。

### 3. 手动 cherry-pick 的 worktree 清理
在手动 `git cherry-pick` 过程中遇到合并冲突时，解析冲突需要读文件、编辑、add、commit，完成后仓库处于 cherry-pick 序列中。此时 `git status` 显示的是待提交状态而非工作区干净。
**建议**：手动 cherry-pick 完成后统一提交，避免部分 cherry-pick 改变 git 状态。或使用 `git cherry-pick --abort` 全量放弃后重来。

### 4. 审查上下文缺失（第 3 轮修复在第 4 轮重复报告）
由于 `code-review-repair-cycle` 子代理以 master 为 base ref，第 3 轮在第 4 轮前提交的修复在第 4 轮审查中仍然被报告为"未修复"。这一方面是保守设计（宁可重复报告也不漏报），但浪费了审查者时间。
**建议**：允许子代理传入 `target ref`（如 `fix/pkg-download-code-review`），审查该 ref 相对于 base 的实际 diff，而非只看 base ref 的当前状态。

## 最佳实践

### Session 级别最佳实践
- 审查代理用 worktree 隔离，修复代理也用 worktree 隔离——可以并行执行，互不干扰
- 修复前 `git config core.autocrlf input` —— 避免 CRLF 污染 git blame 历史
- 修复后 `go build ./...; go test -race ./受影响包/...; golangci-lint run` —— 三层验证
- 精确文件提交：`git add <文件1> <文件2>` 而非 `git add -A`，避免混入无关变更

### API 级别最佳实践
- Transport 接口实现者应用性检查清单：Host header、DomainLimiter 集成、超时配置、连接池管理
- DomainLimiter 的唤醒机制用 atomic CAS + 重试循环替代 channel slot 传递，消除 TOCTOU
- 缓存读后检查配置语义是否使缓存失效（`forceProxy` + "direct" 缓存）
- 批量操作异常应跳过并记录日志而非终止整体
- 带 level 参数的递归函数必须在入口处检查深度上限
- Extractor 填充 Result 是契约的一部分，不应有例外

## 后续建议
- 补充 `forceProxy` 缓存跳过单元测试（先写 "direct" 缓存 → `forceProxy=true` 调用 `Select` → 验证返回代理 URL）
- 如果需要在 `SproxyTransport` 上支持域名限制，推进 Transport 接口新增 `SetDomainLimits` 方法
- 定期检查本轮的 10 个问题是否在其他包中存在类似模式（全局搜索同类代码）
- 记录本次 `code-review-repair-cycle` 的 base ref 上下文缺失问题到 skill 改进清单
