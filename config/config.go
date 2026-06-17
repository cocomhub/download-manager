// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"log/slog"
	"maps"
	"path/filepath"

	"github.com/cocomhub/download-manager/pkg/logutil"
)

const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"

type Config struct {
	Server     Server             `yaml:"server" json:"server"`
	Log        logutil.LogConfig  `yaml:"log" json:"log"`
	Mongo      []MongoSource      `yaml:"mongo" json:"mongo"`
	Downloader Downloader         `yaml:"downloader" json:"downloader"`
	TaskScan   TaskScan           `yaml:"task_scan" json:"task_scan"`
	Runtime    Runtime            `yaml:"runtime" json:"runtime"`
	Contexts   map[string]Context `yaml:"contexts" json:"contexts"`
	Tasks      []Task             `yaml:"tasks" json:"tasks"`
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
	HTTPPort        int        `yaml:"http_port" json:"http_port"`                 // Add port for web UI
	UIOnlyPort      int        `yaml:"ui_only_port" json:"ui_only_port"`           // Port for UI only mode
	WorkDir         string     `yaml:"work_dir" json:"work_dir"`                   // Working directory for cache etc
	LockFile        string     `yaml:"lock_file" json:"lock_file"`                 // Lock file for full mode
	UIOnlyLockFile  string     `yaml:"ui_only_lock_file" json:"ui_only_lock_file"` // Run UI only mode, lock file for UI only mode
	ScraperPath     string     `yaml:"scraper_path" json:"scraper_path"`
	DownloadRootDir string     `yaml:"download_root_dir" json:"download_root_dir"` // Root directory for downloads
	FilesDir        string     `yaml:"files_dir" json:"files_dir"`                 // Root directory for HTTP /files/ serving
	Auth            AuthConfig `yaml:"auth" json:"auth"`
	UIDefaults      UIDefaults `yaml:"ui_defaults" json:"ui_defaults"`
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
	ID             string         `yaml:"id" json:"id"`
	Type           string         `yaml:"type" json:"type"`
	SaveDir        string         `yaml:"save_dir" json:"save_dir"`
	Storage        StorageConfig  `yaml:"storage" json:"storage"`
	StorageContext string         `yaml:"storage_context" json:"storage_context"`
	Extra          map[string]any `yaml:"extra" json:"extra"`
}

type StorageConfig struct {
	Type   string            `yaml:"type" json:"type"`
	Config map[string]string `yaml:"config" json:"config"`
}

// Context defines a named set of common task settings that tasks can reference
// by name via Task.StorageContext. Designed to be extensible for future settings
// (e.g., proxy, downloader overrides) beyond storage.
type Context struct {
	Storage StorageConfig `yaml:"storage" json:"storage"`
}

// AuthConfig defines HTTP authentication settings.
type AuthConfig struct {
	Type     string `yaml:"type" json:"type"`         // "none" | "basic" | "token"
	Username string `yaml:"username" json:"username"` // basic auth username, default "admin"
	Password string `yaml:"password" json:"password"` // environment variable DM_AUTH_PASSWORD takes precedence
	Token    string `yaml:"token" json:"token"`       // environment variable DM_AUTH_TOKEN takes precedence
}

