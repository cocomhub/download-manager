// Copyright 2026 The Cocomhub Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: Apache-2.0

// Package dlcore provides a low-level HTTP download client with support for
// proxy rotation, HLS/FFmpeg, progress tracking, and retry logic.
//
// Deprecated: This package is superseded by github.com/cocomhub/download-manager/pkg/download.
// New code should use the pkg/download package directly. Existing users should
// migrate to pkg/download for ongoing improvements and bug fixes.
//
// This package is retained only for backward compatibility and for use by
// cmd/scraper_get. No new features will be added.
package dlcore
