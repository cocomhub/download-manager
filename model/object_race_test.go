// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"sync"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestSnapshotConcurrentMediaWrites 复现线上 panic 场景：MongoStorage.Update 走
// Snapshot + BSON 编码（读 obj 的 map），同时 soWorker 的 finalizeSmallObject 写
// obj.Extra 的媒体字段。修复前会触发 concurrent map iteration and map write，
// 修复后（Snapshot 在 RLock 下深拷贝 + 访问器全加锁）用 -race 运行应无数据竞争。
func TestSnapshotConcurrentMediaWrites(t *testing.T) {
	obj := &DownloadObject{
		URL:      "u",
		Metadata: map[string]string{"etag": "x"},
		Extra: map[string]any{
			"files": []any{map[string]string{"url": "u", "path": "p"}},
			"tags":  []string{"a"},
		},
	}
	var wg sync.WaitGroup

	// 读侧：模拟 MongoStorage.Update 的 marshal 路径（对应 flusher / UpdateStatus）。
	for range 4 {
		wg.Go(func() {
			for range 2000 {
				snap := obj.Snapshot()
				if _, err := bson.Marshal(bson.M{"$set": snap}); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}

	// 写侧：模拟 makeOnMetadataCallback + finalizeSmallObject 的媒体字段写回。
	for range 2 {
		wg.Go(func() {
			for range 2000 {
				obj.SetMedia(MediaRelCover, "https://c", "/c.jpg")
				obj.SetLocalCover("/c.jpg")
				obj.SetLocalPreview("/p.mp4")
				obj.SetGroupSize(3)
				obj.SetContentGroup("g")
				obj.Lock()
				obj.Metadata["etag"] = "y"
				obj.Unlock()
			}
		})
	}

	wg.Wait()
}
