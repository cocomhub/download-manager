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

	"github.com/cocomhub/download-manager/model"
)

type FileStorage struct {
	filePath     string
	objects      map[string]*model.DownloadObject // Cache: URL -> Object
	mu           sync.RWMutex
	dirty        bool
	saveInterval time.Duration
	saveTimer    *time.Timer
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

	// Initial Load
	if err := fs.loadFromFile(); err != nil {
		return nil, err
	}

	return fs, nil
}

func (s *FileStorage) loadFromFile() error {
	s.mu.Lock()
	defer s.mu.Unlock()

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
	s.mu.RLock()
	defer s.mu.RUnlock()
	if obj, ok := s.objects[id]; ok {
		return obj, nil
	}
	return nil, nil // Not found
}

func (s *FileStorage) Update(obj *model.DownloadObject) error {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, id)
	s.dirty = true

	if s.saveTimer == nil {
		s.saveTimer = time.AfterFunc(s.saveInterval, s.flushAsync)
	}

	return nil
}

func (s *FileStorage) Search(filter any) ([]*model.DownloadObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*model.DownloadObject
	for _, obj := range s.objects {
		list = append(list, obj)
	}
	return list, nil
}

// flushAsync is called by the timer
func (s *FileStorage) flushAsync() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Reset timer so next update triggers a new one
	s.saveTimer = nil

	if s.dirty {
		if err := s.saveLocked(); err != nil {
			slog.Error("Error saving file storage", "error", err)
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
