package config

type Config struct {
	Server ServerConfig `yaml:"server"`
	Tasks  []TaskConfig `yaml:"tasks"`
}

type ServerConfig struct {
	ScanInterval int `yaml:"scan_interval"` // seconds
}

type TaskConfig struct {
	ID      string   `yaml:"id"`
	Type    string   `yaml:"type"`
	URLs    []string `yaml:"urls"`
	SaveDir string   `yaml:"save_dir"`
}
