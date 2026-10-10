// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
		"server": map[string]any{
			"http_port":           cfg.Server.HTTPPort,
			"ui_only_port":        cfg.Server.UIOnlyPort,
			"work_dir":            cfg.Server.WorkDir,
			"download_root_dir":   cfg.Server.DownloadRootDir,
			"files_dir":           cfg.Server.FilesDir,
			"files_allow_symlink": cfg.Server.FilesAllowSymlink,
			"auth":                authConfigView(cfg.Server.Auth),
		},
		"task_scan":  cfg.TaskScan,
		"downloader": downloaderConfigView(cfg.Downloader),
		"ui_defaults": map[string]any{
			"default_save_dir":    cfg.Server.UIDefaults.DefaultSaveDir,
			"window_width":        cfg.Server.UIDefaults.WindowWidth,
			"window_height":       cfg.Server.UIDefaults.WindowHeight,
			"diff_side_by_side":   cfg.Server.UIDefaults.DiffSideBySide,
			"diff_ignore_ws":      cfg.Server.UIDefaults.DiffIgnoreWS,
			"diff_ignore_comment": cfg.Server.UIDefaults.DiffIgnoreComment,
			"status_style":        cfg.Server.UIDefaults.StatusStyle,
		},
		"log_level": cfg.Runtime.LogLevel,
	}
	json.NewEncoder(w).Encode(resp)
}

// authConfigView returns the auth config with secrets redacted
// (password/token never sent to the client).
func authConfigView(auth config.AuthConfig) map[string]any {
	return map[string]any{
		"type":         auth.Type,
		"username":     auth.Username,
		"has_password": auth.Password != "",
		"has_token":    auth.Token != "",
	}
}

// downloaderConfigView 返回 downloader 配置的脱敏视图：机密字段（sproxy_cloud 的
// access_key_secret / api_token）只暴露「是否已配置」，不返回明文。
//
// 安全背景（对抗性评审 P0）：downloader 段原为整体 json 序列化，会把 SproxySig
// AccessKeySecret（等价 sproxy 全权凭据）与旧 Bearer token 明文回给任何能访问
// /api/config/server 的客户端（默认 auth 为空 = 不鉴权）。本视图与 authConfigView
// 同思路脱敏。
//
// 回写安全：updateServerConfig 只逐字段更新 downloader（不含 SproxyCloud 段），
// 因此脱敏后的空值不会被 PUT 回写覆盖真实配置。
func downloaderConfigView(dl config.Downloader) map[string]any {
	m := map[string]any{}
	b, err := json.Marshal(dl)
	if err != nil {
		return m
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]any{}
	}
	if sh, ok := m["sproxy_cloud"].(map[string]any); ok {
		redactStringField(sh, "access_key_secret", "has_access_key_secret")
		redactStringField(sh, "api_token", "has_api_token")
	}
	// 代理列表可能内联 http://user:pass@host —— 去掉 userinfo 再返回（预存缺口）
	redactProxyList(m, "proxies")
	if dc, ok := m["dc_proxy"].(map[string]any); ok {
		redactProxyList(dc, "list")
	}
	return m
}

// redactProxyList 去掉 m[key] 列表中每个代理 URL 的 userinfo（user:pass）。
func redactProxyList(m map[string]any, key string) {
	list, ok := m[key].([]any)
	if !ok {
		return
	}
	out := make([]any, 0, len(list))
	for _, raw := range list {
		if s, ok := raw.(string); ok {
			out = append(out, redactURLUserinfo(s))
			continue
		}
		out = append(out, raw)
	}
	m[key] = out
}

// redactURLUserinfo 返回去掉 user:pass 的 URL；解析失败或本无 userinfo 时原样返回。
func redactURLUserinfo(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = nil
	return u.String()
}

// redactStringField 把 m[key] 从明文替换为""，并在非空时置 m[hasFlag]=true。
func redactStringField(m map[string]any, key, hasFlag string) {
	if v, _ := m[key].(string); v != "" {
		m[hasFlag] = true
	}
	m[key] = ""
}

