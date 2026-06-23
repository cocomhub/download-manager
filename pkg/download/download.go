// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

// Downloader 鏄敤鎴蜂娇鐢ㄧ殑涓昏鍏ュ彛銆?// 鎸佹湁 Selector銆丒xtractor 娉ㄥ唽琛ㄣ€乀ransport 寮曠敤銆丮iddleware 閾惧拰 Metrics锛岀紪鎺掍竴娆″畬鏁翠笅杞姐€?type Downloader struct {
	selector   Selector
	extractors []Extractor
	transport  Transport
	middleware Middleware
	metrics    *MetricRegistry
}

// ErrNoDefaultDownloader 琛ㄧず鏈厤缃粯璁?Downloader銆?var ErrNoDefaultDownloader = errors.New("default downloader not initialized; call download.SetDefault() or configure via download.New()")

// defaultDl 鏄寘绾ч粯璁?Downloader 瀹炰緥锛岄€氳繃 SetDefault 閰嶇疆銆?var (
	defaultDl   *Downloader
	defaultDlMu sync.RWMutex
)

// SetDefault 鏇挎崲鍖呯骇榛樿 Downloader 瀹炰緥銆?// 璋冪敤鍚?Default() 鍜?Get() 灏嗕娇鐢ㄦ瀹炰緥銆?func SetDefault(d *Downloader) {
	defaultDlMu.Lock()
	defaultDl = d
	defaultDlMu.Unlock()
}

// Default 杩斿洖鍖呯骇榛樿 Downloader銆傞娆¤皟鐢ㄦ椂鑻ユ湭鍒濆鍖栵紝鑷姩鍒涘缓銆?func Default() *Downloader {
	defaultDlMu.RLock()
	if defaultDl != nil {
		defaultDlMu.RUnlock()
		return defaultDl
	}
	defaultDlMu.RUnlock()

	defaultDlMu.Lock()
	defer defaultDlMu.Unlock()
	if defaultDl == nil {
		defaultDl = New()
	}
	return defaultDl
}

// Get 浣跨敤榛樿 Downloader 鎵ц涓€娆＄畝鍗曚笅杞姐€?// 鑻ラ粯璁ゅ疄渚嬫湭鍒濆鍖栵紝鑷姩鍒涘缓銆?func Get(ctx context.Context, url, savePath string) error {
	return Default().Download(ctx, &Request{
		URL:      url,
		SavePath: savePath,
	})
}

// New 鍒涘缓 Downloader锛屽彲閫氳繃 Option 鑷畾涔夐厤缃€?// 闆跺弬鏁拌皟鐢ㄦ椂鑷姩娉ㄥ唽 HTTPExtractor銆丼tdlibTransport銆丏efaultSelector銆?// 鑻ヤ紶鍏ヤ簡 WithExtractor锛屽垯涓嶆敞鍐岄粯璁?HTTPExtractor銆?func New(opts ...Option) *Downloader {
	d := &Downloader{
		transport:  NewStdlibTransport(),
		selector:   NewDefaultSelector(),
		extractors: nil, // nil锛歐ithExtractor 閫夐」 append 鍚庨潪 nil锛屽悗缃垽鏂笉浼氳拷鍔犻粯璁?	}
	for _, o := range opts {
		o(d)
	}
	if d.extractors == nil {
		d.extractors = []Extractor{NewHTTPExtractor()}
	}
	return d
}

// Download 鎵ц涓€娆′笅杞界殑瀹屾暣缂栨帓锛?//  1. Selector 鍖归厤 Extractor
//  2. 娉ㄥ叆 Transport 鍜?Selector锛堝鏋?Extractor 鏀寔锛?//  3. 鎵ц Extractor.Extract
func (d *Downloader) Download(ctx context.Context, req *Request) error {
	if req == nil || req.URL == "" || req.SavePath == "" {
		return fmt.Errorf("invalid request: missing URL or SavePath")
	}
	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}
	if req.Result == nil {
		req.Result = &DownloadResult{}
	}

	// 1. Selector 鍖归厤 Extractor
	var ex Extractor
	if d.selector != nil {
		ex = d.selector.MatchExtractor(ctx, req.URL, req.Hint)
	}
	if ex == nil {
		for _, e := range d.extractors {
			if e.Match(ctx, req.URL) {
				ex = e
				break
			}
		}
	}
	if ex == nil {
		return fmt.Errorf("no extractor found for URL: %s", req.URL)
	}

	slog.Debug("Download: matched extractor", "extractor", ex.Name(), "url", req.URL)

	// 2. 涓?Extractor 娉ㄥ叆 Transport 鍜?Selector锛堝鏋滄敮鎸侊級
	if hw, ok := ex.(interface{ SetTransport(Transport) }); ok && d.transport != nil {
		hw.SetTransport(d.transport)
	}
	if hw, ok := ex.(interface{ SetSelector(Selector) }); ok && d.selector != nil {
		hw.SetSelector(d.selector)
	}

	// 3. 涓棿浠堕摼鍖呰 Extractor
	executor := ex
	if d.middleware != nil {
		executor = &middlewareExtractor{
			base: ex,
			mw:   d.middleware,
		}
	}

	// 4. 鎵ц涓嬭浇
	return executor.Extract(ctx, req)
}
