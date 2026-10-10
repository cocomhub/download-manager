// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
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
		"log":       cfg.Log,
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
	if dc, ok := m["proxy"].(map[string]any); ok {
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

// yamlSecretKeyRe 匹配 YAML 中的机密键（值替换为空串）。
// 前置分隔符（行首/空白/花括号/逗号）避免误伤 has_api_token 之类前缀；支持引号键与流式映射。
var yamlSecretKeyRe = regexp.MustCompile(`(?m)(^|[\s{,])("?[']?(?:access_key_secret|api_token|password|secret|token|cookie|set-cookie|authorization|proxy-authorization|x-api-key|scraper_tunnel_key)[']?"?\s*:\s*)(.*)$`)

// yamlBlockSecretRe 匹配「机密键 + 块标量（| / >）」：其缩进正文整体置空。
var yamlBlockSecretRe = regexp.MustCompile(`(?m)^([ 	]*["']?(?:access_key_secret|api_token|password|secret|token|cookie|set-cookie|authorization|proxy-authorization|x-api-key|scraper_tunnel_key)["']?[ 	]*:[ 	]*[|>][-+]?[ 	]*
)((?:[ 	]+[^
]*
?)*)`)

// secretKeyNameRe 判断 map 键名是否为凭据（用于结构化 changes 的递归掩码）。
var secretKeyNameRe = regexp.MustCompile(`(?i)^(cookie|set-cookie|authorization|proxy-authorization|x-api-key|api[-_]?key|token|password|secret|access_key_secret)$`)

// proxyUserinfoRe 匹配 URL 中的 userinfo（user:pass@）。
// 贪婪匹配到最后一个 @（与 url.Parse 取最后 userinfo 的语义一致）。
var proxyUserinfoRe = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.\-]*://)[^/\s]*@`)

// redactYAMLSecrets 掩掉 YAML 文本中的已知机密（机密键值 + 代理 URL 的 user:pass）。
func redactYAMLSecrets(text string) string {
	if text == "" {
		return text
	}
	out := yamlSecretKeyRe.ReplaceAllString(text, `${1}${2}""`)
	out = yamlBlockSecretRe.ReplaceAllString(out, `${1}  ""
`)
	return proxyUserinfoRe.ReplaceAllString(out, `${1}`)
}

// redactDiffChanges 掩掉 diff 结构化结果中代理列表条目的 user:pass
// （否则 changes 会成为 /api/config/diff 的凭据旁路）。
func redactDiffChanges(res map[string]any) {
	changes, ok := res["changes"].([]config.Change)
	if !ok {
		return
	}
	for i := range changes {
		// 全量递归掩码：字符串走 userinfo 剥离，机密键名的值置空。
		// 覆盖 downloader.proxies/proxy.list、contexts（mongo uri）、tasks.<id>.extra
		// （headers.Cookie/Authorization）等所有结构化路径。
		changes[i].A = maskSecretsDeep(changes[i].A)
		changes[i].B = maskSecretsDeep(changes[i].B)
	}
	res["changes"] = changes
}

