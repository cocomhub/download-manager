package download

import "context"

// Selector 是顶层选择器，同时负责匹配提取器和选择代理。
type Selector interface {
	// MatchExtractor 根据 URL 和提示信息返回匹配的 Extractor。
	MatchExtractor(ctx context.Context, url string, hint *DownloadHint) Extractor

	// SelectProxy 根据目标 URL 和提示信息返回代理 URL。
	SelectProxy(ctx context.Context, targetURL string, hint *DownloadHint) (proxyURL string, err error)
}

// ProxySelector 是仅负责代理选择的接口。
type ProxySelector interface {
	// Select 根据目标 URL 和提示信息返回代理 URL。
	Select(ctx context.Context, targetURL string, hint *DownloadHint) (proxyURL string, err error)
}