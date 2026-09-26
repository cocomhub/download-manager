// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	"github.com/cocomhub/download-manager/pkg/logutil"
)

type FileStorage struct {
	filePath     string
	objects      map[string]*model.DownloadObject // Cache: URL -> Object
	mu           sync.RWMutex
	dirty        bool
	saveInterval time.Duration
	saveTimer    *time.Timer
	loaded       bool // 是否已从文件加载（惰性加载标记）
}

func NewFileStorage(config map[string]string) (*FileStorage, error) {
	path, ok := config["path"]
	if !ok || path == "" {
		return nil, fmt.Errorf("file storage requires 'path' config")
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	// Parse save_interval
	interval := 1 * time.Second // Default
	if val, ok := config["save_interval"]; ok && val != "" {
		if i, err := strconv.Atoi(val); err == nil && i > 0 {
			interval = time.Duration(i) * time.Second
		}
	}

	fs := &FileStorage{
		filePath:     path,
		objects:      make(map[string]*model.DownloadObject),
		saveInterval: interval,
	}

	// 惰性加载：不在构造时读全量文件（启动内存峰值优化）。
	// 首次任何读/写访问（ensureLoaded）才从文件加载。
	// 注意：不再调用 loadFromFile()。

	return fs, nil
}

// ensureLoaded 首次访问时从文件加载对象（双重检查锁）。
// 所有读/写入口（Get/Update/Delete/Search/Count/Exists）先调用。
func (s *FileStorage) ensureLoaded() error {
	s.mu.RLock()
	if s.loaded {
		s.mu.RUnlock()
		return nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return nil
	}
	s.loaded = true
	return s.loadFromFile()
}

func (s *FileStorage) loadFromFile() error {
	// 调用方必须已持锁（ensureLoaded 或构造后首次加载）。

	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		return nil // Return empty list if file doesn't exist
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	var list []*model.DownloadObject
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}

	for _, obj := range list {
		s.objects[obj.URL] = obj
	}
	return nil
}

func (s *FileStorage) Get(id string) (*model.DownloadObject, error) {
	if err := s.ensureLoaded(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if obj, ok := s.objects[id]; ok {
		return obj, nil
	}
	return nil, nil // Not found
}

func (s *FileStorage) Update(obj *model.DownloadObject) error {
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.objects[obj.URL] = obj
	s.dirty = true

	// If no timer is running, start one to save after interval
	if s.saveTimer == nil {
		s.saveTimer = time.AfterFunc(s.saveInterval, s.flushAsync)
	}

	return nil
}

func (s *FileStorage) Delete(id string) error {
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, id)
	s.dirty = true

	if s.saveTimer == nil {
		s.saveTimer = time.AfterFunc(s.saveInterval, s.flushAsync)
	}

	return nil
}

func (s *FileStorage) Search(query *core.StorageQuery) ([]*model.DownloadObject, error) {
	if err := s.ensureLoaded(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 过滤先行：仅收集匹配项，避免对全量对象做一次 copy 再过滤。
	// 排序与分页（offset/limit）语义由共享查询层下推执行。
	return FilterAndPageObjects(s.objects, query), nil
}

func (s *FileStorage) Count(query *core.StorageQuery) (int64, error) {
	if err := s.ensureLoaded(); err != nil {
		return 0, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*model.DownloadObject, 0, len(s.objects))
	for _, obj := range s.objects {
		list = append(list, obj)
	}
	return CountObjects(list, query), nil
}

func (s *FileStorage) Exists(ids []string) (map[string]bool, error) {
	if err := s.ensureLoaded(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]bool, len(ids))
	for _, id := range ids {
		_, ok := s.objects[id]
		result[id] = ok
	}
	return result, nil
}

// flushAsync is called by the timer
func (s *FileStorage) flushAsync() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Reset timer so next update triggers a new one
	s.saveTimer = nil

	if s.dirty {
		if err := s.saveLocked(); err != nil {
			slog.Error("Error saving file storage", logutil.LogKeyError, err)
		}
	}
}

func (s *FileStorage) saveLocked() error {
	var list []*model.DownloadObject
	for _, obj := range s.objects {
		list = append(list, obj)
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	// Write to temp file first then rename for atomic write
	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		return err
	}

	s.dirty = false
	return nil
}

// ForceFlush allows manual saving (e.g. on shutdown)
func (s *FileStorage) ForceFlush() error {
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.saveTimer != nil {
		s.saveTimer.Stop()
		s.saveTimer = nil
	}

	if s.dirty {
		return s.saveLocked()
	}
	return nil
}
