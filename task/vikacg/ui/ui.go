// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package ui registers VikACG custom UI assets (image viewer) via the
// TaskUIAssets framework, loaded dynamically into the Web UI when
// the user browses a vikacg-type completed object.
package ui

import (
	"embed"

	"github.com/cocomhub/download-manager/core"
)

//go:embed assets/viewer.js
var assets embed.FS

func init() {
	core.RegisterTaskUI("vikacg", core.TaskUIAssets{
		FS:      assets,
		JSPaths: []string{"assets/viewer.js"},
		Label:   "浏览",
	})
}
