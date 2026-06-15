// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestMainPackageCompiles(t *testing.T) {
	// 编译时检查：确保 main 函数存在
	_ = main
}
