// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"fmt"
	"testing"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

func BenchmarkMemoryStorage_Search(b *testing.B) {
	s, err := NewMemoryStorage(nil)
	if err != nil {
		b.Fatalf("NewMemoryStorage: %v", err)
	}
	for i := range 100 {
		obj := &model.DownloadObject{
			URL:    fmt.Sprintf("http://example.com/file-%d.zip", i),
			Status: model.StatusPending,
		}
		_ = s.Update(obj)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.Search(&core.StorageQuery{
			Filter: core.StorageFilter{Statuses: []string{model.StatusPending}},
		})
	}
}

func BenchmarkMemoryStorage_FullScan(b *testing.B) {
	s, err := NewMemoryStorage(nil)
	if err != nil {
		b.Fatalf("NewMemoryStorage: %v", err)
	}
	for i := range 1000 {
		obj := &model.DownloadObject{
			TaskID: fmt.Sprintf("task-%d", i%10),
			URL:    fmt.Sprintf("http://example.com/file-%d.zip", i),
			Status: model.StatusPending,
		}
		if i%3 == 0 {
			obj.Status = model.StatusCompleted
		} else if i%7 == 0 {
			obj.Status = model.StatusFailed
		}
		_ = s.Update(obj)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.Search(&core.StorageQuery{
			Filter: core.StorageFilter{
				Statuses: []string{model.StatusCompleted},
				TaskIDs:  []string{"task-0"},
			},
		})
	}
}
