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
	"github.com/gorilla/mux"
)

// filesTestSetup creates a temp download root with a benign file and returns
// the ready router along with the root dir. Tests must not rely on any global
// config state beyond the isolated temp dir.
func filesTestSetup(t *testing.T) (*mux.Router, *Server, string) {
	t.Helper()
	root := t.TempDir()
	// A small HTML file exercises the blocked-MIME path (text/html).
	htmlPath := filepath.Join(root, "x.html")
	if err := os.WriteFile(htmlPath, []byte("<html><body>hi</body></html>"), 0o644); err != nil {
		t.Fatalf("write html fixture: %v", err)
	}
	// A plain text file is served normally.
	mp4Path := filepath.Join(root, "x.mp4")
	if err := os.WriteFile(mp4Path, []byte("plain binary content"), 0o644); err != nil {
		t.Fatalf("write text fixture: %v", err)
	}
	// A nested subdirectory file for positive lookups.
	subDir := filepath.Join(root, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "a.txt"), []byte("nested text"), 0o644); err != nil {
		t.Fatalf("write nested fixture: %v", err)
	}

	cfg := &config.Config{
		Server: config.Server{
			WorkDir:  t.TempDir(),
			FilesDir: root,
		},
		Runtime: config.Runtime{
			Mode: config.RunModeFull,
			Download: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{Enabled: true},
			Scheduler: struct {
				Enabled bool `yaml:"enabled" json:"enabled"`
			}{Enabled: true},
		},
	}
	srv := NewServer(newTestManager(cfg))
	return srv.Router(), srv, root
}

func TestFiles_PathTraversalRejected(t *testing.T) {
	_, srv, _ := filesTestSetup(t)

	// 单元级：直接调 filesHandler（不经 mux CleanPath），验证 handler 自身拒绝 ".."。
	req := httptest.NewRequest(http.MethodGet, "/files/../../etc/passwd", nil)
	rr := httptest.NewRecorder()
	srv.filesHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

func TestFiles_MuxCleanPathRedirectsTraversal(t *testing.T) {
	r, _, _ := filesTestSetup(t)

	// 集成级：经 mux 路由，含 ".." 的路径被 gorilla/mux CleanPath 规范化重定向（301），
	// 不会触达 handler —— 这本身也是防穿越（不会 200 服务文件）。
	req := httptest.NewRequest(http.MethodGet, "/files/../../etc/passwd", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code == http.StatusOK {
		t.Errorf("traversal must not serve file content, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestFiles_MIMETextHTMLRejected(t *testing.T) {
	r, _, _ := filesTestSetup(t)

	req := httptest.NewRequest(http.MethodGet, "/files/x.html", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

func TestFiles_ReadOnly(t *testing.T) {
	r, _, _ := filesTestSetup(t)

	req := httptest.NewRequest(http.MethodPost, "/files/x.mp4", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405; body=%s", rr.Code, rr.Body.String())
	}
}

func TestFiles_NoSniffHeader(t *testing.T) {
	r, _, _ := filesTestSetup(t)

	req := httptest.NewRequest(http.MethodGet, "/files/x.mp4", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := rr.Header().Get("Content-Length"); got == "" {
		t.Error("Content-Length header missing on file response")
	}
}
