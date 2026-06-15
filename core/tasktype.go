// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package core

// Task type constants matching task registration keys.
// These are used by the aggregation layer to identify task types
// without importing concrete task packages.
const (
	TaskTypeTktube  = "tktube"
	TaskTypeHanime  = "hanime"
	TaskTypeVikacg  = "vikacg"
	TaskTypeURLList = "url_list"
)
