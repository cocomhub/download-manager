package config

type Config struct {
	Server     ServerConfig     `yaml:"server" json:"server"`
	Log        LogConfig        `yaml:"log" json:"log"`
	Mongo      []MongoSource    `yaml:"mongo" json:"mongo"`
	Tasks      []TaskConfig     `yaml:"tasks" json:"tasks"`
	Downloader DownloaderConfig `yaml:"downloader" json:"downloader"`
}

type LogConfig struct {
	Level      string `yaml:"level" json:"level"`
	Filename   string `yaml:"filename" json:"filename"`
	MaxSize    int    `yaml:"max_size" json:"max_size"`       // megabytes
	MaxBackups int    `yaml:"max_backups" json:"max_backups"` // max number of old log files to retain
	MaxAge     int    `yaml:"max_age" json:"max_age"`         // max number of days to retain old log files
	Compress   bool   `yaml:"compress" json:"compress"`
	Console    bool   `yaml:"console" json:"console"`
}

type ServerConfig struct {
	ScanInterval int    `yaml:"scan_interval" json:"scan_interval"` // seconds
	LockFile     string `yaml:"lock_file" json:"lock_file"`
	HTTPPort     int    `yaml:"http_port" json:"http_port"` // Add port for web UI
}

type MongoSource struct {
	Name string `yaml:"name" json:"name"`
	URI  string `yaml:"uri" json:"uri"`
}

type StorageConfig struct {
	Type   string            `yaml:"type" json:"type"`
	Config map[string]string `yaml:"config" json:"config"`
}

type TaskConfig struct {
	ID      string                 `yaml:"id" json:"id"`
	Type    string                 `yaml:"type" json:"type"`
	URLs    []string               `yaml:"urls" json:"urls"`
	SaveDir string                 `yaml:"save_dir" json:"save_dir"`
	Storage StorageConfig          `yaml:"storage" json:"storage"`
	Extra   map[string]interface{} `yaml:"extra" json:"extra"`
}

type DownloaderConfig struct {
	Proxies []string `yaml:"proxies" json:"proxies"`
	LogDir  string   `yaml:"log_dir" json:"log_dir"`
}
