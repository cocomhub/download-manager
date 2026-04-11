// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"log/slog"
	"path/filepath"
	"reflect"

	"github.com/cocomhub/download-manager/pkg/dlcore"
	"github.com/cocomhub/download-manager/pkg/logutil"
)

type Config struct {
	Server     Server            `yaml:"server" json:"server"`
	Log        logutil.LogConfig `yaml:"log" json:"log"`
	Mongo      []MongoSource     `yaml:"mongo" json:"mongo"`
	Downloader Downloader        `yaml:"downloader" json:"downloader"`
	TaskScan   TaskScan          `yaml:"task_scan" json:"task_scan"`
	Runtime    Runtime           `yaml:"runtime" json:"runtime"`
	Tasks      []Task            `yaml:"tasks" json:"tasks"`
}

type RunMode string

const (
	RunModeFull RunMode = "full"
	RunModeUI   RunMode = "ui"
)

type Runtime struct {
	Mode     RunMode `yaml:"mode" json:"mode"`
	Download struct {
		Enabled bool `yaml:"enabled" json:"enabled"`
	} `yaml:"download" json:"download"`
	Scheduler struct {
		Enabled bool `yaml:"enabled" json:"enabled"`
	} `yaml:"scheduler" json:"scheduler"`
}

type Server struct {
	HTTPPort       int        `yaml:"http_port" json:"http_port"`                 // Add port for web UI
	UIOnlyPort     int        `yaml:"ui_only_port" json:"ui_only_port"`           // Port for UI only mode
	WorkDir        string     `yaml:"work_dir" json:"work_dir"`                   // Working directory for cache etc
	LockFile       string     `yaml:"lock_file" json:"lock_file"`                 // Lock file for full mode
	UIOnlyLockFile string     `yaml:"ui_only_lock_file" json:"ui_only_lock_file"` // Run UI only mode, lock file for UI only mode
	ScraperPath    string     `yaml:"scraper_path" json:"scraper_path"`
	UIDefaults     UIDefaults `yaml:"ui_defaults" json:"ui_defaults"`
}

type MongoSource struct {
	Name string `yaml:"name" json:"name"`
	URI  string `yaml:"uri" json:"uri"`
}

type DcFilesystem struct {
	RootDir  string `yaml:"root_dir" json:"root_dir"`
	LogDir   string `yaml:"log_dir" json:"log_dir"`
	CacheDir string `yaml:"cache_dir" json:"cache_dir"`
}

type DcHTTP struct {
	TimeoutSeconds                  int    `yaml:"timeout_seconds" json:"timeout_seconds"`
	IdleConnTimeoutSeconds          int    `yaml:"idle_conn_timeout_seconds" json:"idle_conn_timeout_seconds"`
	MaxIdleConns                    int    `yaml:"max_idle_conns" json:"max_idle_conns"`
	MaxIdleConnsPerHost             int    `yaml:"max_idle_conns_per_host" json:"max_idle_conns_per_host"`
	DefaultUserAgent                string `yaml:"default_user_agent" json:"default_user_agent"`
	DisableInjectBrowserLikeHeaders bool   `yaml:"disable_inject_browser_like_headers" json:"disable_inject_browser_like_headers"`
}

type DcProxy struct {
	Force                  bool     `yaml:"force" json:"force"`
	List                   []string `yaml:"list" json:"list"`
	DecisionCacheTTLSecs   int      `yaml:"decision_cache_ttl_secs" json:"decision_cache_ttl_secs"`
	DirectProbeTimeoutSecs int      `yaml:"direct_probe_timeout_secs" json:"direct_probe_timeout_secs"`
	BandwidthPathSuffix    string   `yaml:"bandwidth_path_suffix" json:"bandwidth_path_suffix"`
}

type DcProgress struct {
	MinPercentStep     float64 `yaml:"min_percent_step" json:"min_percent_step"`
	MaxIntervalSeconds int     `yaml:"max_interval_seconds" json:"max_interval_seconds"`
}

type EnabledFlag struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

type DcFFmpeg struct {
	Path         string   `yaml:"path" json:"path"`
	ExtraArgs    []string `yaml:"extra_args" json:"extra_args"`
	MoveIfExists struct {
		Enabled bool   `yaml:"enabled" json:"enabled"`
		Dir     string `yaml:"dir" json:"dir"`
	} `yaml:"move_if_exists" json:"move_if_exists"`
	ExternalHLSLog struct {
		Enabled bool   `yaml:"enabled" json:"enabled"`
		Path    string `yaml:"path" json:"path"`
	} `yaml:"external_hls_log" json:"external_hls_log"`
	HLSAutoMarkAsFail bool `yaml:"hls_auto_mark_as_fail" json:"hls_auto_mark_as_fail"`
}

