// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"mime"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cocomhub/download-manager/core"
	"github.com/gorilla/mux"
)

// serveUITypes returns the list of task types that have registered UI assets.
func (s *Server) serveUITypes(w http.ResponseWriter, _ *http.Request) {
	types := core.ListRegisteredUI()
	w.Header().Set(hdrContentType, "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(types)
}

// serveUIConfig returns the JS/CSS paths and label for a task type's UI assets.
func (s *Server) serveUIConfig(w http.ResponseWriter, r *http.Request) {
	taskType := mux.Vars(r)["type"]
	assets, ok := core.GetTaskUI(taskType)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "not_found", "no UI assets for type: "+taskType)
		return
	}
	w.Header().Set(hdrContentType, "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"js":    assets.JSPaths,
		"css":   assets.CSSPaths,
		"label": assets.Label,
	})
}

// serveUIAsset serves a single UI asset file (JS/CSS) from a task type's embedded FS.
func (s *Server) serveUIAsset(w http.ResponseWriter, r *http.Request) {
	taskType := mux.Vars(r)["type"]
	assetPath := mux.Vars(r)["path"]

	assets, ok := core.GetTaskUI(taskType)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "not_found", "no UI assets for type: "+taskType)
		return
	}

	// Security: prevent directory traversal - only allow exact matches from registered paths
	allowed := append(assets.JSPaths, assets.CSSPaths...)
	valid := slices.Contains(allowed, assetPath)
	if !valid {
		writeJSONError(w, http.StatusForbidden, "forbidden", "asset not in allowed list: "+assetPath)
		return
	}

	data, err := assets.FS.ReadFile(assetPath)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "not_found", "asset not found: "+assetPath)
		return
	}

	// Set Content-Type based on extension
	ext := strings.ToLower(filepath.Ext(assetPath))
	switch ext {
	case ".js":
		w.Header().Set(hdrContentType, "application/javascript; charset=utf-8")
	case ".css":
		w.Header().Set(hdrContentType, "text/css; charset=utf-8")
	case ".html":
		w.Header().Set(hdrContentType, "text/html; charset=utf-8")
	default:
		if ct := mime.TypeByExtension(ext); ct != "" {
			w.Header().Set(hdrContentType, ct)
		}
	}

	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
