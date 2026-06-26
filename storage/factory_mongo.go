// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

//go:build !no_mongo

package storage

import "github.com/cocomhub/download-manager/core"

func init() {
	Register("mongo", func(config map[string]string) (core.Storage, error) {
		return NewMongoStorage(config)
	})
}