// maskSecretsDeep 递归掩掉值中的凭据：字符串剥离 URL userinfo，机密键名的值置空。
func maskSecretsDeep(v any) any {
	switch t := v.(type) {
	case string:
		return redactURLUserinfo(t)
	case []string:
		out := make([]string, 0, len(t))
		for _, e := range t {
			out = append(out, redactURLUserinfo(e))
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, e := range t {
			out = append(out, maskSecretsDeep(e))
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(t))
		for k, e := range t {
			if secretKeyNameRe.MatchString(k) {
				out[k] = ""
				continue
			}
			out[k] = redactURLUserinfo(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			if secretKeyNameRe.MatchString(k) {
				out[k] = ""
				continue
			}
			out[k] = maskSecretsDeep(e)
		}
		return out
	case nil:
		return nil
	default:
		// 结构体 / 命名 map 类型（config.Context、StorageConfig、map[string]Context、
		// map[string]TaskTypeDefault 等）：动态类型不匹配上面的分支，若直接返回会把
		// 内联凭据（如 contexts.*.storage.config.uri 的 mongo 密码）原样回泄。
		// 先 JSON 往返成通用形状，再递归掩码。
		b, err := json.Marshal(v)
		if err != nil {
			return v
		}
		var generic any
		if err := json.Unmarshal(b, &generic); err != nil {
			return v
		}
		return maskSecretsDeep(generic)
	}
}

// mergeRedactedProxies 合并请求回传的代理列表与当前配置：
// 请求项若与「当前项的脱敏视图」相同，说明用户未改动该条 → 保留当前项（含凭据）；
// 否则视为用户新增/修改 → 采用请求值。避免脱敏视图回写时静默抹掉代理凭据。
func mergeRedactedProxies(incoming, current []string) []string {
	if incoming == nil {
		return current // 字段未提供 → 不动当前配置
	}
	if len(incoming) == 0 {
		return []string{} // 显式清空
	}
	// 当前条目的「脱敏视图 → 原值」队列：按出现顺序消费，保证
	// ① 同 host+同用户名的多条（仅密码不同）也能各自回填、不把 *** 占位符落成真实密码；
	// ② 用户显式改写（视图不匹配，如去掉 @ 剥离凭据）时采用请求值。
	queue := make(map[string][]string, len(current))
	for _, p := range current {
		red := redactURLUserinfo(p)
		queue[red] = append(queue[red], p)
	}
	out := make([]string, 0, len(incoming))
	for _, p := range incoming {
		if q := queue[p]; len(q) > 0 {
			out = append(out, q[0])
			queue[p] = q[1:]
			continue
		}
		out = append(out, p)
	}
	return out
}

// redactUserinfoFallback 处理 url.Parse 不识别 userinfo 的形态（如无 scheme 的
// "user:pass@host:8080"）：按第一个 @ 前的最后一段作为密码掩码。
func redactUserinfoFallback(raw string) string {
	i := strings.LastIndex(raw, "@") // 与 url.Parse 取最后 userinfo 的语义一致
	if i <= 0 {
		return raw
	}
	head := raw[:i]
	if strings.ContainsAny(head, "/?#") {
		return raw // @ 出现在路径里，不是 userinfo
	}
	j := strings.LastIndex(head, ":")
	if j < 0 {
		return raw
	}
	return head[:j+1] + redactedPassword + raw[i:]
}

// redactedPassword 是脱敏视图中密码的占位符。
const redactedPassword = "***"

// redactURLUserinfo 返回密码已掩码的 URL（保留用户名与 @，如 http://user:***@host:8080）。
// 保留 @ 形态使前端回传时能区分「未改动（仍带占位符）」与「用户显式剥离凭据（无 @）」；
// 同 host 不同用户名的条目也因用户名不同而不会互相错配。解析失败/无 userinfo 时原样返回。
func redactURLUserinfo(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return redactUserinfoFallback(raw)
	}
	if u.User == nil {
		return redactUserinfoFallback(raw)
	}
	user := u.User.Username()
	u.User = nil
	s := u.String()
	// 手工拼回 user:***@：url.String() 会把 * 转义成 %2A，对 UI 不友好。
	if i := strings.Index(s, "://"); i >= 0 {
		return s[:i+3] + url.PathEscape(user) + ":" + redactedPassword + "@" + s[i+3:]
	}
	return s
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
		Log        *logutil.LogConfig `json:"log"`
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
	// 脱敏回写保护：GET 视图返回的是去掉 user:pass 的代理列表；请求若原样回传该视图，
	// 视为「未修改」并保留已存凭据（否则保存配置会静默抹掉代理凭据）。
	cc.Downloader.Proxies = mergeRedactedProxies(req.Downloader.Proxies, cc.Downloader.Proxies)
	cc.Downloader.DomainLimits = req.Downloader.DomainLimits
	// New sub-structures
	cc.Downloader.Filesystem = req.Downloader.Filesystem
	if req.Downloader.HTTP.TimeoutSeconds > 0 {
		cc.Downloader.HTTP = req.Downloader.HTTP
	}
	req.Downloader.Proxy.List = mergeRedactedProxies(req.Downloader.Proxy.List, cc.Downloader.Proxy.List)
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
	// 日志轮转设置（表单 log_* 字段）：仅在请求显式提供时覆盖
	if req.Log != nil {
		cc.Log = *req.Log
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
		if errors.Is(err, manager.ErrInvalidBackupRef) {
			writeJSONError(w, http.StatusBadRequest, errCodeInvalidRequest, err.Error())
			return
		}
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
	// diff 返回的是配置文件原始 YAML：掩掉已知机密键，避免绕过 /api/config/server 的脱敏
	for _, k := range []string{"left_yaml", "right_yaml", "left_norm", "right_norm"} {
		if v, ok := res[k].(string); ok {
			res[k] = redactYAMLSecrets(v)
		}
	}
	redactDiffChanges(res)
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
