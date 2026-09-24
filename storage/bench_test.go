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
	for b.Loop() {
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
	for b.Loop() {
		_, _ = s.Search(&core.StorageQuery{
			Filter: core.StorageFilter{
				Statuses: []string{model.StatusCompleted},
				TaskIDs:  []string{"task-0"},
			},
		})
	}
}

// BenchmarkMemoryStorage_Search5K 大样本（5000 对象）搜索基准，用于聚合下推回归护栏。
func BenchmarkMemoryStorage_Search5K(b *testing.B) {
	b.Helper()
	s, err := NewMemoryStorage(nil)
	if err != nil {
		b.Fatalf("NewMemoryStorage: %v", err)
	}
	for i := range 5000 {
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
	for b.Loop() {
		_, _ = s.Search(&core.StorageQuery{
			Filter: core.StorageFilter{
				Statuses: []string{model.StatusCompleted},
				TaskIDs:  []string{"task-0"},
			},
		})
	}
}

// BenchmarkMemoryStorage_Search10K 大样本（10000 对象）搜索基准。
func BenchmarkMemoryStorage_Search10K(b *testing.B) {
	b.Helper()
	s, err := NewMemoryStorage(nil)
	if err != nil {
		b.Fatalf("NewMemoryStorage: %v", err)
	}
	for i := range 10000 {
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
	for b.Loop() {
		_, _ = s.Search(&core.StorageQuery{
			Filter: core.StorageFilter{
				Statuses: []string{model.StatusCompleted},
				TaskIDs:  []string{"task-0"},
			},
		})
	}
}