// serverConfigUpdate 是 updateServerConfig 的 server 段请求体。
// 只暴露可安全由 UI 修改的字段（不含 lock_file / scraper_tunnel_key 等敏感项）。
type serverConfigUpdate struct {
	HTTPPort          *int    `json:"http_port"`
	UIOnlyPort        *int    `json:"ui_only_port"`
	WorkDir           *string `json:"work_dir"`
	DownloadRootDir   *string `json:"download_root_dir"`
	FilesDir          *string `json:"files_dir"`
	FilesAllowSymlink *bool   `json:"files_allow_symlink"`
	Auth              *struct {
		Type     string `json:"type"`
		Username string `json:"username"`
		Password string `json:"password"`
		Token    string `json:"token"`
	} `json:"auth"`
}

// updateServerConfig updates the server configuration.
func (s *Server) updateServerConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Server     serverConfigUpdate `json:"server"`
		TaskScan   config.TaskScan    `json:"task_scan"`
		Downloader config.Downloader  `json:"downloader"`
		UIDefaults config.UIDefaults  `json:"ui_defaults"`
		LogLevel   string             `json:"log_level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, fmt.Sprintf(errFmtInvalidBody, err))
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
	// Update server section (only non-nil fields are applied)
	if req.Server.HTTPPort != nil {
		cc.Server.HTTPPort = *req.Server.HTTPPort
	}
	if req.Server.UIOnlyPort != nil {
		cc.Server.UIOnlyPort = *req.Server.UIOnlyPort
	}
	if req.Server.WorkDir != nil {
		cc.Server.WorkDir = *req.Server.WorkDir
	}
	if req.Server.DownloadRootDir != nil {
		cc.Server.DownloadRootDir = *req.Server.DownloadRootDir
	}
	if req.Server.FilesDir != nil {
		cc.Server.FilesDir = *req.Server.FilesDir
	}
	if req.Server.FilesAllowSymlink != nil {
		cc.Server.FilesAllowSymlink = *req.Server.FilesAllowSymlink
	}
	s.SetFilesAllowSymlink(cc.Server.FilesAllowSymlink)
	if req.Server.Auth != nil {
		auth := &cc.Server.Auth
		if req.Server.Auth.Type != "" {
			auth.Type = req.Server.Auth.Type
		}
		if req.Server.Auth.Username != "" {
			auth.Username = req.Server.Auth.Username
		}
		if req.Server.Auth.Password != "" {
			auth.Password = req.Server.Auth.Password
		}
		if req.Server.Auth.Token != "" {
			auth.Token = req.Server.Auth.Token
		}
	}
	// Update frontend log level
	if req.LogLevel != "" {
		cc.Runtime.LogLevel = req.LogLevel
	}
	if err := s.mgr.UpdateConfig(cc, &manager.AuditInfo{
		Author:  "ui",
		Source:  "api/config/server",
		Message: "server config updated",
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, errCodeUpdateFailed, fmt.Sprintf("Failed to update server config: %v", err))
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
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, fmt.Sprintf(errFmtInvalidBody, err))
		return
	}
	if err := s.mgr.UpdateLogConfig(newLog); err != nil {
		writeJSONError(w, http.StatusInternalServerError, errCodeUpdateFailed, fmt.Sprintf("Failed to update log config: %v", err))
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
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, fmt.Sprintf(errFmtInvalidBody, err))
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
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, fmt.Sprintf(errFmtInvalidBody, err))
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
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, fmt.Sprintf(errFmtInvalidBody, err))
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
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, fmt.Sprintf(errFmtInvalidBody, err))
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
		writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, "Invalid request body")
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
		writeJSONError(w, http.StatusInternalServerError, errCodeUpdateFailed, fmt.Sprintf("Failed to apply config: %v", err))
		return
	}
	w.WriteHeader(http.StatusOK)
}
