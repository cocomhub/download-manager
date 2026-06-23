// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import "context"

// Option 鏄?Downloader 鐨勯厤缃嚱鏁般€?type Option func(*Downloader)

// WithSelector 娉ㄥ叆鑷畾涔?Selector銆?func WithSelector(s Selector) Option {
	return func(d *Downloader) { d.selector = s }
}

// WithTransport 娉ㄥ叆鑷畾涔?Transport銆?func WithTransport(t Transport) Option {
	return func(d *Downloader) { d.transport = t }
}

// WithExtractor 娉ㄥ唽 Extractor銆?func WithExtractor(ex Extractor) Option {
	return func(d *Downloader) {
		d.extractors = append(d.extractors, ex)
	}
}

// WithMiddleware 璁剧疆涓嬭浇涓棿浠躲€?// 濡傛灉涔嬪墠宸茶缃腑闂翠欢锛屾柊涓棿浠朵細鍖呰鍦ㄥ閮紝褰㈡垚閾惧紡璋冪敤銆?// 娉ㄥ唽椤哄簭锛歔mw1, mw2] => 鎵ц椤哄簭: mw2_before -> mw1_before -> extractor -> mw1_after -> mw2_after
func WithMiddleware(mw Middleware) Option {
	return func(d *Downloader) {
		if d.middleware == nil {
			d.middleware = mw
			return
		}
		// 鏂颁腑闂翠欢鍦ㄦ渶澶栧眰锛氭柊涓棿浠惰皟鐢?next锛堟棫鐨勪腑闂翠欢閾撅級
		existing := d.middleware
		d.middleware = func(ctx context.Context, req *Request, next Extractor) error {
			return mw(ctx, req, &middlewareExtractor{
				base: next,
				mw:   existing,
			})
		}
	}
}

// WithMetricRegistry 璁剧疆鎸囨爣娉ㄥ唽琛紙骞惰嚜鍔ㄥ惎鐢?MetricsMiddleware锛夈€?func WithMetricRegistry(reg *MetricRegistry) Option {
	return func(d *Downloader) {
		d.metrics = reg
		mw := MetricsMiddleware(reg)
		if d.middleware == nil {
			d.middleware = mw
			return
		}
		// 閾惧紡鍖呰锛氭寚鏍囦腑闂翠欢鍦ㄦ渶澶栧眰璁板綍鎬绘椂闂?		existing := d.middleware
		d.middleware = func(ctx context.Context, req *Request, next Extractor) error {
			return mw(ctx, req, &middlewareExtractor{
				base: next,
				mw:   existing,
			})
		}
	}
}

// WithRuleSet 璁剧疆 URL 璺敱瑙勫垯闆嗐€?// RuleSet 鐢ㄤ簬鍦ㄩ€夋嫨 Extractor 涔嬪墠娉ㄨВ Request 鐨?Hint 瀛楁銆?func WithRuleSet(rs *RuleSet) Option {
	return func(d *Downloader) {
		// 鍖呰 selector锛屽湪鍖归厤鍓嶅簲鐢ㄨ鍒欓泦娉ㄨВ
		if d.selector == nil {
			d.selector = &ruleSetSelector{rs: rs}
			return
		}
		// 宸叉湁 selector锛屽寘瑁呬箣
		base := d.selector
		d.selector = &ruleSetSelector{rs: rs, next: base}
	}
}

// ruleSetSelector 鍖呰涓€涓?Selector锛屽湪 MatchExtractor 鍓嶅厛閫氳繃 RuleSet 娉ㄨВ Hint銆?type ruleSetSelector struct {
	rs   *RuleSet
	next Selector
}

func (r *ruleSetSelector) MatchExtractor(ctx context.Context, url string, hint *DownloadHint) Extractor {
	// 鍏堥€氳繃瑙勫垯闆嗗尮閰嶏紝鑻ュ尮閰嶅垯娉ㄨВ Hint
	if matched := r.rs.Match(url, hint); matched != nil {
		if hint == nil {
			hint = &DownloadHint{Extractor: matched.Extractor}
		} else if matched.Extractor != "" {
			hint.Extractor = matched.Extractor
		}
	}
	if r.next != nil {
		return r.next.MatchExtractor(ctx, url, hint)
	}
	return nil
}

func (r *ruleSetSelector) SelectProxy(ctx context.Context, targetURL string, hint *DownloadHint) (string, error) {
	if r.next != nil {
		return r.next.SelectProxy(ctx, targetURL, hint)
	}
	return "", nil
}
