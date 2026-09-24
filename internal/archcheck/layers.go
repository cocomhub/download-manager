// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package archcheck 是分层与包可见性的可执行门禁：把仓库的层级关系
// 变成断言，而不是文档里的一句话。
//
// 新增包必须登记进 Levels，否则校验失败——这是刻意的：门禁的价值就在于
// 强迫作者显式声明新包在依赖图中的位置。
package archcheck

// Levels 是包 → 层级（数字越小越底层）。L(n) 不得导入 L(>n)。
//
// dm 的依赖图实测分层（2026-09-24 代码实证）：
//
//	L0 基础库       model / pkg/logutil / pkg/configutil / pkg/titlegroup / web（零仓内依赖，叶子）
//	L1 领域包       core(model) / config(logutil) / pkg/scrape / pkg/download* / storage(core,model)
//	L2 能力层       downloader(core,model,config,pkg/*) / task*(config,core,model,storage,pkg/*)
//	L3 编排层       manager(config,core,downloader,model,storage,task,pkg/*)
//	L4 接口层       api(config,core,manager,task,web)
//
// 表是**顶层包的冻结契约**：新增顶层包必须登记；确有正当理由的跨层新边，
// 改表并在提交说明里写明理由即可。
var Levels = map[string]int{
	// ---- L0 基础库（零仓内依赖，实测）----
	"github.com/cocomhub/download-manager/model":          0,
	"github.com/cocomhub/download-manager/pkg/logutil":    0,
	"github.com/cocomhub/download-manager/pkg/configutil": 0,
	"github.com/cocomhub/download-manager/pkg/titlegroup": 0,
	"github.com/cocomhub/download-manager/web":            0,

	// ---- L1 领域包（只依赖 L0 / 标准库 / 三方库）----
	"github.com/cocomhub/download-manager/core":                   1,
	"github.com/cocomhub/download-manager/config":                 1,
	"github.com/cocomhub/download-manager/pkg/scrape":             1,
	"github.com/cocomhub/download-manager/pkg/download":           1,
	"github.com/cocomhub/download-manager/pkg/download/extractor": 1,
	"github.com/cocomhub/download-manager/pkg/download/m3u8d":     1,
	"github.com/cocomhub/download-manager/pkg/download/proxy":     1,
	"github.com/cocomhub/download-manager/pkg/download/transport": 1,
	"github.com/cocomhub/download-manager/pkg/scraper_tunnel":     1,
	"github.com/cocomhub/download-manager/storage":                1,

	// ---- L2 能力层（任务/下载器，依赖 L0/L1）----
	"github.com/cocomhub/download-manager/downloader":       2,
	"github.com/cocomhub/download-manager/task":             2,
	"github.com/cocomhub/download-manager/task/TEMPLATE/ui": 2,
	"github.com/cocomhub/download-manager/task/hanime":      2,
	"github.com/cocomhub/download-manager/task/hanime/ui":   2,
	"github.com/cocomhub/download-manager/task/mock":        2,
	"github.com/cocomhub/download-manager/task/tktube":      2,
	"github.com/cocomhub/download-manager/task/tktube/ui":   2,
	"github.com/cocomhub/download-manager/task/urllist":     2,
	"github.com/cocomhub/download-manager/task/urllist/ui":  2,
	"github.com/cocomhub/download-manager/task/vikacg":      2,
	"github.com/cocomhub/download-manager/task/vikacg/ui":   2,

	// ---- L3 编排层（Manager）----
	"github.com/cocomhub/download-manager/manager": 3,

	// ---- L4 接口层（API）----
	"github.com/cocomhub/download-manager/api": 4,
}
