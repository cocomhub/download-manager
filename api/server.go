// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/manager"
	"github.com/cocomhub/download-manager/pkg/logutil"
	"github.com/cocomhub/download-manager/task"
	"github.com/cocomhub/download-manager/web"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"
)

type Server struct {
	mgr *manager.Manager
}

func NewServer(mgr *manager.Manager) *Server {
	return &Server{mgr: mgr}
}

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

func (s *Server) Router() *mux.Router {
	r := mux.NewRouter()

	// Global write middleware: blocks non-GET/HEAD requests when writes are disabled.
	// Individual route .Methods() still restrict to specific HTTP methods on top of this.
	r.Use(s.writeMiddleware)

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

func (s *Server) getRuntime(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	cfg := s.mgr.GetConfig()
	if cfg == nil {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"mode": "full",
			"features": map[string]bool{
				"download":  true,
				"scheduler": true,
			},
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"mode": cfg.Runtime.Mode,
		"features": map[string]bool{
			"download":  cfg.Runtime.Download.Enabled,
			"scheduler": cfg.Runtime.Scheduler.Enabled,
		},
	})
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	status := s.mgr.GetHealthStatus()
	if status.Status == "error" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(status)
}

func (s *Server) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	metrics := s.mgr.CollectMetrics()
	json.NewEncoder(w).Encode(metrics)
}

func (s *Server) failuresHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	taskID := r.URL.Query().Get("task_id")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}
	result := s.mgr.GetFailures(taskID, limit)
	json.NewEncoder(w).Encode(result)
}

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	tasks := s.mgr.GetTaskSummaries()
	json.NewEncoder(w).Encode(tasks)
}

func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	page := int64(1)
	limit := int64(50) // Default

	if pStr := r.URL.Query().Get("page"); pStr != "" {
		if p, err := strconv.ParseInt(pStr, 10, 64); err == nil && p > 0 {
			page = p
		}
	}
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if lStr == "all" {
			limit = -1
		} else if l, err := strconv.ParseInt(lStr, 10, 64); err == nil {
			limit = l
		}
	}

	search := r.URL.Query().Get("search")
	sortBy := r.URL.Query().Get("sort")

	details, err := s.mgr.GetTaskDetails(id, page, limit, search, sortBy)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "task_not_found", fmt.Sprintf("Task %s not found: %v", id, err))
		return
	}
	json.NewEncoder(w).Encode(details)
}

type RetryRequest struct {
	URL string `json:"url"` // Optional, if empty retry all failed
}

type ObjectURLRequest struct {
	URL string `json:"url"`
}

type ObjectURLsRequest struct {
	URLs []string `json:"urls"`
}

func (s *Server) cancelTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	if err := s.mgr.CancelTask(id); err != nil {
		writeJSONError(w, http.StatusBadRequest, "cancel_failed", fmt.Sprintf("Failed to cancel task: %v", err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) cancelObject(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	var req ObjectURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "url is required")
		return
	}
	if err := s.mgr.CancelObject(id, req.URL); err != nil {
		writeJSONError(w, http.StatusBadRequest, "cancel_failed", fmt.Sprintf("Failed to cancel object: %v", err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) undoCancelObject(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	var req ObjectURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "url is required")
		return
	}
	if err := s.mgr.UndoCancelObject(id, req.URL); err != nil {
		writeJSONError(w, http.StatusBadRequest, "undo_failed", fmt.Sprintf("Failed to undo cancel: %v", err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) cancelObjectsBatch(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	var req ObjectURLsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.URLs) == 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "urls is required")
		return
	}
	result := make(map[string]string)
	for _, u := range req.URLs {
		if err := s.mgr.CancelObject(id, u); err != nil {
			result[u] = err.Error()
		} else {
			result[u] = "ok"
		}
	}
	json.NewEncoder(w).Encode(result)
}

func (s *Server) undoCancelObjectsBatch(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	var req ObjectURLsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.URLs) == 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "urls is required")
		return
	}
	result := make(map[string]string)
	for _, u := range req.URLs {
		if err := s.mgr.UndoCancelObject(id, u); err != nil {
			result[u] = err.Error()
		} else {
			result[u] = "ok"
		}
	}
	json.NewEncoder(w).Encode(result)
}

func (s *Server) cancelTasksBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.IDs) == 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "ids is required")
		return
	}
	res := s.mgr.CancelTasks(req.IDs)
	json.NewEncoder(w).Encode(res)
}

