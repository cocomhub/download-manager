// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package dlcore

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cocomhub/download-manager/pkg/download" //nolint:staticcheck
)

// Root 返回 rootDir 的 DirFS。
func Root(rootDir string) fs.FS {
	return os.DirFS(rootDir)
}

// CleanJoin 将 rootDir 与任意元素拼接并用 filepath.Clean 规范化。
func CleanJoin(rootDir string, elems ...string) (string, error) {
	all := append([]string{rootDir}, elems...)
	p := filepath.Join(all...)
	p = filepath.Clean(p)
	return p, nil
}

// IsWithinRoot 委托给 pkg/download.IsWithinRoot。
func IsWithinRoot(rootDir, p string) bool {
	return download.IsWithinRoot(rootDir, p)
}

// ResolvePath 委托给 pkg/download.ResolvePath。
func ResolvePath(rootDir, p string) (string, error) {
	return download.ResolvePath(rootDir, p)
}
