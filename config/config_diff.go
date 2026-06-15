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
func (a Config) Diff(b Config) []Change {
	var changes []Change
	// Server
	if a.Server.HTTPPort != b.Server.HTTPPort {
		changes = append(changes, Change{Path: "server.http_port", A: a.Server.HTTPPort, B: b.Server.HTTPPort})
	}
	if a.Server.WorkDir != b.Server.WorkDir {
		changes = append(changes, Change{Path: "server.work_dir", A: a.Server.WorkDir, B: b.Server.WorkDir})
	}
	if a.Server.LockFile != b.Server.LockFile {
		changes = append(changes, Change{Path: "server.lock_file", A: a.Server.LockFile, B: b.Server.LockFile})
	}
	if a.Server.ScraperPath != b.Server.ScraperPath {
		changes = append(changes, Change{Path: "server.scraper_path", A: a.Server.ScraperPath, B: b.Server.ScraperPath})
	}
	if a.Server.DownloadRootDir != b.Server.DownloadRootDir {
		changes = append(changes, Change{Path: "server.download_root_dir", A: a.Server.DownloadRootDir, B: b.Server.DownloadRootDir})
	}
	if a.Server.UIDefaults.DefaultSaveDir != b.Server.UIDefaults.DefaultSaveDir {
		changes = append(changes, Change{Path: "server.ui_defaults.default_save_dir", A: a.Server.UIDefaults.DefaultSaveDir, B: b.Server.UIDefaults.DefaultSaveDir})
	}
	if a.Server.UIDefaults.WindowWidth != b.Server.UIDefaults.WindowWidth {
		changes = append(changes, Change{Path: "server.ui_defaults.window_width", A: a.Server.UIDefaults.WindowWidth, B: b.Server.UIDefaults.WindowWidth})
	}
	if a.Server.UIDefaults.WindowHeight != b.Server.UIDefaults.WindowHeight {
		changes = append(changes, Change{Path: "server.ui_defaults.window_height", A: a.Server.UIDefaults.WindowHeight, B: b.Server.UIDefaults.WindowHeight})
	}
	if a.Server.UIDefaults.DiffSideBySide != b.Server.UIDefaults.DiffSideBySide {
		changes = append(changes, Change{Path: "server.ui_defaults.diff_side_by_side", A: a.Server.UIDefaults.DiffSideBySide, B: b.Server.UIDefaults.DiffSideBySide})
	}
	if a.Server.UIDefaults.DiffIgnoreWS != b.Server.UIDefaults.DiffIgnoreWS {
		changes = append(changes, Change{Path: "server.ui_defaults.diff_ignore_ws", A: a.Server.UIDefaults.DiffIgnoreWS, B: b.Server.UIDefaults.DiffIgnoreWS})
	}
	if a.Server.UIDefaults.DiffIgnoreComment != b.Server.UIDefaults.DiffIgnoreComment {
		changes = append(changes, Change{Path: "server.ui_defaults.diff_ignore_comment", A: a.Server.UIDefaults.DiffIgnoreComment, B: b.Server.UIDefaults.DiffIgnoreComment})
	}
	if a.Server.UIDefaults.StatusStyle != b.Server.UIDefaults.StatusStyle {
		changes = append(changes, Change{Path: "server.ui_defaults.status_style", A: a.Server.UIDefaults.StatusStyle, B: b.Server.UIDefaults.StatusStyle})
	}

	// Log
	if a.Log.Level != b.Log.Level {
		changes = append(changes, Change{Path: "log.level", A: a.Log.Level, B: b.Log.Level})
	}
	if a.Log.Filename != b.Log.Filename {
		changes = append(changes, Change{Path: "log.filename", A: a.Log.Filename, B: b.Log.Filename})
	}
	if a.Log.MaxSize != b.Log.MaxSize {
		changes = append(changes, Change{Path: "log.max_size", A: a.Log.MaxSize, B: b.Log.MaxSize})
	}
	if a.Log.MaxBackups != b.Log.MaxBackups {
		changes = append(changes, Change{Path: "log.max_backups", A: a.Log.MaxBackups, B: b.Log.MaxBackups})
	}
	if a.Log.MaxAge != b.Log.MaxAge {
		changes = append(changes, Change{Path: "log.max_age", A: a.Log.MaxAge, B: b.Log.MaxAge})
	}
	if a.Log.Console != b.Log.Console {
		changes = append(changes, Change{Path: "log.console", A: a.Log.Console, B: b.Log.Console})
	}
	if a.Log.Compress != b.Log.Compress {
		changes = append(changes, Change{Path: "log.compress", A: a.Log.Compress, B: b.Log.Compress})
	}
	// Downloader
	if a.Downloader.Type != b.Downloader.Type {
		changes = append(changes, Change{Path: "downloader.type", A: a.Downloader.Type, B: b.Downloader.Type})
	}
	if a.Downloader.GlobalConcurrent != b.Downloader.GlobalConcurrent {
		changes = append(changes, Change{Path: "downloader.global_concurrent", A: a.Downloader.GlobalConcurrent, B: b.Downloader.GlobalConcurrent})
	}
	if a.Downloader.MaxRetries != b.Downloader.MaxRetries {
		changes = append(changes, Change{Path: "downloader.max_retries", A: a.Downloader.MaxRetries, B: b.Downloader.MaxRetries})
	}
	if a.Downloader.LogDir != b.Downloader.LogDir {
		changes = append(changes, Change{Path: "downloader.log_dir", A: a.Downloader.LogDir, B: b.Downloader.LogDir})
	}
	if a.Downloader.ForceProxy != b.Downloader.ForceProxy {
		changes = append(changes, Change{Path: "downloader.force_proxy", A: a.Downloader.ForceProxy, B: b.Downloader.ForceProxy})
	}
	if !reflect.DeepEqual(a.Downloader.Proxies, b.Downloader.Proxies) {
		changes = append(changes, Change{Path: "downloader.proxies", A: a.Downloader.Proxies, B: b.Downloader.Proxies})
	}
	// Downloader (new sub-structures)
	if a.Downloader.Filesystem.RootDir != b.Downloader.Filesystem.RootDir {
		changes = append(changes, Change{Path: "downloader.filesystem.root_dir", A: a.Downloader.Filesystem.RootDir, B: b.Downloader.Filesystem.RootDir})
	}
	if a.Downloader.Filesystem.LogDir != b.Downloader.Filesystem.LogDir {
		changes = append(changes, Change{Path: "downloader.filesystem.log_dir", A: a.Downloader.Filesystem.LogDir, B: b.Downloader.Filesystem.LogDir})
	}
	if a.Downloader.Filesystem.CacheDir != b.Downloader.Filesystem.CacheDir {
		changes = append(changes, Change{Path: "downloader.filesystem.cache_dir", A: a.Downloader.Filesystem.CacheDir, B: b.Downloader.Filesystem.CacheDir})
	}
	if a.Downloader.HTTP.TimeoutSeconds != b.Downloader.HTTP.TimeoutSeconds {
		changes = append(changes, Change{Path: "downloader.http.timeout_seconds", A: a.Downloader.HTTP.TimeoutSeconds, B: b.Downloader.HTTP.TimeoutSeconds})
	}
	if a.Downloader.HTTP.IdleConnTimeoutSeconds != b.Downloader.HTTP.IdleConnTimeoutSeconds {
		changes = append(changes, Change{Path: "downloader.http.idle_conn_timeout_seconds", A: a.Downloader.HTTP.IdleConnTimeoutSeconds, B: b.Downloader.HTTP.IdleConnTimeoutSeconds})
	}
	if a.Downloader.HTTP.MaxIdleConns != b.Downloader.HTTP.MaxIdleConns {
		changes = append(changes, Change{Path: "downloader.http.max_idle_conns", A: a.Downloader.HTTP.MaxIdleConns, B: b.Downloader.HTTP.MaxIdleConns})
	}
	if a.Downloader.HTTP.MaxIdleConnsPerHost != b.Downloader.HTTP.MaxIdleConnsPerHost {
		changes = append(changes, Change{Path: "downloader.http.max_idle_conns_per_host", A: a.Downloader.HTTP.MaxIdleConnsPerHost, B: b.Downloader.HTTP.MaxIdleConnsPerHost})
	}
	if a.Downloader.HTTP.DefaultUserAgent != b.Downloader.HTTP.DefaultUserAgent {
		changes = append(changes, Change{Path: "downloader.http.default_user_agent", A: a.Downloader.HTTP.DefaultUserAgent, B: b.Downloader.HTTP.DefaultUserAgent})
	}
	if a.Downloader.HTTP.DisableInjectBrowserLikeHeaders != b.Downloader.HTTP.DisableInjectBrowserLikeHeaders {
		changes = append(changes, Change{Path: "downloader.http.disable_inject_browser_like_headers", A: a.Downloader.HTTP.DisableInjectBrowserLikeHeaders, B: b.Downloader.HTTP.DisableInjectBrowserLikeHeaders})
	}
	if a.Downloader.Proxy.Force != b.Downloader.Proxy.Force {
		changes = append(changes, Change{Path: "downloader.proxy.force", A: a.Downloader.Proxy.Force, B: b.Downloader.Proxy.Force})
	}
	if !reflect.DeepEqual(a.Downloader.Proxy.List, b.Downloader.Proxy.List) {
		changes = append(changes, Change{Path: "downloader.proxy.list", A: a.Downloader.Proxy.List, B: b.Downloader.Proxy.List})
	}
	if a.Downloader.Proxy.DecisionCacheTTLSecs != b.Downloader.Proxy.DecisionCacheTTLSecs {
		changes = append(changes, Change{Path: "downloader.proxy.decision_cache_ttl_secs", A: a.Downloader.Proxy.DecisionCacheTTLSecs, B: b.Downloader.Proxy.DecisionCacheTTLSecs})
	}
	if a.Downloader.Proxy.DirectProbeTimeoutSecs != b.Downloader.Proxy.DirectProbeTimeoutSecs {
		changes = append(changes, Change{Path: "downloader.proxy.direct_probe_timeout_secs", A: a.Downloader.Proxy.DirectProbeTimeoutSecs, B: b.Downloader.Proxy.DirectProbeTimeoutSecs})
	}
	if a.Downloader.Proxy.BandwidthPathSuffix != b.Downloader.Proxy.BandwidthPathSuffix {
		changes = append(changes, Change{Path: "downloader.proxy.bandwidth_path_suffix", A: a.Downloader.Proxy.BandwidthPathSuffix, B: b.Downloader.Proxy.BandwidthPathSuffix})
	}
	if a.Downloader.Progress.MinPercentStep != b.Downloader.Progress.MinPercentStep {
		changes = append(changes, Change{Path: "downloader.progress.min_percent_step", A: a.Downloader.Progress.MinPercentStep, B: b.Downloader.Progress.MinPercentStep})
	}
	if a.Downloader.Progress.MaxIntervalSeconds != b.Downloader.Progress.MaxIntervalSeconds {
		changes = append(changes, Change{Path: "downloader.progress.max_interval_seconds", A: a.Downloader.Progress.MaxIntervalSeconds, B: b.Downloader.Progress.MaxIntervalSeconds})
	}
	if a.Downloader.FFmpeg.Path != b.Downloader.FFmpeg.Path {
		changes = append(changes, Change{Path: "downloader.ffmpeg.path", A: a.Downloader.FFmpeg.Path, B: b.Downloader.FFmpeg.Path})
	}
	if !reflect.DeepEqual(a.Downloader.FFmpeg.ExtraArgs, b.Downloader.FFmpeg.ExtraArgs) {
		changes = append(changes, Change{Path: "downloader.ffmpeg.extra_args", A: a.Downloader.FFmpeg.ExtraArgs, B: b.Downloader.FFmpeg.ExtraArgs})
	}
	if a.Downloader.FFmpeg.MoveIfExists.Enabled != b.Downloader.FFmpeg.MoveIfExists.Enabled {
		changes = append(changes, Change{Path: "downloader.ffmpeg.move_if_exists.enabled", A: a.Downloader.FFmpeg.MoveIfExists.Enabled, B: b.Downloader.FFmpeg.MoveIfExists.Enabled})
	}
	if a.Downloader.FFmpeg.ExternalHLSLog.Enabled != b.Downloader.FFmpeg.ExternalHLSLog.Enabled {
		changes = append(changes, Change{Path: "downloader.ffmpeg.external_hls_log.enabled", A: a.Downloader.FFmpeg.ExternalHLSLog.Enabled, B: b.Downloader.FFmpeg.ExternalHLSLog.Enabled})
	}
	if a.Downloader.FFmpeg.HLSAutoMarkAsFail != b.Downloader.FFmpeg.HLSAutoMarkAsFail {
		changes = append(changes, Change{Path: "downloader.ffmpeg.hls_auto_mark_as_fail", A: a.Downloader.FFmpeg.HLSAutoMarkAsFail, B: b.Downloader.FFmpeg.HLSAutoMarkAsFail})
	}
	// TaskScan
	if a.TaskScan.Disable != b.TaskScan.Disable {
		changes = append(changes, Change{Path: "task_scan.disable", A: a.TaskScan.Disable, B: b.TaskScan.Disable})
	}
	if a.TaskScan.Interval != b.TaskScan.Interval {
		changes = append(changes, Change{Path: "task_scan.interval", A: a.TaskScan.Interval, B: b.TaskScan.Interval})
	}
	// Contexts
	if !reflect.DeepEqual(a.Contexts, b.Contexts) {
		changes = append(changes, Change{Path: "contexts", A: a.Contexts, B: b.Contexts})
	}
	// Tasks
	for _, ta := range a.Tasks {
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
		i := taskIndex(a.Tasks, tb.ID)
		if i == -1 {
			changes = append(changes, Change{Path: "tasks." + tb.ID, A: "removed", B: "present"})
		}
	}
	return changes
}
