// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"fmt"
	"sync"

	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
)

// Factory defines the function signature for creating storage
type Factory func(config map[string]string) (core.Storage, error)

var factories = make(map[string]Factory)

// Register registers a new storage factory
func Register(typ string, f Factory) {
	factories[typ] = f
}

// NewStorage creates a new storage instance based on type and config.
func NewStorage(typ string, config map[string]string) (core.Storage, error) {
	f, ok := factories[typ]
	if !ok {
		return nil, fmt.Errorf("unknown storage type: %s", typ)
	}
	return f(config)
}

// MemoryStorage implementation keeps task state in-process.
type MemoryStorage struct {
	mu      sync.RWMutex
	objects map[string]*model.DownloadObject
}

// NewMemoryStorage creates a new memory storage
func NewMemoryStorage(config map[string]string) (core.Storage, error) {
	return &MemoryStorage{objects: make(map[string]*model.DownloadObject)}, nil
}

func (s *MemoryStorage) Get(id string) (*model.DownloadObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.objects[id], nil
}

func (s *MemoryStorage) Update(obj *model.DownloadObject) error {
	if obj == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[obj.URL] = obj
	return nil
}

func (s *MemoryStorage) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, id)
	return nil
}

func (s *MemoryStorage) Search(query *core.StorageQuery) ([]*model.DownloadObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*model.DownloadObject, 0, len(s.objects))
	for _, obj := range s.objects {
		list = append(list, obj)
	}
	return ApplyQueryToObjects(list, query), nil
}

func (s *MemoryStorage) Count(query *core.StorageQuery) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*model.DownloadObject, 0, len(s.objects))
	for _, obj := range s.objects {
		list = append(list, obj)
	}
	return CountObjects(list, query), nil
}

func (s *MemoryStorage) Exists(ids []string) (map[string]bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]bool, len(ids))
	for _, id := range ids {
		_, ok := s.objects[id]
		result[id] = ok
	}
	return result, nil
}

func init() {
	Register("file", func(config map[string]string) (core.Storage, error) {
		return NewFileStorage(config)
	})
	Register("memory", NewMemoryStorage)
}
