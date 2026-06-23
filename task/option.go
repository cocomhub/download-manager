// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/pkg/scrape"
)

type Options struct {
	store  core.Storage
	driver *scrape.Driver
}

type Option func(*Options)

func WithStore(store core.Storage) Option {
	return func(o *Options) {
		o.store = store
	}
}

func WithDriver(driver *scrape.Driver) Option {
	return func(o *Options) {
		o.driver = driver
	}
}
