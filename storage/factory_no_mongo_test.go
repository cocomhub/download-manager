// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

//go:build no_mongo

package storage

import (
	"errors"
	"testing"
)

func TestNoMongoBuildTag(t *testing.T) {
	s, err := NewStorage("mongo", nil)
	if err == nil {
		t.Fatal("expected error when creating mongo storage with no_mongo build tag")
	}
	if s != nil {
		t.Fatal("expected nil storage when creating mongo storage with no_mongo build tag")
	}
	// Verify we get the specific "not available" error, not "unknown storage type".
	if !errors.Is(err, errMongoNotAvailable) {
		t.Fatalf("expected errMongoNotAvailable, got: %v", err)
	}
}
