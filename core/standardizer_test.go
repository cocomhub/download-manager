// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	"github.com/cocomhub/download-manager/model"
)

// TestStandardizerInterface 验证 Standardizer 接口存在且可被实现
func TestStandardizerInterface(t *testing.T) {
	// 编译时检查：确保 Standardizer 接口存在并包含 Standardize 方法
	var _ Standardizer = &mockStandardizer{}
}

// mockStandardizer 实现 Standardizer 接口用于编译检查
type mockStandardizer struct{}

func (m *mockStandardizer) Standardize(obj *model.DownloadObject) (bool, error) {
	if obj == nil {
		return false, nil
	}
	// 模拟：如果对象没有 ID，设置为 1 并标记已修改
	if obj.GetID() == 0 {
		obj.SetID(1)
		return true, nil
	}
	return false, nil
}

func TestStandardizer_Standardize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		obj          *model.DownloadObject
		wantModified bool
		wantErr      bool
		wantID       int64
	}{
		{
			name:         "nil object",
			obj:          nil,
			wantModified: false,
			wantErr:      false,
			wantID:       0,
		},
		{
			name: "object with ID 0 gets ID assigned",
			obj: &model.DownloadObject{
				URL: "https://example.com/file1",
				ID:  0,
			},
			wantModified: true,
			wantErr:      false,
			wantID:       1,
		},
		{
			name: "object with existing ID not modified",
			obj: &model.DownloadObject{
				URL: "https://example.com/file2",
				ID:  42,
			},
			wantModified: false,
			wantErr:      false,
			wantID:       42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := &mockStandardizer{}
			modified, err := s.Standardize(tt.obj)

			if (err != nil) != tt.wantErr {
				t.Errorf("Standardize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if modified != tt.wantModified {
				t.Errorf("Standardize() modified = %v, want %v", modified, tt.wantModified)
			}
			if tt.obj != nil && tt.obj.ID != tt.wantID {
				t.Errorf("Standardize() obj.ID = %d, want %d", tt.obj.ID, tt.wantID)
			}
		})
	}
}

// TestStorageFilter_NewFields 验证 StorageFilter 的新增字段（MissingID, Tags, TagMode, ExcludeIDs）
func TestStorageFilter_NewFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		filter    StorageFilter
		checkFunc func(t *testing.T, f StorageFilter)
	}{
		{
			name: "MissingID true",
			filter: StorageFilter{
				MissingID: new(true),
			},
			checkFunc: func(t *testing.T, f StorageFilter) {
				if f.MissingID == nil {
					t.Fatal("MissingID is nil")
				}
				if *f.MissingID != true {
					t.Errorf("MissingID = %v, want true", *f.MissingID)
				}
			},
		},
		{
			name: "MissingID false",
			filter: StorageFilter{
				MissingID: new(false),
			},
			checkFunc: func(t *testing.T, f StorageFilter) {
				if f.MissingID == nil {
					t.Fatal("MissingID is nil")
				}
				if *f.MissingID != false {
					t.Errorf("MissingID = %v, want false", *f.MissingID)
				}
			},
		},
		{
			name: "MissingID nil",
			filter: StorageFilter{
				MissingID: nil,
			},
			checkFunc: func(t *testing.T, f StorageFilter) {
				if f.MissingID != nil {
					t.Errorf("MissingID = %v, want nil", *f.MissingID)
				}
			},
		},
		{
			name: "Tags populated",
			filter: StorageFilter{
				Tags: []string{"tag1", "tag2"},
			},
			checkFunc: func(t *testing.T, f StorageFilter) {
				if len(f.Tags) != 2 {
					t.Fatalf("Tags length = %d, want 2", len(f.Tags))
				}
				if f.Tags[0] != "tag1" || f.Tags[1] != "tag2" {
					t.Errorf("Tags = %v, want [tag1 tag2]", f.Tags)
				}
			},
		},
		{
			name: "Tags empty",
			filter: StorageFilter{
				Tags: []string{},
			},
			checkFunc: func(t *testing.T, f StorageFilter) {
				if len(f.Tags) != 0 {
					t.Errorf("Tags length = %d, want 0", len(f.Tags))
				}
			},
		},
		{
			name: "TagMode any",
			filter: StorageFilter{
				TagMode: "any",
			},
			checkFunc: func(t *testing.T, f StorageFilter) {
				if f.TagMode != "any" {
					t.Errorf("TagMode = %q, want any", f.TagMode)
				}
			},
		},
		{
			name: "TagMode all",
			filter: StorageFilter{
				TagMode: "all",
			},
			checkFunc: func(t *testing.T, f StorageFilter) {
				if f.TagMode != "all" {
					t.Errorf("TagMode = %q, want all", f.TagMode)
				}
			},
		},
		{
			name: "TagMode empty",
			filter: StorageFilter{
				TagMode: "",
			},
			checkFunc: func(t *testing.T, f StorageFilter) {
				if f.TagMode != "" {
					t.Errorf("TagMode = %q, want empty", f.TagMode)
				}
			},
		},
		{
			name: "ExcludeIDs populated",
			filter: StorageFilter{
				ExcludeIDs: []int64{1, 2, 3},
			},
			checkFunc: func(t *testing.T, f StorageFilter) {
				if len(f.ExcludeIDs) != 3 {
					t.Fatalf("ExcludeIDs length = %d, want 3", len(f.ExcludeIDs))
				}
				expected := []int64{1, 2, 3}
				for i, v := range expected {
					if f.ExcludeIDs[i] != v {
						t.Errorf("ExcludeIDs[%d] = %d, want %d", i, f.ExcludeIDs[i], v)
					}
				}
			},
		},
		{
			name: "ExcludeIDs empty",
			filter: StorageFilter{
				ExcludeIDs: []int64{},
			},
			checkFunc: func(t *testing.T, f StorageFilter) {
				if len(f.ExcludeIDs) != 0 {
					t.Errorf("ExcludeIDs length = %d, want 0", len(f.ExcludeIDs))
				}
			},
		},
		{
			name: "all new fields combined",
			filter: StorageFilter{
				TaskIDs:    []string{"task1"},
				MissingID:  new(true),
				Tags:       []string{"recommended"},
				TagMode:    "any",
				ExcludeIDs: []int64{5, 10},
			},
			checkFunc: func(t *testing.T, f StorageFilter) {
				if f.MissingID == nil || *f.MissingID != true {
					t.Errorf("MissingID = %v, want true", f.MissingID)
				}
				if len(f.Tags) != 1 || f.Tags[0] != "recommended" {
					t.Errorf("Tags = %v, want [recommended]", f.Tags)
				}
				if f.TagMode != "any" {
					t.Errorf("TagMode = %q, want any", f.TagMode)
				}
				if len(f.ExcludeIDs) != 2 || f.ExcludeIDs[0] != 5 || f.ExcludeIDs[1] != 10 {
					t.Errorf("ExcludeIDs = %v, want [5 10]", f.ExcludeIDs)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.checkFunc(t, tt.filter)
		})
	}
}
