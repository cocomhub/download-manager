package config

type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Log        LogConfig        `yaml:"log"`
	Mongo      []MongoSource    `yaml:"mongo"`
	Tasks      []TaskConfig     `yaml:"tasks"`
	Downloader DownloaderConfig `yaml:"downloader"`
}

type LogConfig struct {
	Level      string `yaml:"level"`
	Filename   string `yaml:"filename"`
	MaxSize    int    `yaml:"max_size"`    // megabytes
	MaxBackups int    `yaml:"max_backups"` // max number of old log files to retain
	MaxAge     int    `yaml:"max_age"`     // max number of days to retain old log files
	Compress   bool   `yaml:"compress"`
	Console    bool   `yaml:"console"`
}

type ServerConfig struct {
	ScanInterval int    `yaml:"scan_interval"` // seconds
	LockFile     string `yaml:"lock_file"`
	HTTPPort     int    `yaml:"http_port"` // Add port for web UI
}

type MongoSource struct {
	Name string `yaml:"name"`
	URI  string `yaml:"uri"`
}

type StorageConfig struct {
	Type   string            `yaml:"type"`
	Config map[string]string `yaml:"config"`
}

type TaskConfig struct {
	ID      string                 `yaml:"id"`
	Type    string                 `yaml:"type"`
	URLs    []string               `yaml:"urls"`
	SaveDir string                 `yaml:"save_dir"`
	Storage StorageConfig          `yaml:"storage"`
	Extra   map[string]interface{} `yaml:"extra"`
}

type DownloaderConfig struct {
	Proxies []string `yaml:"proxies"`
	LogDir  string   `yaml:"log_dir"`
}
