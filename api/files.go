// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// filesHandler serves files under the configured download root in a
// read-only, traversal-safe manner:
//   - only GET/HEAD are allowed
//   - ".." segments and path traversal are rejected with 403
//   - MIME types that can smuggle active content (html/js/svg) are blocked
//   - responses always carry X-Content-Type-Options: nosniff
func (s *Server) filesHandler() http.Handler {
	root := filepath.Clean(s.mgr.GetDownloadRootDir())
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "read-only files")
			return
		}
		upath := r.URL.Path
		if strings.Contains(upath, "..") {
			writeJSONError(w, http.StatusForbidden, "forbidden", "invalid path")
			return
		}
		clean := filepath.Clean(filepath.Join(root, filepath.FromSlash(upath)))
		// 前缀边界：clean 必须等于 root 或位于 root/ 之下（防止 root=/data 时 /data2 误放行）。
		if clean != root && !strings.HasPrefix(clean, root+string(os.PathSeparator)) {
			writeJSONError(w, http.StatusForbidden, "forbidden", "path traversal")
			return
		}
		// symlink 逃逸防护：解析后真实路径必须仍在 root 内。
		if real, err := filepath.EvalSymlinks(clean); err == nil {
			real = filepath.Clean(real)
			if real != root && !strings.HasPrefix(real, root+string(os.PathSeparator)) {
				writeJSONError(w, http.StatusForbidden, "forbidden", "path traversal")
				return
			}
		}
		f, err := os.Open(clean)
		if err != nil {
			writeJSONError(w, http.StatusNotFound, "not_found", "file not found")
			return
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil || stat.IsDir() {
			writeJSONError(w, http.StatusNotFound, "not_found", "not a file")
			return
		}
		buf := make([]byte, 512)
		n, _ := f.Read(buf)
		ct := http.DetectContentType(buf[:n])
		if isBlockedMIME(ct) {
			writeJSONError(w, http.StatusForbidden, "forbidden", "content type blocked")
			return
		}
		_, _ = f.Seek(0, io.SeekStart)
		w.Header().Set("Content-Type", ct)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
		if r.Method == http.MethodHead {
			return
		}
		http.ServeContent(w, r, filepath.Base(clean), stat.ModTime(), f)
	})
}

// isBlockedMIME reports whether the detected content type may carry active
// content that must not be served from the same origin as the UI.
func isBlockedMIME(ct string) bool {
	switch {
	case strings.HasPrefix(ct, "text/html"), strings.HasPrefix(ct, "text/javascript"),
		ct == "application/javascript", strings.HasPrefix(ct, "image/svg+xml"):
		return true
	}
	return false
}
