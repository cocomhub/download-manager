# Errors

Command failures and integration errors.

---

## [ERR-20260614-001] PowerShell BOM 导致全文件变更

**Logged**: 2026-06-14T17:00:00Z
**Priority**: medium
**Status**: resolved
**Area**: config

### Summary
PowerShell 的 `Set-Content` 和 `Out-File` 默认使用 UTF-16 LE + BOM 编码，写入 Go 源码文件后 git 显示全文件变更。

### Error
```
diff --git a/manager/download.go b/manager/download.go
-// Copyright 2026 The Cocomhub Authors. All rights reserved.
+// Copyright 2026 The Cocomhub Authors. All rights reserved.
```

### Context
- 使用 PowerShell 的 `Set-Content -Path file.go -Value $content` 写入 UTF-8 但实际输出 UTF-16
- 使用 `Set-Content -Encoding utf8` 才正确输出 UTF-8 without BOM
- 已改用 bash 的 `sed` 进行文本替换（避免编码问题）

### Resolution
- **Resolved**: 2026-06-14T17:30:00Z
- **Notes**: 后续所有文件修改优先使用 bash `sed` 或 `Edit` 工具（工具内部使用 bash），避免 PowerShell 编码陷阱

### Metadata
- Related Files: manager/download.go
- Tags: encoding, BOM, PowerShell, UTF-8

---

## [ERR-20260614-002] go fmt 后 Edit 工具因格式变化失败

**Logged**: 2026-06-14T18:00:00Z
**Priority**: medium
**Status**: resolved
**Area**: config

### Summary
`go fmt` 修改了文件缩进（tab 对齐），导致后续 `Edit` 调用的 `old_string` 与文件实际内容不匹配。

### Error
Edit 工具返回 "String to replace not found in file"。

### Context
- `runtime_mgr.go` 中 `adjustGlobalWorkers` 函数被 go fmt 重新格式化
- `sed -n '34,45p' | cat -A` 显示实际缩进与预期不同
- 修复：先 `go fmt` 再读文件确认内容

### Resolution
- **Resolved**: 2026-06-14T18:30:00Z
- **Notes**: 工作流改为：go fmt → Read 确认 → Edit

### Metadata
- Tags: gofmt, Edit, formatting
