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

// DefaultSelector 是默认的 Selector 实现。
// 按注册顺序遍历 Extractor，第一个 Match 返回 true 的被选中。
// NewDefaultSelector 创建 DefaultSelector 实例。
func NewDefaultSelector() *DefaultSelector {
	return &DefaultSelector{
		extractors: make([]Extractor, 0),
	}
}

type DefaultSelector struct {
	extractors    []Extractor
	proxySelector ProxySelector
}

func (s *DefaultSelector) WithProxySelector(ps ProxySelector) *DefaultSelector {
	s.proxySelector = ps
	return s
}

func (s *DefaultSelector) MatchExtractor(ctx context.Context, url string, hint *DownloadHint) Extractor {
	if hint != nil && hint.Extractor != "" {
		for _, ex := range s.extractors {
			if ex.Name() == hint.Extractor {
				return ex
			}
		}
	}
	for _, ex := range s.extractors {
		if ex.Match(ctx, url) {
			return ex
		}
	}
	return nil
}

func (s *DefaultSelector) SelectProxy(ctx context.Context, targetURL string, hint *DownloadHint) (string, error) {
	if s.proxySelector != nil {
		return s.proxySelector.Select(ctx, targetURL, hint)
	}
	return "", nil
}