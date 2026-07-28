# 新任务类型开发指南

## 概览

在下载管理器中添加新任务类型需要以下步骤：

1. **Go 后端** — 实现 `core.Task` 接口
2. **Go UI 注册** — `task/<type>/ui/ui.go`
3. **JS UI 插件** — `task/<type>/ui/assets/viewer.js`
4. **配置** — 在 `config.yaml` 中添加任务配置

## 快速开始

使用脚手架脚本自动生成模板：

```bash
./scripts/new-task-type.sh
```

交互式提示输入参数，或直接传参：

```bash
TYPE=mytype LABEL="My Type" HAS_FORM=y HAS_VIEWER=y VIEWER_TYPE=video ./scripts/new-task-type.sh
```

## 手动创建步骤

### 1. 创建 Go 后端

在 `task/<type>/` 下创建 Go 包，实现 `core.Task` 接口。

参考现有实现：
- `task/urllist/` — 简单 URL 列表下载
- `task/tktube/` — 视频网站下载
- `task/hanime/` — 动漫网站下载
- `task/vikacg/` — 图片网站下载

### 2. 创建 UI 注册

```
task/<type>/ui/
├── ui.go            # Go 注册代码
└── assets/
    └── viewer.js    # JS UI 插件
```

模板文件位置：`task/TEMPLATE/`

### 3. 编写 JS UI 插件

#### 3.1 共享模块

| 模块 | 命名空间 | 用途 |
|------|---------|------|
| `data.js` | `TaskUI.Data` | 通用数据访问器（getTitle、getTags、getVideoUrl 等） |
| `dom.js` | `TaskUI.Dom` | DOM 构建辅助（createTagChips、createButton、createLink 等） |
| `modal.js` | `TaskUI.Modal` | Modal 构建器（createOverlay、createPanel、createVideoArea 等） |

#### 3.2 注册方式

```js
TaskUI.register('mytype', {
  type: 'mytype',
  label: 'My Type',
  icon: 'fa-video',           // FontAwesome 图标类
  viewerLabel: '查看',         // 查看器按钮文字

  // 表单（可选）
  renderForm: TaskUI.defineForm({ fields: [...] }),
  renderMeta: TaskUI.defineMeta({ fields: [...] }),
  collectExtra: function(formData) { ... },

  // 查看器（可选）
  shouldShowViewer: function(obj) { return obj.status === 'completed' },
  onClick: function(obj, helpers) { ... },
  renderViewer: function(h, obj, onClose) { ... },
})
```

#### 3.3 查看器类型选择

| 类型 | 适用场景 | 参考实现 |
|------|---------|---------|
| 视频播放器 | 视频/动画内容 | tktube、hanime |
| 图片画廊 | 图片/漫画内容 | vikacg |
| 纯表单 | 无查看器，仅任务创建 | urllist |

### 4. 配置

在 `config.yaml` 中添加任务配置段：

```yaml
tasks:
  mytype:
    enabled: true
    # 类型特定配置...
```

## 验证清单

- [ ] `go build ./...` 编译通过
- [ ] JS 文件语法正确（无控制台错误）
- [ ] 新建任务弹窗显示扩展表单（如有）
- [ ] 任务详情页显示扩展元数据（如有）
- [ ] 点击 completed 对象打开查看器（如有）
- [ ] 查看器 ESC/backdrop 关闭正常
- [ ] 合集/推荐面板正常显示（如有）
- [ ] `make run` 启动后功能正常