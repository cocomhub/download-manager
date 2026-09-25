// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package mockui

import (
	"testing"

	"github.com/cocomhub/download-manager/core"
)

// TestUIAssetsRegistered verifies the mock task UI plugin is registered.
func TestUIAssetsRegistered(t *testing.T) {
	assets, ok := core.GetTaskUI("mock")
	if !ok {
		t.Fatal("mock UI assets not registered")
	}
	if !assets.HasViewer {
		t.Fatal("mock UI should have viewer")
	}
}
