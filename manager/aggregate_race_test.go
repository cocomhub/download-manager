// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"sync"
	"testing"

	"github.com/cocomhub/download-manager/model"
)

// TestRace_VariantPriorityScoreConcurrentRead 验证聚合分组路径（variantPriorityScore
// 读 obj.Metadata["title"]）与并发写 Metadata 不触发 data race。
// 变异验证：去掉 variantPriorityScore 的 RLock 后本测试应红（data race）。
func TestRace_VariantPriorityScoreConcurrentRead(t *testing.T) {
	obj := &model.DownloadObject{
		URL:      "http://127.0.0.1/race/variant.bin",
		SavePath: "race/variant.bin",
		TaskID:   "race-variant",
		Metadata: map[string]string{"title": "some title"},
		Extra:    make(map[string]any),
	}
	tk := &mockTask{id: "race-variant", typ: "tktube"}

	var wg sync.WaitGroup
	// 写方：模拟 resolveApply / applySharedState 写 Metadata（持 obj.Lock）
	for range 3 {
		wg.Go(func() {
			for range 2000 {
				obj.Lock()
				obj.Metadata["title"] = "title-v2"
				obj.Unlock()
			}
		})
	}
	// 读方：variantPriorityScore（内部 RLock）
	for range 3 {
		wg.Go(func() {
			for range 2000 {
				_ = variantPriorityScore(tk, obj)
			}
		})
	}
	wg.Wait()
}
