// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/manager"
	"github.com/cocomhub/download-manager/web"

	"github.com/gorilla/mux"
)

// Server wraps a manager.Manager and exposes HTTP API routes.
type Server struct {
	mgr *manager.Manager
}

// NewServer creates a new API server wrapping the given manager.
func NewServer(mgr *manager.Manager) *Server {
	return &Server{mgr: mgr}
}

// writeDisabled returns true when write operations are disabled.
func (s *Server) writeDisabled() bool {
	cfg := s.mgr.GetConfig()
	if cfg != nil {
		if cfg.Runtime.Mode == config.RunModeUI {
			return true
		}
		// RunModeFull: blocked only when both features are disabled
		return !cfg.Runtime.Download.Enabled && !cfg.Runtime.Scheduler.Enabled
	}
	st := s.mgr.FeaturesStatus()
	return !st.Scheduler && !st.Workers
}

// writeMiddleware is a global middleware that blocks non-GET/HEAD requests
// when write operations are disabled (UI-only mode or both features disabled).
func (s *Server) writeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		if s.writeDisabled() {
			writeJSONError(w, http.StatusMethodNotAllowed, "write_disabled",
				"write operations are disabled in the current mode")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Router returns the configured mux.Router with all API routes registered.
func (s *Server) Router() *mux.Router {
	r := mux.NewRouter()

	// Global write middleware: blocks non-GET/HEAD requests when writes are disabled.
	// Individual route .Methods() still restrict to specific HTTP methods on top of this.
	r.Use(s.writeMiddleware)

	// Custom NotFoundHandler: return JSON for unmatched routes.
	r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSONError(w, http.StatusNotFound, "not_found", "page not found")
	})
	// Custom MethodNotAllowedHandler: return JSON for method mismatches.
	r.MethodNotAllowedHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})

	// API Routes
	r.HandleFunc("/api/runtime", s.getRuntime).Methods("GET")
	r.HandleFunc("/api/healthz", s.healthHandler).Methods("GET")
	r.HandleFunc("/api/tasks", s.listTasks).Methods("GET")
	r.HandleFunc("/api/tasks", s.createTaskPersistent).Methods("POST")
	r.HandleFunc("/api/tasks/{id}", s.getTask).Methods("GET")
	r.HandleFunc("/api/groups/{group}/objects", s.getGroupObjects).Methods("GET")
	r.HandleFunc("/api/tasks/{id}", s.updateTaskPersistent).Methods("PUT")
	r.HandleFunc("/api/tasks/{id}/retry", s.retryTask).Methods("POST")
	r.HandleFunc("/api/tasks/{id}/cancel", s.cancelTask).Methods("POST")
	r.HandleFunc("/api/tasks/cancel_batch", s.cancelTasksBatch).Methods("POST")
	r.HandleFunc("/api/tasks/{id}/object/cancel", s.cancelObject).Methods("POST")
	r.HandleFunc("/api/tasks/{id}/object/undo_cancel", s.undoCancelObject).Methods("POST")
	r.HandleFunc("/api/tasks/{id}/object/cancel_batch", s.cancelObjectsBatch).Methods("POST")
	r.HandleFunc("/api/tasks/{id}/object/undo_cancel_batch", s.undoCancelObjectsBatch).Methods("POST")
	r.HandleFunc("/api/tasks/{id}/reorder", s.reorderTask).Methods("POST")
	r.HandleFunc("/api/tasks/{id}/config", s.updateTaskConfig).Methods("POST")
	r.HandleFunc("/api/tasks/{id}/runtime", s.patchTaskRuntime).Methods("PATCH")
	r.HandleFunc("/api/config/server", s.getServerConfig).Methods("GET")
	r.HandleFunc("/api/config/server", s.updateServerConfig).Methods("POST")
	r.HandleFunc("/api/config/log", s.getLogConfig).Methods("GET")
	r.HandleFunc("/api/config/log", s.updateLogConfig).Methods("POST")
	r.HandleFunc("/api/config/history", s.listConfigHistory).Methods("GET")
	r.HandleFunc("/api/config/rollback", s.rollbackConfig).Methods("POST")
	r.HandleFunc("/api/config/diff", s.diffConfig).Methods("GET")
	r.HandleFunc("/api/config/tag", s.addConfigTag).Methods("POST")
	r.HandleFunc("/api/config/note", s.addConfigNote).Methods("POST")
	r.HandleFunc("/api/config/delete", s.deleteConfigBackup).Methods("POST")
	r.HandleFunc("/api/config/apply", s.applyConfigYAML).Methods("POST")
	r.HandleFunc("/api/downloads", s.getActiveDownloads).Methods("GET")
	r.HandleFunc("/api/aggregate", s.aggregateObjects).Methods("GET")
	r.HandleFunc("/api/events", s.handleEvents).Methods("GET")
	r.HandleFunc("/api/metrics", s.metricsHandler).Methods("GET")
	r.HandleFunc("/api/metrics/failures", s.failuresHandler).Methods("GET")

	// File Preview Route
	// Assuming files are in build/test/downloads based on recent config changes
	// In a real app, this path should be configurable or dynamic per task
	r.PathPrefix("/files/").Handler(http.StripPrefix("/files/", http.FileServer(http.Dir(s.mgr.GetDownloadRootDir()))))

	// Static UI
	subFS, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		log.Fatal("Failed to embed static files:", err)
	}
	r.PathPrefix("/").Handler(http.FileServer(http.FS(subFS)))

	return r
}

// writeJSONError writes a JSON error response with the given status, code, and message.
func writeJSONError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   code,
		"message": msg,
	})
}

// coalesce returns s if non-empty, otherwise returns def.
func coalesce(s string, def string) string {
	if s != "" {
		return s
	}
	return def
}
