// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package ui registers tktube custom UI assets (content-grouped task view)
// via the TaskUIAssets framework.
package ui

import (
	"embed"

	"github.com/cocomhub/download-manager/core"
)

//go:embed assets/viewer.js
var assets embed.FS

func init() {
	core.RegisterTaskUI("tktube", core.TaskUIAssets{
		FS:           assets,
		JSPaths:      []string{"assets/viewer.js"},
		Label:        "",
		HasForm:      true,
		HasViewer:    true,
		HasAggregate: true,
	})
}
