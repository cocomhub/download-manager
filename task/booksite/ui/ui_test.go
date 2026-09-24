// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"testing"

	"github.com/cocomhub/download-manager/core"
)

// TestUIAssetsRegistered verifies the booksite UI plugin is registered.
func TestUIAssetsRegistered(t *testing.T) {
	assets, ok := core.GetTaskUI("booksite")
	if !ok {
		t.Fatal("booksite UI assets not registered")
	}
	if !assets.HasForm || !assets.HasViewer {
		t.Fatal("booksite UI should have form and viewer")
	}
	if assets.Label != "Book Site" {
		t.Fatalf("booksite UI label = %q", assets.Label)
	}
}