type Downloader struct {
	Type              string         `yaml:"type" json:"type"`
	GlobalConcurrent  int            `yaml:"global_concurrent" json:"global_concurrent"`
	MaxRetries        int            `yaml:"max_retries" json:"max_retries"`
	LogDir            string         `yaml:"log_dir" json:"log_dir"`
	ForceProxy        bool           `yaml:"force_proxy" json:"force_proxy"`
	Proxies           []string       `yaml:"proxies" json:"proxies"`
	DomainLimits      map[string]int `yaml:"domain_limits" json:"domain_limits"`
	FfmpegPath        string         `yaml:"ffmpeg_path" json:"ffmpeg_path"`
	HlsAutoMarkAsFail bool           `yaml:"hls_auto_mark_as_fail" json:"hls_auto_mark_as_fail"`
	Filesystem        DcFilesystem   `yaml:"filesystem" json:"filesystem"`
	HTTP              DcHTTP         `yaml:"http" json:"http"`
	Proxy             DcProxy        `yaml:"proxy" json:"proxy"`
	Progress          DcProgress     `yaml:"progress" json:"progress"`
	FFmpeg            DcFFmpeg       `yaml:"ffmpeg" json:"ffmpeg"`
}

type TaskScan struct {
	Disable  bool `yaml:"disable" json:"disable"`
	Interval int  `yaml:"interval" json:"interval"` // seconds
}

type UIDefaults struct {
	DefaultSaveDir    string `yaml:"default_save_dir" json:"default_save_dir"`
	WindowWidth       int    `yaml:"window_width" json:"window_width"`
	WindowHeight      int    `yaml:"window_height" json:"window_height"`
	DiffSideBySide    bool   `yaml:"diff_side_by_side" json:"diff_side_by_side"`
	DiffIgnoreWS      bool   `yaml:"diff_ignore_ws" json:"diff_ignore_ws"`
	DiffIgnoreComment bool   `yaml:"diff_ignore_comment" json:"diff_ignore_comment"`
	StatusStyle       string `yaml:"status_style" json:"status_style"` // pill | dot | overlay
}

type Task struct {
	ID      string         `yaml:"id" json:"id"`
	Type    string         `yaml:"type" json:"type"`
	SaveDir string         `yaml:"save_dir" json:"save_dir"`
	Storage StorageConfig  `yaml:"storage" json:"storage"`
	Extra   map[string]any `yaml:"extra" json:"extra"`
}

type StorageConfig struct {
	Type   string            `yaml:"type" json:"type"`
	Config map[string]string `yaml:"config" json:"config"`
}

func clampInt(v, min, max int) int {
	if min != 0 && v < min {
		return min
	}
	if max != 0 && v > max {
		return max
	}
	return v
}

