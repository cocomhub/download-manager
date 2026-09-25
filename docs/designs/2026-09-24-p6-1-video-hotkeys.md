# P6-1 视频快捷键 + 图片全屏设计（2026-09-24）

## 背景 / 目标

- 现状：videoPlayer.js 已有 Space/K 播放暂停、←/→ J/L 快进快退（skipInterval 10s）、↑/↓ 音量、F 全屏、M 静音、Esc 关闭
- 缺口：**倍速快捷键**（[ / ] 或 < / >）；**图片全屏**（baseViewer 无 lightbox/zoom）
- 目标：视频支持倍速调节快捷键 + 图片全屏观看（用户需求）

## 组件与接口

- `web/static/app/ui/videoPlayer.js`（改）：
  - `handleKeydown` 增加 `[` / `]`（或 `,` / `.`）倍速步进（0.25 步进，范围 0.25-2.0）
  - `KeyS`（或 `>` / `<`）额外：`]` = +0.25，`[` = -0.25；显示当前倍速 OSD
  - 显示倍速值在控制条（已有 playbackRate state）
- `web/static/app/taskui/baseViewer.js`（改）：图片查看器加点击放大/全屏按钮（lightbox 模式）
- `web/static/index.html`（改）：图片查看器区域加全屏按钮

## 数据流

1. 用户按 `]` → handleKeydown 捕获 → setSpeed(state, rate+0.25) → video.playbackRate 更新
2. 用户点图片全屏按钮 → 图片元素 requestFullscreen

## 错误处理

- 倍速范围钳制（0.25-2.0），越界忽略
- 无视频播放时不响应快捷键（已有 currentVideo 检查）

## 测试 + 变异点

- node --test：`videoPlayer.test.js` 纯函数测试（setSpeed 钳制/步进）
- Playwright e2e：`video.spec.ts` 快捷键操作视频（加载 fixture 视频 → 按 ] → 倍速变化）
- 图片全屏：Playwright 点击全屏按钮 → 图片全屏
- **变异点**：删倍速分支 → e2e 红；删全屏按钮 → e2e 红

## 片划分

- P1：视频倍速快捷键 + OSD
- P2：图片全屏 lightbox
- P3：快捷键提示 UI（控制条显示）

## 风险与零回归

- 现有快捷键不冲突（`[`/`]` 未被占用）
- 无视频时快捷键无操作（零回归）
