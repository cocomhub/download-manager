// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/cocomhub/download-manager/remotesave"
	"github.com/gorilla/mux"
)

// triggerRemoteSave 手动触发任务远程保存（POST /api/tasks/{id}/remote_save）。
// body: {"force": true, "url": "..."}（url 为空 = 全任务；可空）。
func (s *Server) triggerRemoteSave(w http.ResponseWriter, r *http.Request) {
	taskID := mux.Vars(r)["id"]
	var req struct {
		Force bool   `json:"force"`
		URL   string `json:"url"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, "invalid body")
			return
		}
	}
	var err error
	if req.URL != "" {
		err = s.mgr.TriggerRemoteSaveObject(taskID, req.URL, req.Force)
	} else {
		err = s.mgr.TriggerRemoteSave(taskID, req.Force)
	}
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "remote_save_trigger_failed", err.Error())
		return
	}
	w.Header().Set(hdrContentType, "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "task_id": taskID, "url": req.URL, "force": req.Force})
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

// getRemoteSaveMetrics 返回任务级 remote-save 指标。
func (s *Server) getRemoteSaveMetrics(w http.ResponseWriter, r *http.Request) {
	taskID := mux.Vars(r)["id"]
	out, err := s.mgr.RemoteSaveMetrics(taskID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "remote_save_metrics_failed", err.Error())
		return
	}
	w.Header().Set(hdrContentType, "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"task_id": taskID, "metrics": out})
}

// getRemoteSaveConflicts 返回逐文件冲突列表（GET /api/tasks/{id}/remote_save/conflicts）。
func (s *Server) getRemoteSaveConflicts(w http.ResponseWriter, r *http.Request) {
	taskID := mux.Vars(r)["id"]
	out, err := s.mgr.RemoteSaveConflicts(taskID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "remote_save_not_found", err.Error())
		return
	}
	total := len(out)
	start, end := remoteSavePageBounds(r, total)
	w.Header().Set(hdrContentType, "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"task_id": taskID, "conflicts": out[start:end], "total": total, "offset": start, "limit": end - start,
	})
}

// getRemoteSaveObjects 返回任务下对象级远程保存状态摘要。
func (s *Server) getRemoteSaveObjects(w http.ResponseWriter, r *http.Request) {
	taskID := mux.Vars(r)["id"]
	out, err := s.mgr.RemoteSaveObjects(taskID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "remote_save_not_found", err.Error())
		return
	}
	total := len(out)
	start, end := remoteSavePageBounds(r, total)
	w.Header().Set(hdrContentType, "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"task_id": taskID, "objects": out[start:end], "total": total, "offset": start, "limit": end - start,
	})
}

// getRemoteSaveMigration 返回对象目录迁移报告（只读，不移动文件）。
func (s *Server) getRemoteSaveMigration(w http.ResponseWriter, r *http.Request) {
	taskID := mux.Vars(r)["id"]
	out, err := s.mgr.RemoteSaveMigrationReport(taskID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "remote_save_migration_failed", err.Error())
		return
	}
	total := len(out)
	start, end := remoteSavePageBounds(r, total)
	w.Header().Set(hdrContentType, "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"task_id": taskID, "items": out[start:end], "total": total, "offset": start, "limit": end - start,
	})
}

// applyRemoteSaveMigration 执行对象目录迁移（远端 rename，不删除）。
func (s *Server) applyRemoteSaveMigration(w http.ResponseWriter, r *http.Request) {
	taskID := mux.Vars(r)["id"]
	limit := 0
	if r.Body != nil && r.ContentLength != 0 {
		var req struct {
			Limit int `json:"limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			limit = req.Limit
		}
	}
	out, err := s.mgr.ApplyRemoteSaveMigration(r.Context(), taskID, limit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "remote_save_migration_failed", err.Error())
		return
	}
	succeeded := 0
	for _, item := range out {
		if item.Success {
			succeeded++
		}
	}
	w.Header().Set(hdrContentType, "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"task_id": taskID, "results": out, "succeeded": succeeded, "failed": len(out) - succeeded,
	})
}

func remoteSavePageBounds(r *http.Request, total int) (int, int) {
	if total == 0 {
		return 0, 0
	}
	offset := 0
	limit := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			offset = n
		}
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if offset >= total {
		return total, total
	}
	if limit == 0 || offset+limit > total {
		return offset, total
	}
	return offset, offset + limit
}

// getRemoteSaveMigrationAudit 返回最近迁移审计记录。
func (s *Server) getRemoteSaveMigrationAudit(w http.ResponseWriter, r *http.Request) {
	taskID := mux.Vars(r)["id"]
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	out, err := s.mgr.RemoteSaveMigrationAudit(taskID, limit)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "remote_save_migration_audit_failed", err.Error())
		return
	}
	w.Header().Set(hdrContentType, "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"task_id": taskID, "entries": out, "total": len(out)})
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
	if err := s.mgr.ResolveRemoteSaveConflict(r.Context(), taskID, req.ObjectURL, req.Path, req.Action); err != nil {
		writeJSONError(w, http.StatusBadRequest, "remote_save_resolve_failed", err.Error())
		return
	}
	w.Header().Set(hdrContentType, "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

// resolveRemoteSaveConflicts 批量处理同一动作（POST /api/tasks/{id}/remote_save/conflicts/resolve_batch）。
func (s *Server) resolveRemoteSaveConflicts(w http.ResponseWriter, r *http.Request) {
	taskID := mux.Vars(r)["id"]
	var req struct {
		Action string                          `json:"action"`
		Items  []remotesave.ConflictActionItem `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, "invalid body")
		return
	}
	if req.Action == "" {
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, "action 必填")
		return
	}
	results, err := s.mgr.ResolveRemoteSaveConflicts(r.Context(), taskID, req.Action, req.Items)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "remote_save_resolve_failed", err.Error())
		return
	}
	succeeded := 0
	for _, res := range results {
		if res.Success {
			succeeded++
		}
	}
	w.Header().Set(hdrContentType, "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true, "succeeded": succeeded, "failed": len(results) - succeeded, "results": results,
	})
}

// getRemotePlayURL 生成/刷新对象签名播放 URL（GET /api/tasks/{id}/object/play_url?url=...）。
func (s *Server) getRemotePlayURL(w http.ResponseWriter, r *http.Request) {
	taskID := mux.Vars(r)["id"]
	objectURL := r.URL.Query().Get("url")
	if objectURL == "" {
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, "url 必填")
		return
	}
	play, exp, err := s.mgr.PlayRemoteURL(taskID, objectURL)
	if err != nil {
		writeJSONError(w, http.StatusConflict, "remote_play_unavailable", err.Error())
		return
	}
	w.Header().Set(hdrContentType, "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"url": play, "expires_at": exp})
}
