// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/manager"
	"github.com/cocomhub/download-manager/pkg/logutil"
	"gopkg.in/yaml.v3"
)

// getServerConfig returns the server configuration.
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

// updateServerConfig updates the server configuration.
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
	// Deep-copy before mutation to avoid data race on shared config
	cc := cur.Clone()
	cc.TaskScan = req.TaskScan
	// Override whole Downloader (new sub-structures included)
	cc.Downloader.Type = req.Downloader.Type
	if req.Downloader.GlobalConcurrent > 0 {
		cc.Downloader.GlobalConcurrent = req.Downloader.GlobalConcurrent
	}
	if req.Downloader.MaxRetries > 0 {
		cc.Downloader.MaxRetries = req.Downloader.MaxRetries
	}
	cc.Downloader.ForceProxy = req.Downloader.ForceProxy
	cc.Downloader.Proxies = req.Downloader.Proxies
	cc.Downloader.DomainLimits = req.Downloader.DomainLimits
	// New sub-structures
	cc.Downloader.Filesystem = req.Downloader.Filesystem
	if req.Downloader.HTTP.TimeoutSeconds > 0 {
		cc.Downloader.HTTP = req.Downloader.HTTP
	}
	cc.Downloader.Proxy = req.Downloader.Proxy
	cc.Downloader.Progress = req.Downloader.Progress
	cc.Downloader.FFmpeg = req.Downloader.FFmpeg
	cc.Server.UIDefaults = req.UIDefaults
	if err := s.mgr.UpdateConfig(cc, &manager.AuditInfo{
		Author:  "ui",
		Source:  "api/config/server",
		Message: "server config updated",
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "update_failed", fmt.Sprintf("Failed to update server config: %v", err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

// getLogConfig returns the log configuration.
func (s *Server) getLogConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.mgr.GetConfig()
	json.NewEncoder(w).Encode(cfg.Log)
}

// updateLogConfig updates the log configuration.
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

// listConfigHistory returns the list of configuration backups.
func (s *Server) listConfigHistory(w http.ResponseWriter, r *http.Request) {
	h, _ := s.mgr.ListConfigBackups()
	json.NewEncoder(w).Encode(h)
}

// rollbackConfig rolls back the configuration to a previous backup.
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

// RollbackRequest is the request body for configuration rollback.
type RollbackRequest struct {
	Filename     string `json:"filename"`
	AuditAuthor  string `json:"audit_author"`
	AuditSource  string `json:"audit_source"`
	AuditMessage string `json:"audit_message"`
}

// diffConfig computes a diff between two configuration files.
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

// addConfigTag adds a tag to a configuration backup.
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

// addConfigNote adds a note to a configuration backup.
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

// deleteConfigBackup deletes a configuration backup.
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

// applyConfigYAML applies a YAML configuration to the manager.
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
