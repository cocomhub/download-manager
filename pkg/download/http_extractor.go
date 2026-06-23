// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const logTimestampFmt = "20060102150405"

// mediaExtensionSet 鏄鏈熶负濯掍綋鏂囦欢鐨?URL 鎵╁睍鍚嶉泦鍚堬紝鐢ㄤ簬 Content-Type 鏍￠獙銆?// 褰?URL 鎵╁睍鍚嶅湪姝ら泦鍚堜腑浣嗗搷搴?Content-Type 涓嶅尮閰嶆湡鏈涚被鍨嬫椂锛屾姤 ErrNoTry銆?const (
	mimePrefixVideo = "video/"
	mimePrefixImage = "image/"
)

var mediaExtensionSet = map[string]string{
	".mp4":  mimePrefixVideo,
	".jpg":  mimePrefixImage,
	".jpeg": mimePrefixImage,
	".png":  mimePrefixImage,
	".gif":  mimePrefixImage,
	".webp": mimePrefixImage,
	".bmp":  mimePrefixImage,
}

// ResponseCheck 鏄?HTTP 鍝嶅簲鏍￠獙鍑芥暟銆傚湪 tryDownload 鎷垮埌鍝嶅簲鍚庛€佸啓鏂囦欢涔嬪墠璋冪敤銆?// 杩斿洖 error 鍒欑粓姝笅杞斤紙ErrNoTry 琛ㄧず姘镐箙缁堟锛屽叾浠?error 鍙噸璇曪級銆?type ResponseCheck func(req *Request, tresp *TransportResponse) error

// HTTPExtractor 鏄€氱敤 HTTP 鏂囦欢涓嬭浇缂栨帓鍣ㄣ€?// 瀹冧娇鐢?Transport 鍋氬瓧鑺備紶杈擄紝鑷繁绠＄悊閲嶈瘯銆佹柇鐐圭画浼犮€丮D5 鏍￠獙銆?type HTTPExtractor struct {
	transport      Transport
	selector       Selector
	maxRetries     int
	rootDir        string
	logDir         string
	ua             string
	allowPaths     []string
	browserHdrs    bool
	cancels        sync.Map // map[string]context.CancelFunc
	responseChecks []ResponseCheck
}

// SetBrowserHeaders 鎺у埗鏄惁娉ㄥ叆 Chrome 椋庢牸娴忚鍣ㄦ爣澶淬€?func (e *HTTPExtractor) SetBrowserHeaders(v bool) { e.browserHdrs = v }

// AddResponseCheck 娉ㄥ唽涓€涓搷搴旀牎楠屽嚱鏁帮紝鍦ㄦ瘡娆′笅杞芥嬁鍒板搷搴斿悗鎵ц銆?// 澶氫釜 check 鎸夋敞鍐岄『搴忔墽琛岋紝浠讳竴杩斿洖 error 鍒欑粓姝笅杞姐€?func (e *HTTPExtractor) AddResponseCheck(fn ResponseCheck) {
	e.responseChecks = append(e.responseChecks, fn)
}

// Cancel 瀹炵幇 Canceller 鎺ュ彛锛屾寜 URL 鍙栨秷姝ｅ湪杩涜鐨勪笅杞姐€?func (e *HTTPExtractor) Cancel(url string) error {
	if v, ok := e.cancels.LoadAndDelete(url); ok {
		if cancel, ok := v.(context.CancelFunc); ok {
			cancel()
		}
	}
	return nil
}

// NewHTTPExtractor 鍒涘缓骞惰繑鍥?HTTPExtractor 瀹炰緥銆?func NewHTTPExtractor() *HTTPExtractor {
	return NewHTTPExtractorWithConfig(5, "", "", "")
}

// NewHTTPExtractorWithConfig 鏍规嵁閰嶇疆鍒涘缓 HTTPExtractor 瀹炰緥銆?func NewHTTPExtractorWithConfig(maxRetries int, userAgent, rootDir, logDir string) *HTTPExtractor {
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"
	}
	return &HTTPExtractor{
		maxRetries: maxRetries,
		rootDir:    rootDir,
		logDir:     logDir,
		ua:         userAgent,
	}
}

