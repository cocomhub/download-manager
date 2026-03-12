// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"fmt"

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

// NewStorage creates a new storage instance based on type and config
func NewStorage(typ string, config map[string]string) (core.Storage, error) {
	if typ == "" {
		return NewMemoryStorage(nil)
	}
	f, ok := factories[typ]
	if !ok {
		return nil, fmt.Errorf("unknown storage type: %s", typ)
	}
	return f(config)
}

// MemoryStorage implementation (No-op persistence)
type MemoryStorage struct{}

// NewMemoryStorage creates a new memory storage
func NewMemoryStorage(config map[string]string) (core.Storage, error) {
	return &MemoryStorage{}, nil
}

func (s *MemoryStorage) Get(id string) (*model.DownloadObject, error)       { return nil, nil }
func (s *MemoryStorage) Update(obj *model.DownloadObject) error             { return nil }
func (s *MemoryStorage) Delete(id string) error                             { return nil }
func (s *MemoryStorage) Search(filter any) ([]*model.DownloadObject, error) { return nil, nil }

func init() {
	Register("file", func(config map[string]string) (core.Storage, error) {
		return NewFileStorage(config)
	})
	Register("mongo", func(config map[string]string) (core.Storage, error) {
		return NewMongoStorage(config)
	})
	Register("memory", NewMemoryStorage)
}
