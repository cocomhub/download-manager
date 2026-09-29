// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

// BenchmarkFileStorage_Search_Pushdown 5000+ 对象 file 后端：下推分页
// （limit=50 offset=100 + sort）vs 全量拉取后内存 ApplyQueryToObjects 分页。
func BenchmarkFileStorage_Search_Pushdown(b *testing.B) {
	dir := b.TempDir()
	s, err := NewFileStorage(map[string]string{"path": filepath.Join(dir, "objects.json"), "save_interval": "3600s"})
	if err != nil {
		b.Fatalf("NewFileStorage: %v", err)
	}
	for i := range 5000 {
		obj := &model.DownloadObject{
			TaskID:   fmt.Sprintf("task-%d", i%10),
			URL:      fmt.Sprintf("http://example.com/file-%04d.zip", i),
			Status:   model.StatusPending,
			Metadata: map[string]string{"date": fmt.Sprintf("2026-%02d-%02d", i%12+1, i%28+1), "title": fmt.Sprintf("title-%04d", i)},
		}
		if i%3 == 0 {
			obj.Status = model.StatusCompleted
		}
		if err := s.Update(obj); err != nil {
			b.Fatalf("Update: %v", err)
		}
	}
	// 预热后强制落盘一次，避免基准热循环中惰性 save timer 并发写盘干扰计时。
	if err := s.ForceFlush(); err != nil {
		b.Fatalf("ForceFlush: %v", err)
	}
	b.ResetTimer()

	b.Run("pushdown_limit50_offset100", func(b *testing.B) {
		for b.Loop() {
			_, _ = s.Search(&core.StorageQuery{
				Filter: core.StorageFilter{Statuses: []string{model.StatusCompleted}},
				Sort:   []core.StorageSort{{Field: "date", Desc: true}, {Field: "url"}},
				Limit:  50,
				Offset: 100,
			})
		}
	})

	b.Run("fullscan_then_page", func(b *testing.B) {
		for b.Loop() {
			all, _ := s.Search(&core.StorageQuery{
				Filter: core.StorageFilter{Statuses: []string{model.StatusCompleted}},
				Sort:   []core.StorageSort{{Field: "date", Desc: true}, {Field: "url"}},
			})
			_ = ApplyQueryToObjects(all, &core.StorageQuery{
				Limit:  50,
				Offset: 100,
			})
		}
	})
}
