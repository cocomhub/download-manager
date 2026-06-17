// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import "reflect"

// Change represents a single configuration change between two Config values.
type Change struct {
	Path string `json:"path"`
	A    any    `json:"a"`
	B    any    `json:"b"`
}

func taskIndex(tasks []Task, id string) int {
	for i, t := range tasks {
		if t.ID == id {
			return i
		}
	}
	return -1
}

// Diff compares two Config values and returns a list of changes.
func (c Config) Diff(b Config) []Change {
	var changes []Change
	// Server
	if c.Server.HTTPPort != b.Server.HTTPPort {
		changes = append(changes, Change{Path: "server.http_port", A: c.Server.HTTPPort, B: b.Server.HTTPPort})
	}
	if c.Server.WorkDir != b.Server.WorkDir {
		changes = append(changes, Change{Path: "server.work_dir", A: c.Server.WorkDir, B: b.Server.WorkDir})
	}
	if c.Server.LockFile != b.Server.LockFile {
		changes = append(changes, Change{Path: "server.lock_file", A: c.Server.LockFile, B: b.Server.LockFile})
	}
	if c.Server.ScraperPath != b.Server.ScraperPath {
		changes = append(changes, Change{Path: "server.scraper_path", A: c.Server.ScraperPath, B: b.Server.ScraperPath})
	}
	if c.Server.DownloadRootDir != b.Server.DownloadRootDir {
		changes = append(changes, Change{Path: "server.download_root_dir", A: c.Server.DownloadRootDir, B: b.Server.DownloadRootDir})
	}
	if c.Server.FilesDir != b.Server.FilesDir {
		changes = append(changes, Change{Path: "server.files_dir", A: c.Server.FilesDir, B: b.Server.FilesDir})
	}
	if c.Server.Auth.Type != b.Server.Auth.Type {
		changes = append(changes, Change{Path: "server.auth.type", A: c.Server.Auth.Type, B: b.Server.Auth.Type})
	}
	if c.Server.Auth.Username != b.Server.Auth.Username {
		changes = append(changes, Change{Path: "server.auth.username", A: c.Server.Auth.Username, B: b.Server.Auth.Username})
	}
	if c.Server.Auth.Password != b.Server.Auth.Password {
		changes = append(changes, Change{Path: "server.auth.password", A: "(redacted)", B: "(redacted)"})
	}
	if c.Server.Auth.Token != b.Server.Auth.Token {
		changes = append(changes, Change{Path: "server.auth.token", A: "(redacted)", B: "(redacted)"})
	}
	if c.Server.UIDefaults.DefaultSaveDir != b.Server.UIDefaults.DefaultSaveDir {
		changes = append(changes, Change{Path: "server.ui_defaults.default_save_dir", A: c.Server.UIDefaults.DefaultSaveDir, B: b.Server.UIDefaults.DefaultSaveDir})
	}
	if c.Server.UIDefaults.WindowWidth != b.Server.UIDefaults.WindowWidth {
		changes = append(changes, Change{Path: "server.ui_defaults.window_width", A: c.Server.UIDefaults.WindowWidth, B: b.Server.UIDefaults.WindowWidth})
	}
	if c.Server.UIDefaults.WindowHeight != b.Server.UIDefaults.WindowHeight {
		changes = append(changes, Change{Path: "server.ui_defaults.window_height", A: c.Server.UIDefaults.WindowHeight, B: b.Server.UIDefaults.WindowHeight})
	}
	if c.Server.UIDefaults.DiffSideBySide != b.Server.UIDefaults.DiffSideBySide {
		changes = append(changes, Change{Path: "server.ui_defaults.diff_side_by_side", A: c.Server.UIDefaults.DiffSideBySide, B: b.Server.UIDefaults.DiffSideBySide})
	}
	if c.Server.UIDefaults.DiffIgnoreWS != b.Server.UIDefaults.DiffIgnoreWS {
		changes = append(changes, Change{Path: "server.ui_defaults.diff_ignore_ws", A: c.Server.UIDefaults.DiffIgnoreWS, B: b.Server.UIDefaults.DiffIgnoreWS})
	}
	if c.Server.UIDefaults.DiffIgnoreComment != b.Server.UIDefaults.DiffIgnoreComment {
		changes = append(changes, Change{Path: "server.ui_defaults.diff_ignore_comment", A: c.Server.UIDefaults.DiffIgnoreComment, B: b.Server.UIDefaults.DiffIgnoreComment})
	}
	if c.Server.UIDefaults.StatusStyle != b.Server.UIDefaults.StatusStyle {
		changes = append(changes, Change{Path: "server.ui_defaults.status_style", A: c.Server.UIDefaults.StatusStyle, B: b.Server.UIDefaults.StatusStyle})
	}

	// Log
	if c.Log.Level != b.Log.Level {
		changes = append(changes, Change{Path: "log.level", A: c.Log.Level, B: b.Log.Level})
	}
	if c.Log.Filename != b.Log.Filename {
		changes = append(changes, Change{Path: "log.filename", A: c.Log.Filename, B: b.Log.Filename})
	}
	if c.Log.MaxSize != b.Log.MaxSize {
		changes = append(changes, Change{Path: "log.max_size", A: c.Log.MaxSize, B: b.Log.MaxSize})
	}
	if c.Log.MaxBackups != b.Log.MaxBackups {
		changes = append(changes, Change{Path: "log.max_backups", A: c.Log.MaxBackups, B: b.Log.MaxBackups})
	}
	if c.Log.MaxAge != b.Log.MaxAge {
		changes = append(changes, Change{Path: "log.max_age", A: c.Log.MaxAge, B: b.Log.MaxAge})
	}
	if c.Log.Console != b.Log.Console {
		changes = append(changes, Change{Path: "log.console", A: c.Log.Console, B: b.Log.Console})
	}
	if c.Log.Compress != b.Log.Compress {
		changes = append(changes, Change{Path: "log.compress", A: c.Log.Compress, B: b.Log.Compress})
	}
	// Downloader
	if c.Downloader.Type != b.Downloader.Type {
		changes = append(changes, Change{Path: "downloader.type", A: c.Downloader.Type, B: b.Downloader.Type})
	}
	if c.Downloader.GlobalConcurrent != b.Downloader.GlobalConcurrent {
		changes = append(changes, Change{Path: "downloader.global_concurrent", A: c.Downloader.GlobalConcurrent, B: b.Downloader.GlobalConcurrent})
	}
	if c.Downloader.MaxRetries != b.Downloader.MaxRetries {
		changes = append(changes, Change{Path: "downloader.max_retries", A: c.Downloader.MaxRetries, B: b.Downloader.MaxRetries})
	}
	if c.Downloader.LogDir != b.Downloader.LogDir {
		changes = append(changes, Change{Path: "downloader.log_dir", A: c.Downloader.LogDir, B: b.Downloader.LogDir})
	}
	if c.Downloader.ForceProxy != b.Downloader.ForceProxy {
		changes = append(changes, Change{Path: "downloader.force_proxy", A: c.Downloader.ForceProxy, B: b.Downloader.ForceProxy})
	}
	if !reflect.DeepEqual(c.Downloader.Proxies, b.Downloader.Proxies) {
		changes = append(changes, Change{Path: "downloader.proxies", A: c.Downloader.Proxies, B: b.Downloader.Proxies})
	}
	// Downloader (new sub-structures)
	if c.Downloader.Filesystem.RootDir != b.Downloader.Filesystem.RootDir {
		changes = append(changes, Change{Path: "downloader.filesystem.root_dir", A: c.Downloader.Filesystem.RootDir, B: b.Downloader.Filesystem.RootDir})
	}
	if c.Downloader.Filesystem.LogDir != b.Downloader.Filesystem.LogDir {
		changes = append(changes, Change{Path: "downloader.filesystem.log_dir", A: c.Downloader.Filesystem.LogDir, B: b.Downloader.Filesystem.LogDir})
	}
	if c.Downloader.Filesystem.CacheDir != b.Downloader.Filesystem.CacheDir {
		changes = append(changes, Change{Path: "downloader.filesystem.cache_dir", A: c.Downloader.Filesystem.CacheDir, B: b.Downloader.Filesystem.CacheDir})
	}
	if c.Downloader.HTTP.TimeoutSeconds != b.Downloader.HTTP.TimeoutSeconds {
		changes = append(changes, Change{Path: "downloader.http.timeout_seconds", A: c.Downloader.HTTP.TimeoutSeconds, B: b.Downloader.HTTP.TimeoutSeconds})
	}
	if c.Downloader.HTTP.IdleConnTimeoutSeconds != b.Downloader.HTTP.IdleConnTimeoutSeconds {
		changes = append(changes, Change{Path: "downloader.http.idle_conn_timeout_seconds", A: c.Downloader.HTTP.IdleConnTimeoutSeconds, B: b.Downloader.HTTP.IdleConnTimeoutSeconds})
	}
	if c.Downloader.HTTP.MaxIdleConns != b.Downloader.HTTP.MaxIdleConns {
		changes = append(changes, Change{Path: "downloader.http.max_idle_conns", A: c.Downloader.HTTP.MaxIdleConns, B: b.Downloader.HTTP.MaxIdleConns})
	}
	if c.Downloader.HTTP.MaxIdleConnsPerHost != b.Downloader.HTTP.MaxIdleConnsPerHost {
		changes = append(changes, Change{Path: "downloader.http.max_idle_conns_per_host", A: c.Downloader.HTTP.MaxIdleConnsPerHost, B: b.Downloader.HTTP.MaxIdleConnsPerHost})
	}
	if c.Downloader.HTTP.DefaultUserAgent != b.Downloader.HTTP.DefaultUserAgent {
		changes = append(changes, Change{Path: "downloader.http.default_user_agent", A: c.Downloader.HTTP.DefaultUserAgent, B: b.Downloader.HTTP.DefaultUserAgent})
	}
	if c.Downloader.HTTP.DisableInjectBrowserLikeHeaders != b.Downloader.HTTP.DisableInjectBrowserLikeHeaders {
		changes = append(changes, Change{Path: "downloader.http.disable_inject_browser_like_headers", A: c.Downloader.HTTP.DisableInjectBrowserLikeHeaders, B: b.Downloader.HTTP.DisableInjectBrowserLikeHeaders})
	}
	if c.Downloader.Proxy.Force != b.Downloader.Proxy.Force {
		changes = append(changes, Change{Path: "downloader.proxy.force", A: c.Downloader.Proxy.Force, B: b.Downloader.Proxy.Force})
	}
	if !reflect.DeepEqual(c.Downloader.Proxy.List, b.Downloader.Proxy.List) {
		changes = append(changes, Change{Path: "downloader.proxy.list", A: c.Downloader.Proxy.List, B: b.Downloader.Proxy.List})
	}
	if c.Downloader.Proxy.DecisionCacheTTLSecs != b.Downloader.Proxy.DecisionCacheTTLSecs {
		changes = append(changes, Change{Path: "downloader.proxy.decision_cache_ttl_secs", A: c.Downloader.Proxy.DecisionCacheTTLSecs, B: b.Downloader.Proxy.DecisionCacheTTLSecs})
	}
	if c.Downloader.Proxy.DirectProbeTimeoutSecs != b.Downloader.Proxy.DirectProbeTimeoutSecs {
		changes = append(changes, Change{Path: "downloader.proxy.direct_probe_timeout_secs", A: c.Downloader.Proxy.DirectProbeTimeoutSecs, B: b.Downloader.Proxy.DirectProbeTimeoutSecs})
	}
	if c.Downloader.Proxy.BandwidthPathSuffix != b.Downloader.Proxy.BandwidthPathSuffix {
		changes = append(changes, Change{Path: "downloader.proxy.bandwidth_path_suffix", A: c.Downloader.Proxy.BandwidthPathSuffix, B: b.Downloader.Proxy.BandwidthPathSuffix})
	}
	if c.Downloader.Progress.MinPercentStep != b.Downloader.Progress.MinPercentStep {
		changes = append(changes, Change{Path: "downloader.progress.min_percent_step", A: c.Downloader.Progress.MinPercentStep, B: b.Downloader.Progress.MinPercentStep})
	}
	if c.Downloader.Progress.MaxIntervalSeconds != b.Downloader.Progress.MaxIntervalSeconds {
		changes = append(changes, Change{Path: "downloader.progress.max_interval_seconds", A: c.Downloader.Progress.MaxIntervalSeconds, B: b.Downloader.Progress.MaxIntervalSeconds})
	}
	if c.Downloader.FFmpeg.Path != b.Downloader.FFmpeg.Path {
		changes = append(changes, Change{Path: "downloader.ffmpeg.path", A: c.Downloader.FFmpeg.Path, B: b.Downloader.FFmpeg.Path})
	}
	if !reflect.DeepEqual(c.Downloader.FFmpeg.ExtraArgs, b.Downloader.FFmpeg.ExtraArgs) {
		changes = append(changes, Change{Path: "downloader.ffmpeg.extra_args", A: c.Downloader.FFmpeg.ExtraArgs, B: b.Downloader.FFmpeg.ExtraArgs})
	}
	if c.Downloader.FFmpeg.MoveIfExists.Enabled != b.Downloader.FFmpeg.MoveIfExists.Enabled {
		changes = append(changes, Change{Path: "downloader.ffmpeg.move_if_exists.enabled", A: c.Downloader.FFmpeg.MoveIfExists.Enabled, B: b.Downloader.FFmpeg.MoveIfExists.Enabled})
	}
	if c.Downloader.FFmpeg.ExternalHLSLog.Enabled != b.Downloader.FFmpeg.ExternalHLSLog.Enabled {
		changes = append(changes, Change{Path: "downloader.ffmpeg.external_hls_log.enabled", A: c.Downloader.FFmpeg.ExternalHLSLog.Enabled, B: b.Downloader.FFmpeg.ExternalHLSLog.Enabled})
	}
	if c.Downloader.FFmpeg.HLSAutoMarkAsFail != b.Downloader.FFmpeg.HLSAutoMarkAsFail {
		changes = append(changes, Change{Path: "downloader.ffmpeg.hls_auto_mark_as_fail", A: c.Downloader.FFmpeg.HLSAutoMarkAsFail, B: b.Downloader.FFmpeg.HLSAutoMarkAsFail})
	}
	// TaskScan
	if c.TaskScan.Disable != b.TaskScan.Disable {
		changes = append(changes, Change{Path: "task_scan.disable", A: c.TaskScan.Disable, B: b.TaskScan.Disable})
	}
	if c.TaskScan.Interval != b.TaskScan.Interval {
		changes = append(changes, Change{Path: "task_scan.interval", A: c.TaskScan.Interval, B: b.TaskScan.Interval})
	}
	// Contexts
	if !reflect.DeepEqual(c.Contexts, b.Contexts) {
		changes = append(changes, Change{Path: "contexts", A: c.Contexts, B: b.Contexts})
	}
	// Tasks
	for _, ta := range c.Tasks {
		j := taskIndex(b.Tasks, ta.ID)
		if j == -1 {
			changes = append(changes, Change{Path: "tasks." + ta.ID, A: "present", B: "removed"})
			continue
		}
		tb := b.Tasks[j]
		if ta.Type != tb.Type {
			changes = append(changes, Change{Path: "tasks." + ta.ID + ".type", A: ta.Type, B: tb.Type})
		}
		if ta.SaveDir != tb.SaveDir {
			changes = append(changes, Change{Path: "tasks." + ta.ID + ".save_dir", A: ta.SaveDir, B: tb.SaveDir})
		}
		if ta.Storage.Type != tb.Storage.Type || !reflect.DeepEqual(ta.Storage.Config, tb.Storage.Config) {
			changes = append(changes, Change{Path: "tasks." + ta.ID + ".storage", A: ta.Storage, B: tb.Storage})
		}
		if ta.StorageContext != tb.StorageContext {
			changes = append(changes, Change{Path: "tasks." + ta.ID + ".storage_context", A: ta.StorageContext, B: tb.StorageContext})
		}
		if !reflect.DeepEqual(ta.Extra, tb.Extra) {
			changes = append(changes, Change{Path: "tasks." + ta.ID + ".extra", A: ta.Extra, B: tb.Extra})
		}
	}
	for _, tb := range b.Tasks {
		i := taskIndex(c.Tasks, tb.ID)
		if i == -1 {
			changes = append(changes, Change{Path: "tasks." + tb.ID, A: "removed", B: "present"})
		}
	}
	return changes
}
