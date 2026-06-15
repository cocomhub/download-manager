// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"testing"

	"github.com/cocomhub/download-manager/config"
)

func TestNew_Native(t *testing.T) {
	cfg := config.Downloader{Type: "native"}
	d := New(cfg)
	if d == nil {
		t.Fatal("New() with type=native returned nil")
	}
	if got := d.Name(); got == "" {
		t.Error("Name() returned empty")
	}
}

func TestNew_Default(t *testing.T) {
	cfg := config.Downloader{Type: ""}
	d := New(cfg)
	if d == nil {
		t.Fatal("New() with empty type returned nil")
	}
}

func TestNew_UnknownType(t *testing.T) {
	cfg := config.Downloader{Type: "nonexistent"}
	d := New(cfg)
	if d == nil {
		t.Fatal("New() with unknown type returned nil")
	}
}