// SetTransport 娉ㄥ叆 Transport 瀹炰緥锛堝疄鐜?ExtractorWithTransport 鎺ュ彛锛夈€?func (e *HTTPExtractor) SetTransport(t Transport) { e.transport = t }

// SetSelector 娉ㄥ叆 Selector 瀹炰緥锛堝疄鐜?ExtractorWithSelector 鎺ュ彛锛夈€?func (e *HTTPExtractor) SetSelector(s Selector) { e.selector = s }

// SetAllowPaths 璁剧疆涓嬭浇璺緞鐧藉悕鍗曪紙鍙€夛級銆?func (e *HTTPExtractor) SetAllowPaths(paths []string) { e.allowPaths = paths }

// Name 杩斿洖鎻愬彇鍣ㄥ悕绉般€?func (e *HTTPExtractor) Name() string { return "http" }

// Match 鍒ゆ柇鏄惁閫傚悎澶勭悊璇?URL銆侶TTPExtractor 澶勭悊闈?m3u8 鐨?URL銆?func (e *HTTPExtractor) Match(ctx context.Context, url string) bool {
	return !strings.Contains(strings.ToLower(url), ".m3u8")
}

// Extract 鎵ц瀹屾暣鐨?HTTP 鏂囦欢涓嬭浇缂栨帓銆?func (e *HTTPExtractor) Extract(ctx context.Context, req *Request) error {
	// 纭繚 Transport 宸叉敞鍏?	if e.transport == nil {
		return fmt.Errorf("http: transport not set, call SetTransport before Extract")
	}

	// 鍒涘缓 per-URL 鍙彇娑堢殑 context锛屾敮鎸佹寜 URL 绮剧‘鍙栨秷
	dlCtx, dlCancel := context.WithCancel(ctx)
	defer e.cancels.Delete(req.URL)
	defer dlCancel()
	e.cancels.Store(req.URL, dlCancel)

	rPath := req.SavePath
	var err error
	if e.rootDir != "" {
		rPath, err = ResolvePathWithAllowList(e.rootDir, e.allowPaths, req.SavePath)
		if err != nil {
			return err
		}
	}

	// 鍒涘缓鐩綍
	if err := os.MkdirAll(filepath.Dir(rPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// 閫夋嫨浠ｇ悊
	proxyURL := ""
	if e.selector != nil {
		proxyURL, err = e.selector.SelectProxy(dlCtx, req.URL, req.Hint)
		if err != nil {
			slog.Warn("Proxy selection failed, falling back to direct", "url", req.URL, "error", err)
		}
	}

	// ETag/checksum 妫€鏌ワ細濡傛灉鏂囦欢瀹屾暣涓斿唴瀹规湭鍙橈紝璺宠繃涓嬭浇
	prevETag := ""
	prevChecksum := ""
	if req.Metadata != nil {
		prevETag = req.Metadata["etag"]
		prevChecksum = req.Metadata["checksum"]
	}

	action := ResolveAction(rPath, prevETag, prevChecksum, os.Stat, func(path string) (string, error) {
		_, hexMD5, err := ComputeFileMD5(path)
		return hexMD5, err
	})
	if action == ActionSkip {
		slog.Info("Skipping download 鈥?file unchanged (ETag + checksum match)", "file", req.SavePath)
		if req.Result == nil {
			req.Result = &DownloadResult{}
		}
		req.Result.StatusCode = http.StatusNotModified
		req.Result.TotalSize = func() int64 {
			if fi, _ := os.Stat(rPath); fi != nil {
				return fi.Size()
			}
			return 0
		}()
		if req.OnProgress != nil {
			req.OnProgress(100, req.Result.TotalSize, req.Result.TotalSize)
		}
		return nil
	}

	// 妫€鏌ュ凡鏈夋枃浠跺ぇ灏忥紙鏂偣缁紶鏀寔锛?	startOffset := int64(0)
	if fi, statErr := os.Stat(rPath); statErr == nil && fi.Size() > 0 {
		// 鏂囦欢瀛樺湪锛?		// - ActionSkip: 璺宠繃涓嬭浇锛圗Tag+checksum 鍖归厤锛夛紝淇濈暀 startOffset=0
		// - ActionResume: 缁紶
		// - ActionReDownload/ActionDownload: 鏃?ETag 鍏冩暟鎹絾鏈夋枃浠?鈫?涔熷皾璇曠画浼?		//   濡傛灉鏈嶅姟鍣ㄤ笉鏀寔缁紶浼氳繑鍥?200锛屼唬鐮佽嚜鍔ㄥ洖閫€
		if action == ActionResume || (action == ActionDownload && fi.Size() > 0) {
			startOffset = fi.Size()
			slog.Info("Resuming download (best-effort)", "file", req.SavePath, "offset", startOffset)
		} else if action == ActionReDownload {
			// ETag 涓嶄竴鑷存垨鏂囦欢鎹熷潖锛屾竻闄ゅ悗閲嶆柊涓嬭浇
			_ = os.Remove(rPath)
			slog.Info("Removing stale file for re-download", "file", req.SavePath)
		}
	}

	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}
	if req.Result == nil {
		req.Result = &DownloadResult{}
	}

	// 閲嶈瘯寰幆
	maxRetries := e.maxRetries
	if maxRetries <= 0 {
		maxRetries = 5 // 淇濆簳閲嶈瘯锛堜笌 Manager 灞備竴鑷达級
	}
	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-dlCtx.Done():
			return dlCtx.Err()
		default:
		}

		var success bool
		success, err = e.tryDownload(dlCtx, rPath, req.URL, proxyURL, startOffset, req)
		if err == nil && success {
			return nil
		}
		// tryDownload 杩斿洖 304 鏃?success=true 涓?req.Result.StatusCode==304
		if err == nil && success && req.Result != nil && req.Result.StatusCode == http.StatusNotModified {
			slog.Info("Download: 304 Not Modified, file unchanged", "file", req.SavePath)
			return nil
		}
		if err != nil && IsNoTry(err) {
			return err
		}
		if !success && err == nil {
			// 闇€瑕侀噸鏂板紑濮嬶紙濡?MD5 涓嶅尮閰嶆垨 416锛?			startOffset = 0
			continue
		}

		slog.Warn("Download attempt failed, retrying", "attempt", attempt, "url", req.URL, "error", err)
		time.Sleep(time.Duration(attempt) * time.Second)
	}

	return fmt.Errorf("%w: max retries reached (%d)", ErrNoTry, e.maxRetries)
}

