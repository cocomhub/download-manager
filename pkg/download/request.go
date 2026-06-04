package download

import "io"

// DownloadHint 携带下载提示信息，帮助 Selector 和 Extractor 做出决策。
type DownloadHint struct {
	FileSize    int64
	ContentType string
	Extractor   string
	Tags        map[string]string
}

// Request 描述一个下载请求，包含目标 URL、保存路径、头信息、进度回调等。
type Request struct {
	URL           string
	SavePath      string
	Headers       map[string]string
	TrackProgress bool
	OnProgress    func(progress float64, downloaded, total int64)
	Metadata      map[string]string
	Hint          *DownloadHint
}

// RangeRequest 描述一个 HTTP Range 请求的偏移量。
type RangeRequest struct {
	Offset int64
}

// TransportRequest 是 Transport 层使用的完整请求描述。
type TransportRequest struct {
	URL      string
	Method   string
	Headers  map[string]string
	Body     []byte
	Range    *RangeRequest
	ProxyURL string
}

// TransportResponse 是 Transport 层返回的响应，包含 Body 和元数据。
type TransportResponse struct {
	Body          io.ReadCloser
	StatusCode    int
	ContentLength int64
	Headers       map[string]string
	ProxyURL      string
}