// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"

	"github.com/cocomhub/download-manager/model"
)

// PathStrategy defines how save paths are resolved for download objects.
type PathStrategy interface {
	Resolve(baseDir string, taskID string, title string, fileType string) (dir string, filename string)
}

// Scraper is implemented by tasks that support Manager-driven page scraping.
// Manager's scan loop calls Scrape(ctx) periodically to discover new objects.
// Implementations MUST honor ctx cancellation to allow timely shutdown.
type Scraper interface {
	Scrape(ctx context.Context) error
}

// SmallObjectInfo 鎻忚堪涓庝富瀵硅薄鍏宠仈鐨勫皬瀵硅薄锛坧review/cover/thumb锛夈€?// 灏忓璞＄敱 Manager 缁熶竴涓嬭浇锛屼笌涓讳綋鏂囦欢鍏辩敤鐩稿悓鐨勪笅杞介€昏緫锛圗Tag/checksum/閲嶈瘯锛夈€?type SmallObjectInfo struct {
	URL      string `json:"url"`      // 婧愮珯 URL
	SavePath string `json:"savePath"` // 鏈湴淇濆瓨璺緞
	Rel      string `json:"rel"`      // 鍏崇郴绫诲瀷: "cover", "preview", "thumb"
}

// SmallObjectProvider 鏄彲閫夋帴鍙ｏ紝鐢辨湁灏忓璞★紙灏侀潰/棰勮绛夛級鐨勪换鍔″疄鐜般€?// Task 鍙礋璐ｆ彁渚涘皬瀵硅薄鐨勪俊鎭紝瀹為檯涓嬭浇鐢?Manager 鐨?smallObjectWorker 姹犲畬鎴愩€?type SmallObjectProvider interface {
	// SmallObjects 杩斿洖缁欏畾涓诲璞″叧鑱旂殑灏忓璞″垪琛ㄣ€?	SmallObjects(obj *model.DownloadObject) []SmallObjectInfo
}
