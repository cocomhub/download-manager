// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package core

type StorageProvider interface {
	GetStorage() Storage
}

type PathStrategy interface {
	Resolve(baseDir string, taskID string, title string, fileType string) (string, string)
}
