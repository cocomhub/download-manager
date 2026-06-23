// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/manager"
	"github.com/gorilla/mux"
)

// getRuntime returns the current runtime mode and feature status.
func (s *Server) getRuntime(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(hdrContentType, "application/json")
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

// healthHandler returns the health status of the manager.
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(hdrContentType, "application/json")
	status := s.mgr.GetHealthStatus()
	if status.Status == "error" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(status)
}

// listTasks returns a summary of all tasks.
func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	tasks := s.mgr.GetTaskSummaries()
	json.NewEncoder(w).Encode(tasks)
}

// getTask returns detailed information about a specific task.
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

// cancelTask cancels a task by ID.
func (s *Server) cancelTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	if err := s.mgr.CancelTask(id); err != nil {
		writeJSONError(w, http.StatusBadRequest, "cancel_failed", fmt.Sprintf("Failed to cancel task: %v", err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

// cancelObject cancels a specific download object within a task.
func (s *Server) cancelObject(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	var req ObjectURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, "url is required")
		return
	}
	if err := s.mgr.CancelObject(id, req.URL); err != nil {
		writeJSONError(w, http.StatusBadRequest, "cancel_failed", fmt.Sprintf("Failed to cancel object: %v", err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

// undoCancelObject undoes the cancellation of a specific download object.
func (s *Server) undoCancelObject(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	var req ObjectURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, "url is required")
		return
	}
	if err := s.mgr.UndoCancelObject(id, req.URL); err != nil {
		writeJSONError(w, http.StatusBadRequest, "undo_failed", fmt.Sprintf("Failed to undo cancel: %v", err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

// cancelObjectsBatch cancels multiple objects in a task by their URLs.
func (s *Server) cancelObjectsBatch(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	var req ObjectURLsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.URLs) == 0 {
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, "urls is required")
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

// undoCancelObjectsBatch undoes cancellation for multiple objects in a task.
func (s *Server) undoCancelObjectsBatch(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	var req ObjectURLsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.URLs) == 0 {
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, "urls is required")
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

// cancelTasksBatch cancels multiple tasks by their IDs.
func (s *Server) cancelTasksBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.IDs) == 0 {
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, "ids is required")
		return
	}
	res := s.mgr.CancelTasks(req.IDs)
	json.NewEncoder(w).Encode(res)
}

// retryTask retries a failed object, or all failed objects, for a task.
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

// updateTaskConfig updates concurrency and refresh interval for a task.
func (s *Server) updateTaskConfig(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	var req TaskConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, fmt.Sprintf(errFmtInvalidBody, err))
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
		writeJSONError(w, http.StatusBadRequest, errCodeUpdateFailed, fmt.Sprintf("Failed to update task config: %v", err))
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"applied": applied,
	})
}

// patchTaskRuntime patches runtime configuration (concurrency/refresh) for a task.
func (s *Server) patchTaskRuntime(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	var req TaskConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, fmt.Sprintf(errFmtInvalidBody, err))
		return
	}
	audit := &manager.AuditInfo{
		Author:  coalesce(req.AuditAuthor, "ui"),
		Source:  coalesce(req.AuditSource, "api/tasks/runtime"),
		Message: coalesce(req.AuditMessage, ""),
	}
	applied, err := s.mgr.SetTaskConfig(id, req.Concurrency, req.RefreshInterval, audit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, errCodeUpdateFailed, fmt.Sprintf("Failed to update task runtime: %v", err))
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

// reorderTask reorders a download object within a task.
func (s *Server) reorderTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var req ReorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, fmt.Sprintf(errFmtInvalidBody, err))
		return
	}

	if req.URL == "" {
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, "URL is required")
		return
	}

	if err := s.mgr.ReorderObject(id, req.URL, req.NewIndex); err != nil {
		writeJSONError(w, http.StatusBadRequest, "reorder_failed", fmt.Sprintf("Failed to reorder object: %v", err))
		return
	}

	w.WriteHeader(http.StatusOK)
}

// createTaskPersistent creates a new task via config update (POST /api/tasks).
func (s *Server) createTaskPersistent(w http.ResponseWriter, r *http.Request) {
	var t config.Task
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, fmt.Sprintf(errFmtInvalidBody, err))
		return
	}
	if t.ID == "" || t.Type == "" {
		writeJSONError(w, http.StatusBadRequest, "missing_id_type", "id and type are required")
		return
	}
	cur := s.mgr.GetConfig()
	// Deep-copy before mutation to avoid data race on shared config
	cc := cur.Clone()
	// prevent duplicate
	for _, existing := range cc.Tasks {
		if existing.ID == t.ID {
			writeJSONError(w, http.StatusConflict, "duplicate_id", fmt.Sprintf("task id %s already exists", t.ID))
			return
		}
	}
	cc.Tasks = append(cc.Tasks, t)
	if err := s.mgr.UpdateConfig(cc, &manager.AuditInfo{
		Author:  "ui",
		Source:  "api/tasks/post",
		Message: fmt.Sprintf("task %s created", t.ID),
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "create_failed", fmt.Sprintf("Failed to create task %s: %v", t.ID, err))
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// updateTaskPersistent updates an existing task via config update (PUT /api/tasks/{id}).
func (s *Server) updateTaskPersistent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "missing_id", "missing id")
		return
	}
	var t config.Task
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, fmt.Sprintf(errFmtInvalidBody, err))
		return
	}
	cur := s.mgr.GetConfig()
	// Deep-copy before mutation to avoid data race on shared config
	cc := cur.Clone()
	found := false
	for i := range cc.Tasks {
		if cc.Tasks[i].ID == id {
			cc.Tasks[i] = t
			cc.Tasks[i].ID = id
			found = true
			break
		}
	}
	if !found {
		writeJSONError(w, http.StatusNotFound, "not_found", fmt.Sprintf("task %s not found", id))
		return
	}
	if err := s.mgr.UpdateConfig(cc, &manager.AuditInfo{
		Author:  "ui",
		Source:  "api/tasks/put",
		Message: fmt.Sprintf("task %s updated", id),
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, errCodeUpdateFailed, fmt.Sprintf("Failed to update task %s: %v", id, err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

// getActiveDownloads returns the list of active downloads.
func (s *Server) getActiveDownloads(w http.ResponseWriter, r *http.Request) {
	actives := s.mgr.GetActiveDownloads()
	json.NewEncoder(w).Encode(actives)
}

// getGroupObjects returns download objects for a specific content group.
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

// handleEvents provides SSE streaming for manager events.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set(hdrCacheControl, hdrNoCache)
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
			w.Write([]byte("data: "))
			w.Write(data)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}
