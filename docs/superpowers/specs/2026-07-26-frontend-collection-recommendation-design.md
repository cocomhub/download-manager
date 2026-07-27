# 前端合集面板、推荐面板与播放器导航设计

> 日期：2026-07-26
> 状态：设计稿
> 关联项目：download-manager

## 1. 背景与目标

### 1.1 当前问题

- hanime 查看器右栏显示"播放列表"，点击项在新窗口打开，不是播放器内切换
- tktube 查看器右栏是"关联视频"占位符（"后续实现"）
- 播放器是单视频播放，没有上/下一集导航
- 没有基于标签的推荐功能
- 合集 API 和推荐 API 已后端实现，但前端未调用

### 1.2 目标

1. **合集面板**：在查看器右栏显示合集列表，支持点击切换播放对象，支持折叠
2. **推荐面板**：在查看器右栏（合集面板下方）显示推荐结果，支持标签选择、模式切换、排序切换
3. **播放器导航**：播放器底部添加上/下一集按钮，支持合集内切换
4. **进入时定位当前项**：合集列表自动滚动到当前视频位置

## 2. 整体架构

### 2.1 模块划分

```
web/static/app/
├── api.js                   # 新增 getObject, getCollection 方法
├── videoPlayer.js           # 新增 playPrev/playNext/switchToCollectionItem
├── taskui/
│   ├── collection.js        # 新建：合集面板组件
│   └── recommendation.js    # 新建：推荐面板组件
└── tasks/
    ├── hanime/ui/assets/viewer.js   # 集成合集+推荐面板
    └── tktube/ui/assets/viewer.js   # 集成合集+推荐面板
```

### 2.2 数据流

```
查看器打开
  ↓
同时请求：
  GET /api/objects/{type}/{id}           → 对象详情
  GET /api/objects/{type}/{id}/collection → 合集列表
  GET /api/aggregate?tags=...&exclude_ids=... → 推荐结果
  ↓
渲染：播放器 + 合集面板（右栏上方）+ 推荐面板（右栏下方）
  ↓
用户点击合集项
  ↓
GET /api/objects/{type}/{newId} → 更新播放器
合集面板高亮切换
推荐面板保持不动
```

## 3. API 层扩展

### 3.1 `api.js` 新增方法

```javascript
// 按任务类型+ID 获取单个对象
getObject: function (type, id) {
    return this.get('/api/objects/' + type + '/' + id)
},

// 获取对象所在合集
getCollection: function (type, id) {
    return this.get('/api/objects/' + type + '/' + id + '/collection')
}
```

### 3.2 `api.js` 扩展 `aggregate` 方法

```javascript
aggregate: function (params) {
    var query = '?page=' + (params.page || 1) + '&limit=' + (params.limit || 50)
    if (params.search) query += '&search=' + encodeURIComponent(params.search)
    if (params.sort) query += '&sort=' + params.sort
    if (params.status) query += '&status=' + params.status
    if (params.types) query += '&types=' + params.types
    if (params.groupBy) query += '&group_by=' + params.groupBy
    // 推荐参数
    if (params.tags) query += '&tags=' + encodeURIComponent(params.tags)
    if (params.tagMode) query += '&tag_mode=' + params.tagMode
    if (params.excludeIds) query += '&exclude_ids=' + params.excludeIds
    return this.get('/api/aggregate' + query)
}
```

## 4. 合集面板（`collection.js`）

### 4.1 接口

```javascript
window.CollectionPanel = {
    create: function (options) {
        // options: { type, currentId, onPlayItem }
        // 返回 { element, update, destroy }
    }
}
```

### 4.2 面板结构

```
┌─ 合集 (N) ────────────────── [折叠按钮] ─┐
│  □ EP 1                          00:05:23 │
│  □ EP 2                          00:04:15 │
│  ▶ EP 3 (当前)                   00:05:45 │  ← 高亮+滚动可见
│  □ EP 4                          00:06:01 │
│  ...                                       │
└────────────────────────────────────────────┘
```

### 4.3 行为

