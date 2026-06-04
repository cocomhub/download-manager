package download

import "context"

// Transport 接口封装底层 HTTP 传输层，支持代理和自定义请求/响应处理。
type Transport interface {
	// Name 返回传输层的名称。
	Name() string

	// RoundTrip 执行一次完整的 HTTP 往返，返回响应或错误。
	RoundTrip(ctx context.Context, req *TransportRequest) (*TransportResponse, error)
}