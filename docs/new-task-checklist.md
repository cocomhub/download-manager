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

> 脚手架脚本会从 `task/TEMPLATE/task.go.tmpl` + `adapter.go.tmpl` 自动生成 Go 骨架
> （`task/<type>/task.go` + `task/<type>/adapter.go`），并在 UI 目录生成插件。
> 下面的 1-2 步是骨架生成后需要补全的站点逻辑。

### 1. 创建 Go 后端（骨架 + 站点逻辑）

脚手架生成 `task/<type>/task.go`（注册工厂 + `NewTask` + 组装 PagingScanner/SiteAdapter）和
`task/<type>/adapter.go`（SiteAdapter 骨架）。

#### 1.1 task.go — 补全站点配置读取

- 在 `NewTask` 中从 `cfg.Extra` 读取站点特定参数（keyword、user_id 等）
- 若站点无需分页列表（直连型，参考 `task/urllist/`），可覆盖 `Scrape` 返回 nil

#### 1.2 adapter.go — 实现分页生命周期 + 对象构建

`task.SiteAdapter` 接口（`task/siteadapter.go`）由 `PagingScanner`（`task/scanner.go`）消费，
驱动「分页抓取 → 条目解析 → 去重 → 对象构建 → 持久化」管线：

| 方法 | 职责 |
|------|------|
| `BuildPageURL(page)` | 构造第 page 页 URL（1 起） |
| `RunScraper(url)` | 抓取页面内容（HTML/JSON）；API 型站点可把 page 编码进 URL 在此解码（参考 vikacg） |
| `ParseTotalPages(html)` | 提取总页数；未知返回 ≤0（PagingScanner 用空页熔断停止） |
| `ParsePage(html)` | 解析条目（站点私有切片类型） |
| `ItemsToURLs(items)` | 提取去重 URL，长度与条目数一致 |
| `BuildObject(items, index)` | 构建 DownloadObject；**缓存优先**：先查 `BaseTask.GetCachedObject(url)` 复用已存在对象 |

组装（脚手架已生成）：

```go
adapter := &{{TYPE}}Adapter{t: t}
scanner := task.NewPagingScanner(bt, adapter)
bt.SetScanner(scanner)
bt.SetSelf(t)
```

#### 1.3 详情解析（可选）

有详情页解析需求时实现 `ResolveObject`：解析详情页填充 `Metadata`（title/date/tags）与
`Extra`（files/images）。参考 `task/tktube/`（HTML 详情）、`task/vikacg/`（cache + scrapeAndBuild）。

参考现有实现：
- `task/urllist/` — 简单 URL 列表下载（无分页/详情）
- `task/tktube/` — 视频网站（分页 + HTML 详情）
- `task/hanime/` — 动漫网站（分页 + HTML 详情）
- `task/vikacg/` — 图片网站（API 分页 + cache 优先详情）

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