- **初始化**：调用 `GET /api/objects/{type}/{id}/collection`
- **高亮当前项**：根据 `currentId` 匹配列表项，添加高亮样式
- **滚动到当前项**：使用 `scrollIntoView` 确保当前项可见，即使排在最后一项
- **点击切换**：调用 `onPlayItem(item)`，由外部更新播放器
- **折叠**：支持折叠/展开，默认展开，折叠后仅显示标题行
- **更新**：`update({ currentId })` 方法切换高亮，不重新请求列表

## 5. 推荐面板（`recommendation.js`）

### 5.1 接口

```javascript
window.RecommendationPanel = {
    create: function (options) {
        // options: { type, currentId, tags, onPlayItem }
        // 返回 { element, update, refresh, destroy }
    }
}
```

### 5.2 面板结构

```
┌─ 推荐 ─────────────────────── [折叠按钮] ─┐
│  标签: [action] [comedy] [drama] [全部]    │
│  模式: [any ▼]  排序: [随机 ▼]             │
│  ┌──────────────────────────────────────┐ │
│  │ 推荐结果 1                   ⭐ 0.85 │ │
│  │ 推荐结果 2                   ⭐ 0.75 │ │
│  │ ...                                  │ │
│  └──────────────────────────────────────┘ │
└────────────────────────────────────────────┘
```

### 5.3 行为

- **初始化**：从 `options.tags` 获取标签列表，默认全选
- **首次请求**：`GET /api/aggregate?types={type}&tags={tags}&tag_mode=any&exclude_ids={id}&sort=random&limit=20`
- **标签选择器**：显示当前对象的所有标签，点击切换选中状态
  - 点击标签 → 切换选中 → 重新请求
  - "全部"按钮 → 全选/取消全选
- **模式切换**：下拉选择 `any`（任一匹配）/ `all`（全部匹配）
- **排序切换**：下拉选择 `random`（随机）/ `date_desc`（最新）/ `tag_match_desc`（最相关）
- **推荐结果**：每个结果展示封面、标题、标签，点击打开该对象的查看器
- **折叠**：支持折叠/展开，默认展开
- **切换合集对象时**：推荐面板保持不动（不重新请求）

## 6. 播放器导航（`videoPlayer.js`）

### 6.1 新增方法

```javascript
// 合集列表（由合集面板设置）
collectionList: [],

// 上/下一集
playPrev: function () {
    var idx = this.collectionList.findIndex(o => o.id === this.currentVideo.id)
    if (idx > 0) {
        this.switchToCollectionItem(this.collectionList[idx - 1])
    }
},

playNext: function () {
    var idx = this.collectionList.findIndex(o => o.id === this.currentVideo.id)
    if (idx < this.collectionList.length - 1) {
        this.switchToCollectionItem(this.collectionList[idx + 1])
    }
},

switchToCollectionItem: function (item) {
    var self = this
    // 1. 获取新对象详情
    AppAPI.getObject(item.type, item.id).then(function (obj) {
        // 2. 更新播放器
        self.currentVideo = obj
        // 3. 更新标题
        // 4. 更新视频源
        // 5. 通知合集面板高亮切换
        if (self.onCollectionSwitch) {
            self.onCollectionSwitch(item.id)
        }
    })
}
```

### 6.2 播放器控制栏

在现有播放器底部控制栏中，进度条与播放按钮之间新增导航按钮：

```
  [<<] [播放/暂停] [>>]    EP 3 / 5    00:05:45 / 00:30:00
```

- 仅在 `collectionList.length > 1` 时显示导航按钮
- 第一集时禁用"上一集"按钮
- 最后一集时禁用"下一集"按钮

## 7. 查看器集成

### 7.1 hanime 查看器

- 移除旧的播放列表渲染（`getPlaylist` 函数保留，供 Standardize 使用）
- 右栏上方渲染合集面板
- 右栏下方渲染推荐面板
- 查看器关闭时销毁面板

### 7.2 tktube 查看器

- 替换"关联视频"占位符
- 右栏上方渲染合集面板
- 右栏下方渲染推荐面板
- 查看器关闭时销毁面板

## 8. 未在此设计中涵盖的内容

- 推荐面板的评分算法（匹配度得分）
- 大数据量下的推荐结果缓存
- 合集面板的拖拽排序
- 移动端适配