// tryDownload 鎵ц鍗曟涓嬭浇灏濊瘯銆傝繑鍥?success=true 琛ㄧず涓嬭浇瀹屾垚锛屽惁鍒欒繑鍥為敊璇€?func (e *HTTPExtractor) tryDownload(ctx context.Context, rPath, rawURL, proxyURL string, startOffset int64, req *Request) (success bool, err error) {

	// ---- 杩涘害鏃ュ織鏂囦欢鍒涘缓 ----
	var logWriter io.Writer
	if e.logDir != "" {
		logFileName := filepath.Base(rPath)
		logFile := filepath.Join(e.logDir, logFileName+"."+
			time.Now().Format(logTimestampFmt)+".progress.log")
		f, fErr := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if fErr != nil {
			slog.Warn("Failed to create progress log file", "file", logFile, "error", fErr)
		} else {
			defer f.Close()
			logWriter = f
		}
	}
	started := time.Now()
	defer func() {
		if err != nil && logWriter != nil {
			fmt.Fprintf(logWriter, "%s Download failed: %v\n",
				time.Now().Format(time.RFC3339Nano), err)
		}
	}()

	// ---- 鏃ュ織锛氫繚瀛樿矾寰?+ 浠ｇ悊 ----
	if logWriter != nil {
		fmt.Fprintf(logWriter, "Save file to %s\n", rPath)
		if proxyURL != "" {
			// 鑴辨晱浠ｇ悊 URL锛氬幓闄よ璇佷俊鎭?			safeProxy := proxyURL
			if parsed, pErr := url.Parse(proxyURL); pErr == nil {
				parsed.User = nil
				safeProxy = parsed.String()
			}
			fmt.Fprintf(logWriter, "Using proxy: %s\n", safeProxy)
		} else {
			fmt.Fprintf(logWriter, "Direct connection\n")
		}
		fmt.Fprintf(logWriter, "Requesting URL: %s\n\n", rawURL)
	}

	treq := &TransportRequest{
		URL:      rawURL,
		Method:   "GET",
		ProxyURL: proxyURL,
		Headers:  e.buildHeaders(req),
	}

	// 鏂偣缁紶锛氳缃?Range 澶?	if startOffset > 0 {
		treq.Range = &RangeRequest{Offset: startOffset}
	}

	tresp, tErr := e.transport.RoundTrip(ctx, treq)
	if tErr != nil {
		return false, tErr
	}
	defer tresp.Body.Close()

	// ---- 鏃ュ織锛欻TTP 璇锋眰澶?+ 鍝嶅簲澶?----
	if logWriter != nil {
		// 闇€瑕佽劚鏁忕殑鏁忔劅璇锋眰澶村垪琛?		redactedHeaders := map[string]bool{
			"authorization":       true,
			"cookie":              true,
			"proxy-authorization": true,
			"x-api-key":           true,
		}

		fmt.Fprintf(logWriter, "[%s] Request:\n", treq.Method)
		fmt.Fprintf(logWriter, "URL: %s\n", rawURL)
		if treq.ProxyURL != "" {
			fmt.Fprintf(logWriter, "Proxy: %s\n", treq.ProxyURL)
		} else if proxyURL != "" {
			fmt.Fprintf(logWriter, "Proxy: %s\n", proxyURL)
		}
		fmt.Fprintf(logWriter, "Headers:\n")
		for k, v := range treq.Headers {
			if redactedHeaders[strings.ToLower(k)] {
				v = "[REDACTED]"
			}
			fmt.Fprintf(logWriter, "\t%s: %s\n", k, v)
		}
		if treq.Range != nil && treq.Range.Offset > 0 {
			fmt.Fprintf(logWriter, "\tRange: bytes=%d-\n", treq.Range.Offset)
		}
		fmt.Fprintf(logWriter, "\n")

		fmt.Fprintf(logWriter, "[%d] Response:\n", tresp.StatusCode)
		if statusText := http.StatusText(tresp.StatusCode); statusText != "" {
			fmt.Fprintf(logWriter, "Status: %d %s\n", tresp.StatusCode, statusText)
		}
		fmt.Fprintf(logWriter, "Content-Length: %d\n", tresp.ContentLength)
		fmt.Fprintf(logWriter, "Headers:\n")
		for k, v := range tresp.Headers {
			if redactedHeaders[strings.ToLower(k)] {
				v = "[REDACTED]"
			}
			fmt.Fprintf(logWriter, "\t%s: %s\n", k, v)
		}
		fmt.Fprintf(logWriter, "\n")
	}

	// 妫€鏌?HTTP 鐘舵€佺爜
	if tresp.StatusCode == http.StatusForbidden || tresp.StatusCode == http.StatusNotFound {
		return false, fmt.Errorf("%w: HTTP %d", ErrNoTry, tresp.StatusCode)
	}

	// 澶勭悊 304 Not Modified 鈥?鏂囦欢鏈彉鏇达紝璺宠繃涓嬭浇
	if tresp.StatusCode == http.StatusNotModified {
		if logWriter != nil {
			fmt.Fprintf(logWriter, "Server responded with 304 Not Modified, file unchanged\n")
		}
		// 涓嶅疄闄呭啓鍏ユ枃浠讹紝鐩存帴瑙嗕负鎴愬姛
		req.Result.StatusCode = http.StatusNotModified
		req.Result.ContentLength = 0
		req.Result.TotalSize = 0
		if fi, stErr := os.Stat(rPath); stErr == nil {
			req.Result.TotalSize = fi.Size()
		}
		if req.OnProgress != nil {
			req.OnProgress(100, req.Result.TotalSize, req.Result.TotalSize)
		}
		// 淇濆瓨 ETag锛?04 鍝嶅簲涔熶細鎼哄甫 ETag锛屼笌 200 涓€鑷达級
		if etag := tresp.Headers["Etag"]; etag != "" {
			setReqMetadata(req, "etag", etag)
		} else if etag := tresp.Headers["ETag"]; etag != "" {
			setReqMetadata(req, "etag", etag)
		}
		return true, nil
	}

	if tresp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		// 416 Range Not Satisfiable 鈫?鏂囦欢鍙兘宸插彉锛岄噸鏂颁粠 0 寮€濮?		if logWriter != nil {
			fmt.Fprintf(logWriter, "Server responded with 416 Range Not Satisfiable, restarting download\n")
		}
		return false, nil
	}

	if tresp.StatusCode != http.StatusOK && tresp.StatusCode != http.StatusPartialContent {
		if tresp.StatusCode >= 400 {
			// 500+ 绾ч敊璇厑璁搁噸璇曪紙闈?ErrNoTry锛?			return false, fmt.Errorf("HTTP %d", tresp.StatusCode)
		}
		return false, fmt.Errorf("HTTP error: %d", tresp.StatusCode)
	}

	// Content-Type 涓ユ牸鏍￠獙锛氬獟浣撴墿灞曞悕蹇呴』杩斿洖鍖归厤鐨?Content-Type 鍓嶇紑
	// 浣跨敤 url.Parse 鍚庣殑 Path 鍙栨墿灞曞悕锛岄伩鍏嶆煡璇㈠弬鏁帮紙?token=abc锛夊共鎵?	mediaExt := ""
	if parsedURL, parseErr := url.Parse(rawURL); parseErr == nil {
		mediaExt = strings.ToLower(filepath.Ext(parsedURL.Path))
	}
	if ext := mediaExt; mediaExtensionSet[ext] != "" {
		expectedPrefix := mediaExtensionSet[ext]
		ct := tresp.Headers["Content-Type"]
		if ct == "" {
			ct = tresp.Headers["content-type"]
		}
		if !strings.HasPrefix(ct, expectedPrefix) {
			if logWriter != nil {
				fmt.Fprintf(logWriter, "Content-Type mismatch for %s: expected %s*, got %s\n", ext, expectedPrefix, ct)
			}
			return false, fmt.Errorf("%w: invalid content type: expected %s*, got %s", ErrNoTry, expectedPrefix, ct)
		}
	}

	// 鍝嶅簲鏍￠獙閽╁瓙锛氭敞鍐岀殑 ResponseCheck 鎸夐『搴忔墽琛岋紝浠讳竴杩斿洖 error 鍒欑粓姝笅杞?	for _, check := range e.responseChecks {
		if err := check(req, tresp); err != nil {
			if logWriter != nil {
				fmt.Fprintf(logWriter, "Response check failed: %v\n", err)
			}
			return false, err
		}
	}

	// 濡傛灉璇锋眰浜?Range 浣嗘湇鍔″櫒杩斿洖 200锛堣€岄潪 206锛夛紝璇存槑涓嶆敮鎸佹柇鐐圭画浼?	// 杩斿洖 (false, nil) 璁╁灞傞噸缃?startOffset=0 浠庡ご涓嬭浇瀹屾暣鍐呭
	if startOffset > 0 && tresp.StatusCode == http.StatusOK {
		if logWriter != nil {
			fmt.Fprintf(logWriter, "Server doesn't support resume, restarting download\n")
		}
		return false, nil
	}

	// 璁＄畻鎬诲ぇ灏?	totalSize := tresp.ContentLength
	if cr := tresp.Headers["Content-Range"]; cr != "" {
		parts := strings.Split(cr, "/")
		if len(parts) == 2 {
			if parsed, pErr := strconv.ParseInt(parts[1], 10, 64); pErr == nil {
				totalSize = parsed
			}
		}
	}
	// 鏂偣缁紶鏃舵娴嬫湇鍔″櫒鍐呭鏄惁宸插彉鏇达細濡傛灉鏈嶅姟鍣ㄨ繑鍥炵殑瀹屾暣鍐呭姣旀湰鍦板凡鏈夋枃浠惰繕灏忥紝
	// 璇存槑鏂囦欢宸茶鏇挎崲/鎴柇锛屽繀椤婚噸缃?offset 閲嶆柊涓嬭浇瀹屾暣鍐呭銆?	if startOffset > 0 && totalSize > 0 && totalSize < startOffset {
		slog.Info("Server content changed during resume, restarting download", "file", rPath, "serverSize", totalSize, "localSize", startOffset)
		return false, nil
	}
	if startOffset > 0 && totalSize > 0 {
		totalSize += startOffset
	}

	// 鍐欏叆鏂囦欢
	fileFlags := os.O_CREATE | os.O_WRONLY
	if startOffset > 0 {
		fileFlags |= os.O_APPEND
	} else {
		fileFlags |= os.O_TRUNC
	}

	file, fErr := os.OpenFile(rPath, fileFlags, 0644)
	if fErr != nil {
		return false, fmt.Errorf("failed to open file: %w", fErr)
	}
	defer file.Close()

	// 鏃ュ織锛氭枃浠舵ā寮?+ 棰勬湡澶у皬
	if logWriter != nil {
		if startOffset > 0 {
			fmt.Fprintf(logWriter, "File mode: append (resume from offset %d)\n", startOffset)
		} else {
			fmt.Fprintf(logWriter, "File mode: truncate (new download)\n")
		}
		fmt.Fprintf(logWriter, "Expected total: %d bytes\n\n", totalSize)
	}

	var reader io.Reader = tresp.Body
	if req.TrackProgress && req.OnProgress != nil && totalSize > 0 {
		// 鍦?ProgressReader 澶栧眰娉ㄥ叆杩涘害鏃ュ織鍥炶皟锛堜笌鍘熸湁鍥炶皟缁勫悎锛夈€?		// 娉ㄦ剰锛氫娇鐢ㄥ眬閮ㄥ彉閲忚€岄潪淇敼 req.OnProgress锛岄伩鍏嶉噸璇曟椂鍥炶皟閾剧疮绉€?		onProgress := req.OnProgress
		if logWriter != nil {
			onProgress = ComposeProgress(
				req.OnProgress,
				NewProgressLogCallback(
					WithLogWriter(logWriter),
					WithMinPercentStep(0.5),
					WithMaxInterval(10*time.Second),
				),
			)
		}
		reader = NewProgressReader(tresp.Body, totalSize, onProgress)
	}

	if _, cErr := io.Copy(file, reader); cErr != nil {
		return false, fmt.Errorf("failed to write file: %w", cErr)
	}

	// 濉啓缁撴灉
	req.Result.StatusCode = tresp.StatusCode
	req.Result.ContentLength = totalSize
	req.Result.TotalSize = totalSize

	// MD5 鏍￠獙
	if wantMd5 := TryGetMd5(tresp.Headers); wantMd5 != "" {
		base64MD5, hexMD5, md5Err := ComputeFileMD5(rPath)
		if md5Err != nil {
			return false, fmt.Errorf("failed to compute MD5: %w", md5Err)
		}
		if base64MD5 != wantMd5 && hexMD5 != wantMd5 {
			slog.Warn("MD5 mismatch, retrying download", "want", wantMd5, "got", base64MD5)
			if logWriter != nil {
				fmt.Fprintf(logWriter, "MD5 check failed: want %s, got %s (hex: %s)\n",
					wantMd5, base64MD5, hexMD5)
			}
			return false, nil // return false 瑙﹀彂閲嶆柊涓嬭浇
		}
		req.Result.MD5Base64 = base64MD5
		req.Result.MD5Hex = hexMD5
		setReqMetadata(req, "checksum", hexMD5)
		if logWriter != nil {
			fmt.Fprintf(logWriter, "MD5 check passed: %s (hex: %s)\n", base64MD5, hexMD5)
		}
	}

	// 淇濆瓨 ETag 鍒?metadata锛堜緵涓嬫涓嬭浇鏃跺喅绛栵級
	if etag := tresp.Headers["Etag"]; etag != "" {
		setReqMetadata(req, "etag", etag)
	} else if etag := tresp.Headers["ETag"]; etag != "" {
		setReqMetadata(req, "etag", etag)
	}

	// 濡傛灉鏈嶅姟绔病缁?ETag 浣?MD5 鏍￠獙閫氳繃浜嗭紝鐢?MD5 hex 浣滀负寮辨牎楠屼緷鎹?	if req.Metadata["etag"] == "" && req.Result.MD5Hex != "" {
		setReqMetadata(req, "etag", `"`+req.Result.MD5Hex+`"`)
	}

	// 璁剧疆 Last-Modified 鏃堕棿
	if modTimeStr := tresp.Headers["Last-Modified"]; modTimeStr != "" {
		if modTime, pErr := time.Parse(time.RFC1123, modTimeStr); pErr == nil {
			req.Result.ModTime = modTime.Format(time.RFC3339Nano)
			_ = os.Chtimes(rPath, modTime, modTime)
		}
	}

	// 鏃ュ織锛氫笅杞藉畬鎴愪俊鎭?	if logWriter != nil {
		elapsed := time.Since(started)
		avgSpeed := float64(totalSize) / elapsed.Seconds()
		var speedUnit string
		speedVal := avgSpeed
		switch {
		case speedVal >= 1<<30:
			speedVal /= 1 << 30
			speedUnit = "GB/s"
		case speedVal >= 1<<20:
			speedVal /= 1 << 20
			speedUnit = "MB/s"
		case speedVal >= 1<<10:
			speedVal /= 1 << 10
			speedUnit = "KB/s"
		default:
			speedUnit = "B/s"
		}
		fmt.Fprintf(logWriter, "Download completed, total size: %d bytes\n", totalSize)
		fmt.Fprintf(logWriter, "Elapsed: %.2f s, average speed: %.2f %s\n",
			elapsed.Seconds(), speedVal, speedUnit)
	}

	return true, nil
}

