// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
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
