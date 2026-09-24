// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package ui registers booksite custom UI assets via the TaskUIAssets framework.
package ui

import (
	"embed"

	"github.com/cocomhub/download-manager/core"
)

//go:embed assets/viewer.js
var assets embed.FS

func init() {
	core.RegisterTaskUI("booksite", core.TaskUIAssets{
		FS:        assets,
		JSPaths:   []string{"assets/viewer.js"},
		Label:     "Book Site",
		HasForm:   true,
		HasViewer: true,
	})
}
