// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import "github.com/cocomhub/download-manager/core"

type Options struct {
	store core.Storage
}

type Option func(*Options)

func WithStore(store core.Storage) Option {
	return func(o *Options) {
		o.store = store
	}
}
