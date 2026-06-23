// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package dlcore

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func Root(rootDir string) fs.FS {
	return os.DirFS(rootDir)
}

func CleanJoin(rootDir string, elems ...string) (string, error) {
	all := append([]string{rootDir}, elems...)
	p := filepath.Join(all...)
	p = filepath.Clean(p)
	return p, nil
}

func IsWithinRoot(rootDir, p string) bool {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return false
	}
	absP, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	if absRoot == absP {
		return true
	}
	if !strings.HasSuffix(absRoot, string(filepath.Separator)) {
		absRoot = absRoot + string(filepath.Separator)
	}
	return strings.HasPrefix(absP, absRoot)
}

func ResolvePath(rootDir, p string) (string, error) {
	if rootDir == "" {
		return p, nil
	}
	if filepath.IsAbs(p) {
		if IsWithinRoot(rootDir, p) {
			return p, nil
		}
		return "", fmt.Errorf("path outside root: %s", p)
	}
	rp, err := CleanJoin(rootDir, p)
	if err != nil {
		return "", err
	}
	if !IsWithinRoot(rootDir, rp) {
		return "", fmt.Errorf("path outside root: %s", p)
	}
	return rp, nil
}
