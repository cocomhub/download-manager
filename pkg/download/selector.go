// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import "context"

// Selector 鏄《灞傞€夋嫨鍣紝鍚屾椂璐熻矗鍖归厤鎻愬彇鍣ㄥ拰閫夋嫨浠ｇ悊銆?type Selector interface {
	// MatchExtractor 鏍规嵁 URL 鍜屾彁绀轰俊鎭繑鍥炲尮閰嶇殑 Extractor銆?	MatchExtractor(ctx context.Context, url string, hint *DownloadHint) Extractor

	// SelectProxy 鏍规嵁鐩爣 URL 鍜屾彁绀轰俊鎭繑鍥炰唬鐞?URL銆?	SelectProxy(ctx context.Context, targetURL string, hint *DownloadHint) (proxyURL string, err error)
}

// ProxySelector 鏄粎璐熻矗浠ｇ悊閫夋嫨鐨勬帴鍙ｃ€?type ProxySelector interface {
	// Select 鏍规嵁鐩爣 URL 鍜屾彁绀轰俊鎭繑鍥炰唬鐞?URL銆?	Select(ctx context.Context, targetURL string, hint *DownloadHint) (proxyURL string, err error)
}

// DefaultSelector 鏄粯璁ょ殑 Selector 瀹炵幇銆?// 鎸夋敞鍐岄『搴忛亶鍘?Extractor锛岀涓€涓?Match 杩斿洖 true 鐨勮閫変腑銆?// NewDefaultSelector 鍒涘缓 DefaultSelector 瀹炰緥銆?func NewDefaultSelector() *DefaultSelector {
	return &DefaultSelector{
		extractors: make([]Extractor, 0),
	}
}

type DefaultSelector struct {
	extractors    []Extractor
	proxySelector ProxySelector
}

// AddExtractor 鍚?DefaultSelector 娉ㄥ唽涓€涓?Extractor銆?func (s *DefaultSelector) AddExtractor(ex Extractor) *DefaultSelector {
	s.extractors = append(s.extractors, ex)
	return s
}

// WithProxySelector 璁剧疆浠ｇ悊閫夋嫨鍣ㄣ€?func (s *DefaultSelector) WithProxySelector(ps ProxySelector) *DefaultSelector {
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