func (s *Server) retryTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var req RetryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Allow empty body (err) for "retry all failed"
		_ = err
	}

	if req.URL != "" {
		if err := s.mgr.RetryObject(id, req.URL); err != nil {
			writeJSONError(w, http.StatusBadRequest, "retry_failed", fmt.Sprintf("Failed to retry object: %v", err))
			return
		}
	} else {
		if err := s.mgr.RetryAllFailed(id); err != nil {
			writeJSONError(w, http.StatusBadRequest, "retry_failed", fmt.Sprintf("Failed to retry all failed objects: %v", err))
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

type TaskConfigRequest struct {
	Concurrency     *int   `json:"concurrency"`
	RefreshInterval *int   `json:"refresh_interval"`
	AuditAuthor     string `json:"audit_author"`
	AuditMessage    string `json:"audit_message"`
	AuditSource     string `json:"audit_source"`
}

func (s *Server) updateTaskConfig(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	var req TaskConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("Invalid request body: %v", err))
		return
	}
	audit := &manager.AuditInfo{
		Author:  coalesce(req.AuditAuthor, "ui"),
		Source:  coalesce(req.AuditSource, "api/tasks/config"),
		Message: coalesce(req.AuditMessage, ""),
	}
	if audit.Message == "" {
		if req.Concurrency != nil && req.RefreshInterval != nil {
			audit.Message = fmt.Sprintf("task %s runtime: concurrency=%d, refresh_interval=%d", id, *req.Concurrency, *req.RefreshInterval)
		} else if req.Concurrency != nil {
			audit.Message = fmt.Sprintf("task %s runtime: concurrency=%d", id, *req.Concurrency)
		} else if req.RefreshInterval != nil {
			audit.Message = fmt.Sprintf("task %s runtime: refresh_interval=%d", id, *req.RefreshInterval)
		}
	}
	applied, err := s.mgr.SetTaskConfig(id, req.Concurrency, req.RefreshInterval, audit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "update_failed", fmt.Sprintf("Failed to update task config: %v", err))
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"applied": applied,
	})
}

func (s *Server) patchTaskRuntime(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	var req TaskConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("Invalid request body: %v", err))
		return
	}
	audit := &manager.AuditInfo{
		Author:  coalesce(req.AuditAuthor, "ui"),
		Source:  coalesce(req.AuditSource, "api/tasks/runtime"),
		Message: coalesce(req.AuditMessage, ""),
	}
	applied, err := s.mgr.SetTaskConfig(id, req.Concurrency, req.RefreshInterval, audit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "update_failed", fmt.Sprintf("Failed to update task runtime: %v", err))
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"applied": applied,
	})
}

type ReorderRequest struct {
	URL      string `json:"url"`
	NewIndex int    `json:"new_index"`
}

func (s *Server) reorderTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var req ReorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	if req.URL == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "URL is required")
		return
	}

	if err := s.mgr.ReorderObject(id, req.URL, req.NewIndex); err != nil {
		writeJSONError(w, http.StatusBadRequest, "reorder_failed", fmt.Sprintf("Failed to reorder object: %v", err))
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) listConfigHistory(w http.ResponseWriter, r *http.Request) {
	h, _ := s.mgr.ListConfigBackups()
	json.NewEncoder(w).Encode(h)
}

type RollbackRequest struct {
	Filename     string `json:"filename"`
	AuditAuthor  string `json:"audit_author"`
	AuditSource  string `json:"audit_source"`
	AuditMessage string `json:"audit_message"`
}

func (s *Server) rollbackConfig(w http.ResponseWriter, r *http.Request) {
	var req RollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Filename == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("Invalid request body: %v", err))
		return
	}
	if err := s.mgr.RollbackConfig(req.Filename, &manager.AuditInfo{
		Author:  coalesce(req.AuditAuthor, "ui"),
		Source:  coalesce(req.AuditSource, "api/config/rollback"),
		Message: coalesce(req.AuditMessage, fmt.Sprintf("rollback to %s", req.Filename)),
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "rollback_failed", fmt.Sprintf("Failed to rollback config: %v", err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) deleteConfigBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Filename == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("Invalid request body: %v", err))
		return
	}
	if err := s.mgr.DeleteConfigBackup(req.Filename); err != nil {
		writeJSONError(w, http.StatusBadRequest, "delete_failed", fmt.Sprintf("Failed to delete backup: %v", err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) diffConfig(w http.ResponseWriter, r *http.Request) {
	left := r.URL.Query().Get("left")
	right := r.URL.Query().Get("right")
	ignoreWS := r.URL.Query().Get("ignore_ws") == "1"
	ignoreComments := r.URL.Query().Get("ignore_comments") == "1"
	res, err := s.mgr.DiffConfigFilesOpts(left, right, ignoreWS, ignoreComments)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "diff_failed", fmt.Sprintf("Failed to diff config files: %v", err))
		return
	}
	json.NewEncoder(w).Encode(res)
}

