package config

type Config struct {
	Server ServerConfig  `yaml:"server"`
	Mongo  []MongoSource `yaml:"mongo"`
	Tasks  []TaskConfig  `yaml:"tasks"`
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
	ID      string        `yaml:"id"`
	Type    string        `yaml:"type"`
	URLs    []string      `yaml:"urls"`
	SaveDir string        `yaml:"save_dir"`
	Storage StorageConfig `yaml:"storage"`
}
