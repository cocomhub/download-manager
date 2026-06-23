// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package task

import "strings"

func NormalizeType(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