func (s *Server) applyConfigYAML(w http.ResponseWriter, r *http.Request) {
	var req struct {
		YAML         string `json:"yaml"`
		AuditAuthor  string `json:"audit_author"`
		AuditSource  string `json:"audit_source"`
		AuditMessage string `json:"audit_message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.YAML) == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}
	var cfg config.Config
	if err := yaml.Unmarshal([]byte(req.YAML), &cfg); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_yaml", fmt.Sprintf("YAML parse error: %v", err))
		return
	}
	cfg.ValidateAndClamp()
	if err := s.mgr.UpdateConfig(&cfg, &manager.AuditInfo{
		Author:  coalesce(req.AuditAuthor, "ui"),
		Source:  coalesce(req.AuditSource, "api/config/apply"),
		Message: coalesce(req.AuditMessage, "apply YAML"),
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "update_failed", fmt.Sprintf("Failed to apply config: %v", err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) addConfigTag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filename string `json:"filename"`
		Tag      string `json:"tag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Filename == "" || req.Tag == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("Invalid request body: %v", err))
		return
	}
	if err := s.mgr.AddConfigTag(req.Filename, req.Tag); err != nil {
		writeJSONError(w, http.StatusBadRequest, "tag_failed", fmt.Sprintf("Failed to add tag: %v", err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) addConfigNote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filename string `json:"filename"`
		Message  string `json:"message"`
		Author   string `json:"author"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Filename == "" || req.Message == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("Invalid request body: %v", err))
		return
	}
	if err := s.mgr.AddConfigNote(req.Filename, req.Message, req.Author); err != nil {
		writeJSONError(w, http.StatusBadRequest, "note_failed", fmt.Sprintf("Failed to add note: %v", err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func writeJSONError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   code,
		"message": msg,
	})
}

func coalesce(s string, def string) string {
	if s != "" {
		return s
	}
	return def
}

// Persistent Task Management (create/update via config)
func (s *Server) createTaskPersistent(w http.ResponseWriter, r *http.Request) {
	var t config.Task
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("Invalid request body: %v", err))
		return
	}
	if t.ID == "" || t.Type == "" {
		writeJSONError(w, http.StatusBadRequest, "missing_id_type", "id and type are required")
		return
	}
	cur := s.mgr.GetConfig()
	// prevent duplicate
	for _, existing := range cur.Tasks {
		if existing.ID == t.ID {
			writeJSONError(w, http.StatusConflict, "duplicate_id", fmt.Sprintf("task id %s already exists", t.ID))
			return
		}
	}
	cur.Tasks = append(cur.Tasks, t)
	if err := s.mgr.UpdateConfig(cur, &manager.AuditInfo{
		Author:  "ui",
		Source:  "api/tasks/post",
		Message: fmt.Sprintf("task %s created", t.ID),
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "create_failed", fmt.Sprintf("Failed to create task %s: %v", t.ID, err))
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) updateTaskPersistent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "missing_id", "missing id")
		return
	}
	var t config.Task
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("Invalid request body: %v", err))
		return
	}
	cur := s.mgr.GetConfig()
	found := false
	for i := range cur.Tasks {
		if cur.Tasks[i].ID == id {
			cur.Tasks[i] = t
			cur.Tasks[i].ID = id
			found = true
			break
		}
	}
	if !found {
		writeJSONError(w, http.StatusNotFound, "not_found", fmt.Sprintf("task %s not found", id))
		return
	}
	if err := s.mgr.UpdateConfig(cur, &manager.AuditInfo{
		Author:  "ui",
		Source:  "api/tasks/put",
		Message: fmt.Sprintf("task %s updated", id),
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "update_failed", fmt.Sprintf("Failed to update task %s: %v", id, err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) getServerConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.mgr.GetConfig()
	resp := map[string]any{
		"task_scan":  cfg.TaskScan,
		"downloader": cfg.Downloader,
		"ui_defaults": map[string]any{
			"default_save_dir":    cfg.Server.UIDefaults.DefaultSaveDir,
			"window_width":        cfg.Server.UIDefaults.WindowWidth,
			"window_height":       cfg.Server.UIDefaults.WindowHeight,
			"diff_side_by_side":   cfg.Server.UIDefaults.DiffSideBySide,
			"diff_ignore_ws":      cfg.Server.UIDefaults.DiffIgnoreWS,
			"diff_ignore_comment": cfg.Server.UIDefaults.DiffIgnoreComment,
			"status_style":        cfg.Server.UIDefaults.StatusStyle,
		},
	}
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) updateServerConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskScan   config.TaskScan   `json:"task_scan"`
		Downloader config.Downloader `json:"downloader"`
		UIDefaults config.UIDefaults `json:"ui_defaults"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("Invalid request body: %v", err))
		return
	}
	cur := s.mgr.GetConfig()
	cur.TaskScan = req.TaskScan
	// Override whole Downloader (new sub-structures included)
	cur.Downloader.Type = req.Downloader.Type
	if req.Downloader.GlobalConcurrent > 0 {
		cur.Downloader.GlobalConcurrent = req.Downloader.GlobalConcurrent
	}
	if req.Downloader.MaxRetries > 0 {
		cur.Downloader.MaxRetries = req.Downloader.MaxRetries
	}
	cur.Downloader.ForceProxy = req.Downloader.ForceProxy
	cur.Downloader.Proxies = req.Downloader.Proxies
	cur.Downloader.DomainLimits = req.Downloader.DomainLimits
	// New sub-structures
	cur.Downloader.Filesystem = req.Downloader.Filesystem
	if req.Downloader.HTTP.TimeoutSeconds > 0 {
		cur.Downloader.HTTP = req.Downloader.HTTP
	}
	cur.Downloader.Proxy = req.Downloader.Proxy
	cur.Downloader.Progress = req.Downloader.Progress
	cur.Downloader.FFmpeg = req.Downloader.FFmpeg
	cur.Server.UIDefaults = req.UIDefaults
	if err := s.mgr.UpdateConfig(cur, &manager.AuditInfo{
		Author:  "ui",
		Source:  "api/config/server",
		Message: "server config updated",
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "update_failed", fmt.Sprintf("Failed to update server config: %v", err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) getLogConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.mgr.GetConfig()
	json.NewEncoder(w).Encode(cfg.Log)
}

func (s *Server) updateLogConfig(w http.ResponseWriter, r *http.Request) {
	var newLog logutil.LogConfig
	if err := json.NewDecoder(r.Body).Decode(&newLog); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("Invalid request body: %v", err))
		return
	}
	if err := s.mgr.UpdateLogConfig(newLog); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "update_failed", fmt.Sprintf("Failed to update log config: %v", err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) getActiveDownloads(w http.ResponseWriter, r *http.Request) {
	actives := s.mgr.GetActiveDownloads()
	json.NewEncoder(w).Encode(actives)
}

func (s *Server) aggregateObjects(w http.ResponseWriter, r *http.Request) {
	page := int64(1)
	limit := int64(50)
	if pStr := r.URL.Query().Get("page"); pStr != "" {
		if p, err := strconv.ParseInt(pStr, 10, 64); err == nil && p > 0 {
			page = p
		}
	}
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if lStr == "all" {
			limit = -1
		} else if l, err := strconv.ParseInt(lStr, 10, 64); err == nil {
			limit = l
		}
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	sortBy := r.URL.Query().Get("sort")
	status := r.URL.Query().Get("status")
	groupBy := r.URL.Query().Get("group_by")
	typesParam := strings.TrimSpace(r.URL.Query().Get("types"))
	var types []string
	if typesParam != "" {
		for t := range strings.SplitSeq(typesParam, ",") {
			t = task.NormalizeType(t)
			if t != "" {
				types = append(types, t)
			}
		}
	}
	var (
		res map[string]any
		err error
	)
	if groupBy == "content" {
		res, err = s.mgr.AggregateByContent(page, limit, search, sortBy, status, types)
	} else {
		res, err = s.mgr.AggregateObjects(page, limit, search, sortBy, status, types)
	}
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "aggregate_failed", fmt.Sprintf("Failed to aggregate objects: %v", err))
		return
	}
	json.NewEncoder(w).Encode(res)
}

func (s *Server) getGroupObjects(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	group := vars["group"]
	taskID := strings.TrimSpace(r.URL.Query().Get("task_id"))
	taskType := strings.TrimSpace(r.URL.Query().Get("task_type"))
	if taskID == "" || taskType == "" {
		writeJSONError(w, http.StatusBadRequest, "missing_scope", "task_id and task_type are required")
		return
	}
	list := s.mgr.GetObjectsByScopedGroup(taskID, taskType, group)
	json.NewEncoder(w).Encode(map[string]any{
		"task_id":   taskID,
		"task_type": taskType,
		"group":     group,
		"objects":   list,
		"total":     len(list),
	})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Flush immediately to establish connection
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, "streaming_not_supported", "Streaming not supported")
		return
	}
	flusher.Flush()

	// Subscribe to manager events
	eventChan := s.mgr.Subscribe()
	defer s.mgr.Unsubscribe(eventChan)

	// Listen for client disconnect
	notify := r.Context().Done()

	for {
		select {
		case <-notify:
			return
		case event := <-eventChan:
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			// SSE format: "data: ...\n\n"
			// Using "event" field for type if needed, but we put type in payload
			// Let's just send data
			w.Write([]byte("data: "))
			w.Write(data)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}