// setReqMetadata 鍐欏叆 req.Metadata 骞惰Е鍙?OnMetadata 鍥炶皟锛堝鏈夛級锛?// 纭繚璋冪敤鏂硅兘绔嬪嵆鎸佷箙鍖栵紝閬垮厤 crash 绐楀彛瀵艰嚧 ETag/checksum 涓㈠け銆?func setReqMetadata(req *Request, key, value string) {
	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}
	req.Metadata[key] = value
	if req.OnMetadata != nil {
		req.OnMetadata(key, value)
	}
}

func (e *HTTPExtractor) buildHeaders(req *Request) map[string]string {
	h := make(map[string]string)
	if req.Headers != nil {
		maps.Copy(h, req.Headers)
	}
	if _, ok := h["User-Agent"]; !ok && e.ua != "" {
		h["User-Agent"] = e.ua
	}

	// 娉ㄥ叆 Chrome 椋庢牸娴忚鍣ㄦ爣澶达紙闄ら潪绂佺敤锛夛紝鐒跺悗鍦ㄦ渶鍚庣敤 req.Headers 瑕嗙洊
	if e.browserHdrs {
		browser := map[string]string{
			"Accept":             "*/*",
			"Cache-Control":      "no-cache",
			"Pragma":             "no-cache",
			"Priority":           "i",
			"Sec-Ch-Ua":          `"Google Chrome";v="143", "Chromium";v="143", "Not A(Brand";v="24"`,
			"Sec-Ch-Ua-Mobile":   "?0",
			"Sec-Ch-Ua-Platform": `"macOS"`,
			"Sec-Fetch-Dest":     "video",
			"Sec-Fetch-Mode":     "no-cors",
			"Sec-Fetch-Site":     "same-origin",
		}
		for k, v := range browser {
			if _, exists := h[k]; !exists {
				h[k] = v
			}
		}
	}

	// 濡傛灉涔嬪墠鏈?ETag 璁板綍锛岃缃?If-None-Match 鏉′欢璇锋眰澶?	if req.Metadata != nil {
		if etag := req.Metadata["etag"]; etag != "" {
			if _, has := h["If-None-Match"]; !has {
				h["If-None-Match"] = etag
			}
		}
	}
	return h
}