func (c *Config) ValidateAndClamp() {
	if c == nil {
		return
	}
	isFSConfigured := func(fs DcFilesystem) bool {
		return fs.RootDir != "" || fs.LogDir != "" || fs.CacheDir != ""
	}
	isHTTPConfigured := func(h DcHTTP) bool {
		return h.TimeoutSeconds != 0 || h.IdleConnTimeoutSeconds != 0 ||
			h.MaxIdleConns != 0 || h.MaxIdleConnsPerHost != 0 ||
			h.DefaultUserAgent != "" || h.DisableInjectBrowserLikeHeaders
	}
	isProxyConfigured := func(p DcProxy) bool {
		return p.Force || len(p.List) > 0 || p.DecisionCacheTTLSecs != 0 ||
			p.DirectProbeTimeoutSecs != 0 || p.BandwidthPathSuffix != ""
	}
	isFFmpegConfigured := func(f DcFFmpeg) bool {
		return f.Path != "" || len(f.ExtraArgs) > 0 || f.MoveIfExists.Enabled ||
			f.ExternalHLSLog.Enabled || f.HLSAutoMarkAsFail
	}
	// Runtime defaults and validation
	if c.Runtime.Mode == "" {
		c.Runtime.Mode = RunModeFull
	}
	if c.Runtime.Mode != RunModeFull && c.Runtime.Mode != RunModeUI {
		slog.Warn("invalid runtime mode, fallback to full", "mode", string(c.Runtime.Mode))
		c.Runtime.Mode = RunModeFull
	}
	// Keep current behavior: both enabled by default when unspecified.
	// This method may be called after unmarshalling into a pre-initialized struct.
	// So do not force override false values here.
	if c.TaskScan.Interval <= 0 {
		c.TaskScan.Interval = 10
	}
	c.Downloader.GlobalConcurrent = clampInt(c.Downloader.GlobalConcurrent, 1, 100)
	if c.Downloader.MaxRetries < 0 {
		c.Downloader.MaxRetries = 0
	}
	// Log config lower bounds
	if c.Log.MaxSize <= 0 {
		c.Log.MaxSize = 1
	}
	if c.Log.MaxBackups < 0 {
		c.Log.MaxBackups = 0
	}
	if c.Log.MaxAge < 0 {
		c.Log.MaxAge = 0
	}
	// Per-task extra validations
	for i := range c.Tasks {
		t := &c.Tasks[i]
		if t.Extra == nil {
			continue
		}
		if v, ok := t.Extra["max_concurrent"]; ok {
			switch vv := v.(type) {
			case int:
				t.Extra["max_concurrent"] = clampInt(vv, 0, 100)
			case float64:
				t.Extra["max_concurrent"] = clampInt(int(vv), 0, 100)
			}
		}
		if v, ok := t.Extra["refresh_interval"]; ok {
			switch vv := v.(type) {
			case int:
				t.Extra["refresh_interval"] = clampInt(vv, 10, 86400)
			case float64:
				t.Extra["refresh_interval"] = clampInt(int(vv), 10, 86400)
			}
		}
	}
	// UI defaults clamp
	if c.Server.UIDefaults.WindowWidth < 480 {
		c.Server.UIDefaults.WindowWidth = 480
	}
	if c.Server.UIDefaults.WindowHeight < 320 {
		c.Server.UIDefaults.WindowHeight = 320
	}
	if c.Server.UIDefaults.DefaultSaveDir == "" {
		c.Server.UIDefaults.DefaultSaveDir = "./downloads"
	}
	if c.Server.UIDefaults.StatusStyle == "" {
		c.Server.UIDefaults.StatusStyle = "pill"
	}

	// Migration: old fields -> new sub-structures when new not explicitly set
	if !isFSConfigured(c.Downloader.Filesystem) && c.Downloader.LogDir != "" {
		c.Downloader.Filesystem.LogDir = c.Downloader.LogDir
	}
	if !isProxyConfigured(c.Downloader.Proxy) {
		if len(c.Downloader.Proxies) > 0 {
			c.Downloader.Proxy.List = append([]string(nil), c.Downloader.Proxies...)
		}
		if c.Downloader.ForceProxy {
			c.Downloader.Proxy.Force = true
		}
	}
	if !isFFmpegConfigured(c.Downloader.FFmpeg) {
		if c.Downloader.FfmpegPath != "" {
			c.Downloader.FFmpeg.Path = c.Downloader.FfmpegPath
		}
		if c.Downloader.HlsAutoMarkAsFail {
			c.Downloader.FFmpeg.HLSAutoMarkAsFail = true
		}
	}

	// Defaults for new sub-structures
	if c.Downloader.Filesystem.RootDir == "" {
		wd := c.Server.WorkDir
		if wd == "" {
			wd = "."
		}
		c.Downloader.Filesystem.RootDir = filepath.Join(wd, "downloads")
	}
	if c.Downloader.Filesystem.LogDir == "" {
		c.Downloader.Filesystem.LogDir = "logs"
	}
	if c.Downloader.Filesystem.CacheDir == "" {
		c.Downloader.Filesystem.CacheDir = ".cache"
	}

	if !isHTTPConfigured(c.Downloader.HTTP) {
		c.Downloader.HTTP.TimeoutSeconds = 600
		c.Downloader.HTTP.IdleConnTimeoutSeconds = 30
		c.Downloader.HTTP.MaxIdleConns = 100
		c.Downloader.HTTP.MaxIdleConnsPerHost = 10
		c.Downloader.HTTP.DefaultUserAgent = dlcore.DefaultUserAgent
	} else {
		// Partial defaults if some fields were left zero
		if c.Downloader.HTTP.TimeoutSeconds == 0 {
			c.Downloader.HTTP.TimeoutSeconds = 600
		}
		if c.Downloader.HTTP.IdleConnTimeoutSeconds == 0 {
			c.Downloader.HTTP.IdleConnTimeoutSeconds = 30
		}
		if c.Downloader.HTTP.MaxIdleConns == 0 {
			c.Downloader.HTTP.MaxIdleConns = 100
		}
		if c.Downloader.HTTP.MaxIdleConnsPerHost == 0 {
			c.Downloader.HTTP.MaxIdleConnsPerHost = 10
		}
		if c.Downloader.HTTP.DefaultUserAgent == "" {
			c.Downloader.HTTP.DefaultUserAgent = dlcore.DefaultUserAgent
		}
	}

	if c.Downloader.Proxy.DecisionCacheTTLSecs == 0 {
		c.Downloader.Proxy.DecisionCacheTTLSecs = 1
	}
	if c.Downloader.Proxy.DirectProbeTimeoutSecs == 0 {
		c.Downloader.Proxy.DirectProbeTimeoutSecs = 3
	}
	if c.Downloader.Proxy.BandwidthPathSuffix == "" {
		c.Downloader.Proxy.BandwidthPathSuffix = "/bandwidth"
	}

	if c.Downloader.Progress.MinPercentStep <= 0 {
		c.Downloader.Progress.MinPercentStep = 0.5
	}
	if c.Downloader.Progress.MaxIntervalSeconds <= 0 {
		c.Downloader.Progress.MaxIntervalSeconds = 10
	}

	if c.Downloader.FFmpeg.Path == "" {
		c.Downloader.FFmpeg.Path = "ffmpeg"
	}
}

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