// FileRoot returns the root directory for HTTP /files/ serving.
// Prefer FilesDir if set, otherwise fall back to Downloader.Filesystem.RootDir.
func (c *Config) FileRoot() string {
	if c.Server.FilesDir != "" {
		return c.Server.FilesDir
	}
	return c.Downloader.Filesystem.RootDir
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

// Clone returns a deep copy of the Config, including all map and slice fields.
func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}
	cc := *c // shallow copy value

	// Deep-copy Tasks (each has Extra map and Storage.Config map)
	cc.Tasks = make([]Task, len(c.Tasks))
	for i, t := range c.Tasks {
		tc := t
		if t.Extra != nil {
			tc.Extra = make(map[string]any, len(t.Extra))
			maps.Copy(tc.Extra, t.Extra)
		}
		if t.Storage.Config != nil {
			tc.Storage.Config = make(map[string]string, len(t.Storage.Config))
			maps.Copy(tc.Storage.Config, t.Storage.Config)
		}
		cc.Tasks[i] = tc
	}

	// Deep-copy Contexts (each has Storage.Config map)
	if c.Contexts != nil {
		cc.Contexts = make(map[string]Context, len(c.Contexts))
		for k, ctx := range c.Contexts {
			ctxc := ctx
			if ctx.Storage.Config != nil {
				ctxc.Storage.Config = make(map[string]string, len(ctx.Storage.Config))
				maps.Copy(ctxc.Storage.Config, ctx.Storage.Config)
			}
			cc.Contexts[k] = ctxc
		}
	}

	// Deep-copy Downloader slices and maps
	if c.Downloader.Proxies != nil {
		cc.Downloader.Proxies = make([]string, len(c.Downloader.Proxies))
		copy(cc.Downloader.Proxies, c.Downloader.Proxies)
	}
	if c.Downloader.DomainLimits != nil {
		cc.Downloader.DomainLimits = make(map[string]int, len(c.Downloader.DomainLimits))
		maps.Copy(cc.Downloader.DomainLimits, c.Downloader.DomainLimits)
	}
	if c.Downloader.Proxy.List != nil {
		cc.Downloader.Proxy.List = make([]string, len(c.Downloader.Proxy.List))
		copy(cc.Downloader.Proxy.List, c.Downloader.Proxy.List)
	}
	if c.Downloader.FFmpeg.ExtraArgs != nil {
		cc.Downloader.FFmpeg.ExtraArgs = make([]string, len(c.Downloader.FFmpeg.ExtraArgs))
		copy(cc.Downloader.FFmpeg.ExtraArgs, c.Downloader.FFmpeg.ExtraArgs)
	}
	// MoveIfExists and ExternalHLSLog are plain structs — shallow copy is sufficient.

	// Deep-cache Mongo (simple struct slice, no maps)
	if c.Mongo != nil {
		cc.Mongo = make([]MongoSource, len(c.Mongo))
		copy(cc.Mongo, c.Mongo)
	}

	return &cc
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
	// Migrate deprecated "native_http" type to "native_old"
	if c.Downloader.Type == "native_http" {
		slog.Warn("config: downloader type 'native_http' is deprecated, migrating to 'native_old'. " +
			"Use type 'native' for the new pkg/download path.")
		c.Downloader.Type = "native_old"
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
	// Server defaults
	if c.Server.HTTPPort <= 0 {
		c.Server.HTTPPort = 8080
	}
	if c.Server.UIOnlyPort <= 0 {
		c.Server.UIOnlyPort = 8091
	}
	if c.Server.DownloadRootDir == "" {
		c.Server.DownloadRootDir = filepath.Join(c.Server.WorkDir, "downloads")
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
		c.Downloader.HTTP.DefaultUserAgent = defaultUserAgent
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
			c.Downloader.HTTP.DefaultUserAgent = defaultUserAgent
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

	// Context resolution: substitute named context references for inline storage.
	// Precedence: explicit inline storage (Storage.Type != "") > storage_context
	// reference > zero value. A task with both keeps inline storage; the context
	// name is preserved in the struct for round-trip fidelity.
	for i := range c.Tasks {
		t := &c.Tasks[i]
		if t.StorageContext == "" {
			continue
		}
		if t.Storage.Type != "" {
			continue // explicit inline takes precedence
		}
		if len(c.Contexts) == 0 {
			slog.Warn("task references context but no contexts defined",
				"task_id", t.ID, "context", t.StorageContext)
			continue
		}
		ctx, ok := c.Contexts[t.StorageContext]
		if !ok {
			slog.Warn("task references unknown context",
				"task_id", t.ID, "context", t.StorageContext)
			continue
		}
		t.Storage = ctx.Storage
	}
}
