// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"sync"
	"testing"

	"github.com/cocomhub/download-manager/model"
)

// TestRace_ObjectMetadataConcurrentRW 验证 obj.Metadata/Extra 并发读写
// （resolve 写 obj.Lock vs 读方 RLock）不触发 data race。
// 模拟 storage/query、scheduler hasFiles 的读路径与任务 resolve 写路径竞争。
func TestRace_ObjectMetadataConcurrentRW(t *testing.T) {
	obj := &model.DownloadObject{
		URL:      "http://127.0.0.1/race/rw.bin",
		SavePath: "race/rw.bin",
		Metadata: make(map[string]string),
		Extra:    make(map[string]any),
	}

	var wg sync.WaitGroup
	// 写方：模拟 resolveApply 写 Metadata/Extra（持 obj.Lock）
	for range 4 {
		wg.Go(func() {
			for range 1000 {
				obj.Lock()
				obj.Metadata["title"] = "t"
				obj.Extra["files"] = []map[string]string{{"url": "u", "path": "p"}}
				obj.Unlock()
			}
		})
	}
	// 读方：模拟 hasFiles / storage query（持 obj.RLock）
	for range 4 {
		wg.Go(func() {
			for range 1000 {
				obj.RLock()
				_ = obj.Extra["files"]
				_ = obj.Metadata["title"]
				obj.RUnlock()
			}
		})
	}
	wg.Wait()
}
