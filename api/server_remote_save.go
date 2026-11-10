// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

// triggerRemoteSave 手动触发任务远程保存（POST /api/tasks/{id}/remote_save）。
// body: {"force": true}（可空；force 默认 false）。
func (s *Server) triggerRemoteSave(w http.ResponseWriter, r *http.Request) {
	taskID := mux.Vars(r)["id"]
	var req struct {
		Force bool `json:"force"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, "invalid body")
			return
		}
	}
	if err := s.mgr.TriggerRemoteSave(taskID, req.Force); err != nil {
		writeJSONError(w, http.StatusBadRequest, "remote_save_trigger_failed", err.Error())
		return
	}
	w.Header().Set(hdrContentType, "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "task_id": taskID, "force": req.Force})
}

// getRemoteSaveStatus 返回任务远程保存聚合状态（GET /api/tasks/{id}/remote_save）。
func (s *Server) getRemoteSaveStatus(w http.ResponseWriter, r *http.Request) {
	taskID := mux.Vars(r)["id"]
	out, err := s.mgr.RemoteSaveStatus(taskID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "remote_save_not_found", err.Error())
		return
	}
	w.Header().Set(hdrContentType, "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// getRemoteSaveConflicts 返回逐文件冲突列表（GET /api/tasks/{id}/remote_save/conflicts）。
func (s *Server) getRemoteSaveConflicts(w http.ResponseWriter, r *http.Request) {
	taskID := mux.Vars(r)["id"]
	out, err := s.mgr.RemoteSaveConflicts(taskID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "remote_save_not_found", err.Error())
		return
	}
	w.Header().Set(hdrContentType, "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"task_id": taskID, "conflicts": out, "total": len(out)})
}

// resolveRemoteSaveConflict 对单个冲突文件执行人工处理（B 阶段 UI 后端）。
func (s *Server) resolveRemoteSaveConflict(w http.ResponseWriter, r *http.Request) {
	taskID := mux.Vars(r)["id"]
	var req struct {
		ObjectURL string `json:"object_url"`
		Path      string `json:"path"`
		Action    string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, "invalid body")
		return
	}
	if req.ObjectURL == "" || req.Path == "" || req.Action == "" {
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, "object_url/path/action 必填")
		return
	}
	if err := s.mgr.ResolveRemoteSaveConflict(context.Background(), taskID, req.ObjectURL, req.Path, req.Action); err != nil {
		writeJSONError(w, http.StatusBadRequest, "remote_save_resolve_failed", err.Error())
		return
	}
	w.Header().Set(hdrContentType, "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}
