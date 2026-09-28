// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/manager"
)

// newTestManagerWithRoot 构造带指定下载根的 manager（测试辅助）。
func newTestManagerWithRoot(t *testing.T, root string) *manager.Manager {
	t.Helper()
	cfg := &config.Config{}
	cfg.Server.DownloadRootDir = root
	return manager.NewManager(cfg)
}

// TestFilesHandler_NormalFile 回归：root 内正常文件默认 200 且开关开启也 200。
func TestFilesHandler_NormalFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}

	// 默认（防护开）：正常文件 200。
	s1 := &Server{mgr: newTestManagerWithRoot(t, root)}
	h1 := s1.filesHandler()
	r1 := httptest.NewRequest(http.MethodGet, "/ok.txt", nil)
	w1 := httptest.NewRecorder()
	h1.ServeHTTP(w1, r1)
	if w1.Code != http.StatusOK {
		t.Errorf("默认正常文件应 200，got %d", w1.Code)
	}

	// 开关开启：正常文件 200。
	s2 := &Server{mgr: newTestManagerWithRoot(t, root), filesAllowSymlink: true}
	h2 := s2.filesHandler()
	r2 := httptest.NewRequest(http.MethodGet, "/ok.txt", nil)
	w2 := httptest.NewRecorder()
	h2.ServeHTTP(w2, r2)
	if w2.Code != http.StatusOK {
		t.Errorf("开关开启正常文件应 200，got %d", w2.Code)
	}
}

// TestFilesHandler_SymlinkEscapeBlocked 逃逸防护逻辑：
// root 内文件经 symlink 指向 root 外 → 403（Windows 建 symlink 受限，用真实逃逸路径模拟）。
// 用 root 外真实文件 + root 内 symlink 条件跳过（Windows）；核心逻辑在 EvalSymlinks 后比较。
func TestFilesHandler_SymlinkEscapeBlocked(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// 目标在 root 外。
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlink unsupported（Windows 无权限）")
	}

	s := &Server{mgr: newTestManagerWithRoot(t, root)}
	h := s.filesHandler()
	req := httptest.NewRequest(http.MethodGet, "/link/secret.txt", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("逃逸应 403，got %d", w.Code)
	}
}

// TestFilesHandler_SymlinkAllowed 开关开启：symlink 逃逸放行（mac 系统 symlink 场景）。
func TestFilesHandler_SymlinkAllowed(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlink unsupported（Windows 无权限）")
	}

	s := &Server{mgr: newTestManagerWithRoot(t, root), filesAllowSymlink: true}
	h := s.filesHandler()
	req := httptest.NewRequest(http.MethodGet, "/link/secret.txt", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("开关开启应放行，got %d", w.Code)
	}
}
