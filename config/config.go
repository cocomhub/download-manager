package config

import (
	"reflect"

	"download-manager/logutil"
)

type Config struct {
	Server     Server            `yaml:"server" json:"server"`
	Log        logutil.LogConfig `yaml:"log" json:"log"`
	Mongo      []MongoSource     `yaml:"mongo" json:"mongo"`
	Downloader Downloader        `yaml:"downloader" json:"downloader"`
	TaskScan   TaskScan          `yaml:"task_scan" json:"task_scan"`
	Tasks      []Task            `yaml:"tasks" json:"tasks"`
}

type Server struct {
	HTTPPort    int        `yaml:"http_port" json:"http_port"` // Add port for web UI
	WorkDir     string     `yaml:"work_dir" json:"work_dir"`   // Working directory for cache etc
	LockFile    string     `yaml:"lock_file" json:"lock_file"`
	ScraperPath string     `yaml:"scraper_path" json:"scraper_path"`
	UIDefaults  UIDefaults `yaml:"ui_defaults" json:"ui_defaults"`
}

type MongoSource struct {
	Name string `yaml:"name" json:"name"`
	URI  string `yaml:"uri" json:"uri"`
}

type Downloader struct {
	Type             string   `yaml:"type" json:"type"`
	GlobalConcurrent int      `yaml:"global_concurrent" json:"global_concurrent"`
	MaxRetries       int      `yaml:"max_retries" json:"max_retries"`
	LogDir           string   `yaml:"log_dir" json:"log_dir"`
	ForceProxy       bool     `yaml:"force_proxy" json:"force_proxy"`
	Proxies          []string `yaml:"proxies" json:"proxies"`
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
}

type Task struct {
	ID      string                 `yaml:"id" json:"id"`
	Type    string                 `yaml:"type" json:"type"`
	SaveDir string                 `yaml:"save_dir" json:"save_dir"`
	Storage StorageConfig          `yaml:"storage" json:"storage"`
	Extra   map[string]interface{} `yaml:"extra" json:"extra"`
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
				t.Extra["max_concurrent"] = clampInt(vv, 1, 100)
			case float64:
				t.Extra["max_concurrent"] = clampInt(int(vv), 1, 100)
			}
		}
		if v, ok := t.Extra["refresh_interval"]; ok {
			switch vv := v.(type) {
			case int:
				if vv < 10 {
					t.Extra["refresh_interval"] = 10
				}
			case float64:
				iv := int(vv)
				if iv < 10 {
					iv = 10
				}
				t.Extra["refresh_interval"] = iv
			}
		}
	}
	// UI defaults clamp
	if c.Server.UIDefaults.WindowWidth < 480 {
		c.Server.UIDefaults.WindowWidth = 1200
	}
	if c.Server.UIDefaults.WindowHeight < 320 {
		c.Server.UIDefaults.WindowHeight = 800
	}
	if c.Server.UIDefaults.DefaultSaveDir == "" {
		c.Server.UIDefaults.DefaultSaveDir = "./downloads"
	}
}

type Change struct {
	Path string      `json:"path"`
	A    interface{} `json:"a"`
	B    interface{} `json:"b"`
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
