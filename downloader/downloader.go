// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
)

func New(config config.Downloader) core.Downloader {
	switch config.Type {
	case "wget":
		return NewWgetDownloader(config)
	default:
		return NewNativeHTTPDownloader(config)
	}
}
