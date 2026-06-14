# Playwright Codegen 使用指南（AI 交互式测试录制）

Playwright Codegen 可录制用户操作并自动生成测试代码。AI 可通过此能力将自然语言操作步骤转化为自动化测试。

## 启动方式

```bash
# 1. 确保 Go test server 在运行
cd cmd/playwright-server && go build -o playwright-server.exe .
cd test/playwright && npx playwright codegen http://localhost:19199 \
  --target playwright-test \
  --test-id-attribute data-testid \
  -o specs/codegen-captured.spec.ts
```

## AI 操作流程

1. 启动 codegen → 浏览器窗口打开
2. AI 通过 **Chrome DevTools MCP** 操作页面（导航、点击、输入）
3. Codegen 窗口实时录制所有操作生成测试代码
4. 保存到 `specs/` 目录，纳入 CI

## 已在 Chrome DevTools MCP 中配置的工具

- `chrome-devtools-mcp:chrome-devtools` — 浏览器交互自动化
- `chrome-devtools-mcp:troubleshooting` — 连接调试
- `chrome-devtools-mcp:debug-optimize-lcp` — 性能优化
- `chrome-devtools-mcp:memory-leak-debugging` — 内存泄漏检测
- `chrome-devtools-mcp:a11y-debugging` — 无障碍审计

## data-testid 已注入的关键锚点

| testid | 用途 |
|---|---|
| `sidebar` | 侧边栏容器 |
| `task-{id}` | 每个任务项 |
| `object-{url}` | 每个对象卡片/行 |
| `view-mode-{mode}` | 视图切换按钮 |
| `btn-cancel-{url}` / `btn-undo-{url}` | 操作按钮 |
| `search-input` | 搜索输入框 |
| `status-indicator` | 连接状态 |
