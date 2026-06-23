// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

//go:build no_mongo

package storage

import (
	"testing"
)

func TestNoMongoBuildTag(t *testing.T) {
	_, err := NewStorage("mongo", nil)
	if err == nil {
		t.Fatal("expected error when creating mongo storage with no_mongo build tag")
	}
}
