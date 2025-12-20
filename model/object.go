package model

// DownloadObject 代表一个具体的下载对象
type DownloadObject struct {
	URL      string                 `json:"url"`
	SavePath string                 `json:"save_path"`
	Metadata map[string]string      `json:"metadata"`
	Extra    map[string]interface{} `json:"extra"`
	Status   string                 `json:"status"` // pending, downloading, completed, failed
}

const (
	StatusPending     = "pending"
	StatusDownloading = "downloading"
	StatusCompleted   = "completed"
	StatusFailed      = "failed"
)
