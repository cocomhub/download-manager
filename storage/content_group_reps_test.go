// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

//go:build !no_mongo

package storage

import (
	"testing"

	"github.com/cocomhub/download-manager/core"
)

// TestMongoStorageImplementsContentGroupRepresentatives 编译期断言：
// MongoStorage 实现 ContentGroupRepresentatives（分组聚合下推存储层）。
func TestMongoStorageImplementsContentGroupRepresentatives(t *testing.T) {
	var _ core.ContentGroupRepresentatives = (*MongoStorage)(nil)
}
