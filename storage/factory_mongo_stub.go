// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

//go:build no_mongo

package storage

import (
	"errors"

	"github.com/cocomhub/download-manager/core"
)

// errMongoNotAvailable is returned when mongo storage is requested in a no_mongo build.
var errMongoNotAvailable = errors.New("mongo storage not available: built with no_mongo tag")

func init() {
	Register("mongo", func(config map[string]string) (core.Storage, error) {
		return nil, errMongoNotAvailable
	})
}
