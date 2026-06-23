// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/config"
	"github.com/cocomhub/download-manager/core"
	"github.com/cocomhub/download-manager/model"
	dlcore "github.com/cocomhub/download-manager/pkg/dlcore" //nolint:staticcheck // SA1019: needed for ErrNoTry comparison
)

// ================================================================
// Beacon: 鍙紪绋?HTTP 娴嬭瘯鏈嶅姟鍣?// ================================================================

// beaconHandler 瀹氫箟鍗曚釜绔偣鐨勫搷搴旇涓?type beaconHandler struct {
	statusCode int
	headers    map[string]string
	bodyFunc   func(r *http.Request) (int, map[string]string, []byte)
	body       []byte
}

// Beacon 鏄竴涓熀浜?httptest.Server 鐨勫彲缂栫▼ HTTP 鏈嶅姟鍣ㄣ€?// 鏀寔娉ㄥ唽棰勯厤缃殑澶勭悊鍣紝鑷姩璁板綍鎵€鏈夋敹鍒扮殑璇锋眰銆?type Beacon struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	handlers map[string]beaconHandler
	requests []*http.Request
}

// NewBeacon 鍒涘缓骞跺惎鍔ㄤ竴涓祴璇?HTTP 鏈嶅姟鍣ㄣ€?func NewBeacon(t *testing.T) *Beacon {
	t.Helper()
	b := &Beacon{
		t:        t,
		handlers: make(map[string]beaconHandler),
	}
	b.srv = httptest.NewServer(http.HandlerFunc(b.ServeHTTP))
	t.Cleanup(b.srv.Close)
	return b
}

// ServeHTTP 瀹炵幇 http.Handler銆?func (b *Beacon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 璁板綍璇锋眰
	reqCopy := r.Clone(context.Background())
	b.mu.Lock()
	b.requests = append(b.requests, reqCopy)
	b.mu.Unlock()

	// 鍖归厤 handler
	key := r.Method + " " + r.URL.Path
	b.mu.Lock()
	h, ok := b.handlers[key]
	b.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// 鍔ㄦ€佸搷搴?	if h.bodyFunc != nil {
		code, headers, body := h.bodyFunc(r)
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(code)
		if body != nil {
			w.Write(body)
		}
		return
	}

	// 闈欐€佸搷搴?	for k, v := range h.headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(h.statusCode)
	if h.body != nil {
		w.Write(h.body)
	}
}

// URL 杩斿洖鏈嶅姟鍣ㄥ熀纭€ URL銆?func (b *Beacon) URL() string { return b.srv.URL }

// Close 鍏抽棴鏈嶅姟鍣ㄣ€?func (b *Beacon) Close() { b.srv.Close() }

// Reset 娓呯┖璇锋眰璁板綍銆?func (b *Beacon) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.requests = nil
}

// Requests 杩斿洖鎵€鏈夋敹鍒扮殑璇锋眰鐨勫壇鏈€?func (b *Beacon) Requests() []*http.Request {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make([]*http.Request, len(b.requests))
	for i, r := range b.requests {
		result[i] = r.Clone(context.Background())
	}
	return result
}

// RequestCount 杩斿洖鏀跺埌鐨勮姹傛暟閲忋€?func (b *Beacon) RequestCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.requests)
}

// 鈥斺€斺€斺€斺€斺€?Handler 宸ュ巶 鈥斺€斺€斺€斺€斺€?
// HandleFile 娉ㄥ唽杩斿洖鍥哄畾鍐呭鐨?200 OK銆?func (b *Beacon) HandleFile(method, path, content, contentType string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[method+" "+path] = beaconHandler{
		statusCode: http.StatusOK,
		headers: map[string]string{
			"Content-Type":   contentType,
			"Content-Length": fmt.Sprintf("%d", len(content)),
		},
		body: []byte(content),
	}
}

// HandleRangeContent 娉ㄥ唽鏀寔 Range 璇锋眰鐨勬枃浠跺鐞嗗櫒銆?func (b *Beacon) HandleRangeContent(method, path, content string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	data := []byte(content)
	b.handlers[method+" "+path] = beaconHandler{
		bodyFunc: func(r *http.Request) (int, map[string]string, []byte) {
			rangeHeader := r.Header.Get("Range")
			if rangeHeader == "" {
				return http.StatusOK, map[string]string{
					"Content-Type":   "application/octet-stream",
					"Content-Length": fmt.Sprintf("%d", len(data)),
					"Accept-Ranges":  "bytes",
				}, data
			}
			// 瑙ｆ瀽 "bytes=N-"
			var start int
			if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-", &start); err != nil || start >= len(data) {
				return http.StatusRequestedRangeNotSatisfiable, map[string]string{
					"Content-Range": fmt.Sprintf("bytes */%d", len(data)),
				}, nil
			}
			partial := data[start:]
			return http.StatusPartialContent, map[string]string{
				"Content-Type":   "application/octet-stream",
				"Content-Length": fmt.Sprintf("%d", len(partial)),
				"Content-Range":  fmt.Sprintf("bytes %d-%d/%d", start, len(data)-1, len(data)),
				"Accept-Ranges":  "bytes",
			}, partial
		},
	}
}

// HandleError 娉ㄥ唽杩斿洖鎸囧畾鐘舵€佺爜鐨勯敊璇鐞嗗櫒銆?func (b *Beacon) HandleError(method, path string, statusCode int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[method+" "+path] = beaconHandler{
		statusCode: statusCode,
		body:       []byte(http.StatusText(statusCode)),
	}
}

// HandleWithMD5 娉ㄥ唽甯?MD5 鍝嶅簲澶寸殑鏂囦欢澶勭悊鍣ㄣ€?// md5Source: "X-Amz-Meta-Md5chksum" / "Etag" / "Content-MD5"
func (b *Beacon) HandleWithMD5(method, path, content, md5Header, md5Value string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[method+" "+path] = beaconHandler{
		statusCode: http.StatusOK,
		headers: map[string]string{
			"Content-Type":   "application/octet-stream",
			"Content-Length": fmt.Sprintf("%d", len(content)),
			md5Header:        md5Value,
		},
		body: []byte(content),
	}
}

// HandleTextContent 娉ㄥ唽杩斿洖 text/html 鐨勫鐞嗗櫒锛岀敤浜庢祴璇?Content-Type 妫€娴嬨€?func (b *Beacon) HandleTextContent(method, path string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	body := []byte("<html><body>not a video</body></html>")
	b.handlers[method+" "+path] = beaconHandler{
		statusCode: http.StatusOK,
		headers: map[string]string{
			"Content-Type":   "text/html; charset=utf-8",
			"Content-Length": fmt.Sprintf("%d", len(body)),
		},
		body: body,
	}
}

// HandleSlow 娉ㄥ唽鏈夊欢杩熺殑澶勭悊鍣ㄣ€?func (b *Beacon) HandleSlow(method, path, content string, delay time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[method+" "+path] = beaconHandler{
		bodyFunc: func(r *http.Request) (int, map[string]string, []byte) {
			time.Sleep(delay)
			return http.StatusOK, map[string]string{
				"Content-Type":   "text/plain",
				"Content-Length": fmt.Sprintf("%d", len(content)),
			}, []byte(content)
		},
	}
}

// HandleDynamic 娉ㄥ唽涓€涓嚜瀹氫箟 bodyFunc 澶勭悊鍣ㄣ€?func (b *Beacon) HandleDynamic(method, path string, fn func(r *http.Request) (int, map[string]string, []byte)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[method+" "+path] = beaconHandler{
		bodyFunc: fn,
	}
}

// ================================================================
// DownloadResult: 涓€娆′笅杞界殑璁板綍缁撴灉
// ================================================================

// DownloadResult 璁板綍涓€娆′笅杞界殑缁撴灉锛屽寘鎷敊璇€佸璞＄姸鎬佸拰鏂囦欢鍐呭銆?type DownloadResult struct {
	Err         error
	Obj         *model.DownloadObject
	FileContent []byte
	FileSize    int64
}

// ================================================================
// Comparator: 鍙屽疄鐜板姣旇繍琛屽櫒
// ================================================================

// ComparatorOptions 閰嶇疆 Comparator 鐨勯€夐」鍑芥暟銆?type ComparatorOptions struct {
	MaxRetries           int
	RootDir              string
	LogDir               string
	InjectBrowserHeaders bool
}

// ComparatorOption 鏄厤缃?Comparator 鐨勯€夐」鍑芥暟銆?type ComparatorOption func(*ComparatorOptions)

func WithMaxRetries(n int) ComparatorOption {
	return func(o *ComparatorOptions) { o.MaxRetries = n }
}

func WithInjectBrowserHeaders(v bool) ComparatorOption {
	return func(o *ComparatorOptions) { o.InjectBrowserHeaders = v }
}

// Comparator 瀵规瘮杩愯鍣紝鍚屾椂浣跨敤鏃э紙dlcore锛夊拰鏂帮紙pkg/download锛夊疄鐜?// 鎵ц涓嬭浇骞跺姣旇涓恒€?type Comparator struct {
	t       *testing.T
	beacon  *Beacon
	oldDL   core.Downloader
	newDL   core.Downloader
	rootDir string
}

// NewComparator 鍒涘缓瀵规瘮杩愯鍣紝鍚屾椂鏋勫缓鏃э紙dlcore锛夊拰鏂帮紙pkg/download锛変笅杞藉櫒銆?func NewComparator(t *testing.T, beacon *Beacon, opts ...ComparatorOption) *Comparator {
	t.Helper()
	var o ComparatorOptions
	for _, opt := range opts {
		opt(&o)
	}

	rootDir := o.RootDir
	if rootDir == "" {
		rootDir = t.TempDir()
	}

	// 鍩虹閰嶇疆
	// 娉ㄦ剰锛氫笉璁剧疆 LogDir銆侼ativeHTTPDownloader 浼氬皢 LogDir 閫氳繃 filepath.Join(rootDir, logDir) 鎷兼帴锛?	// 褰撲袱涓兘鏄?Windows 缁濆璺緞鏃朵細浜х敓闈炴硶璺緞銆?	// 闇€瑕佷娇鐢ㄦ棩蹇楃殑娴嬭瘯搴旇烦杩囨垨鐩存帴鏋勯€?NativeHTTPDownloader銆?	baseCfg := config.Downloader{
		MaxRetries: 3,
		Filesystem: config.DcFilesystem{
			RootDir: rootDir,
		},
		HTTP: config.DcHTTP{
			TimeoutSeconds:                  30,
			DefaultUserAgent:                "TestAgent/1.0",
			DisableInjectBrowserLikeHeaders: !o.InjectBrowserHeaders,
		},
		Progress: config.DcProgress{
			MinPercentStep:     0.1,
			MaxIntervalSeconds: 1,
		},
	}

	// 鏃ц矾寰勶細native_old 鈫?dlcore
	cfgOld := baseCfg
	cfgOld.Type = "native_old"
	oldDL := NewNativeHTTPDownloader(cfgOld)

	// 鏂拌矾寰勶細native 鈫?pkg/download 鈫?DownloaderAdapter
	cfgNew := baseCfg
	cfgNew.Type = "native"
	newDL := New(cfgNew)

	return &Comparator{
		t:       t,
		beacon:  beacon,
		oldDL:   oldDL,
		newDL:   newDL,
		rootDir: rootDir,
	}
}

// Check 鏄姣旀柇瑷€鍑芥暟銆?type Check func(t *testing.T, old, new *DownloadResult)

// Run 鐢ㄦ棫瀹炵幇鍜屾柊瀹炵幇鍒嗗埆鎵ц涓嬭浇锛岀劧鍚庤繍琛屾墍鏈?check 鏂█銆?func (c *Comparator) Run(name string, obj *model.DownloadObject, headers map[string]string, checks ...Check) {
	c.t.Run(name, func(t *testing.T) {
		// 涓烘瘡涓疄鐜板垱寤虹嫭绔嬬殑 obj 鍓湰锛岄伩鍏嶅叡浜姸鎬?		oldObj := copyObject(obj)
		newObj := copyObject(obj)

		// 杩愯鏃у疄鐜?		var oldResult DownloadResult
		oldResult.Obj = oldObj
		oldResult.Err = c.oldDL.Download(oldObj, headers)
		collectFileResult(t, c.rootDir, &oldResult)

		// 杩愯鏂板疄鐜?		var newResult DownloadResult
		newResult.Obj = newObj
		newResult.Err = c.newDL.Download(newObj, headers)
		collectFileResult(t, c.rootDir, &newResult)

		// 鎵ц鎵€鏈夋柇瑷€
		for i, check := range checks {
			if check == nil {
				continue
			}
			check(t, &oldResult, &newResult)
			if t.Failed() {
				t.Logf("check %d/%d failed for test %q", i+1, len(checks), name)
				return
			}
		}
	})
}

// DlcoreOnlyRun 浠呰繍琛屾棫瀹炵幇锛坉lcore锛夌殑涓嬭浇锛岃褰曟柊瀹炵幇鐨勫弬鑰冭涓恒€?// name 鏄祴璇曞悕锛屼細鑷姩娣诲姞 "[dlcore-only]" 鍚庣紑銆?// checks 浣跨敤鏃㈡湁 Check 绫诲瀷锛屽湪鍐呴儴灏?newResult 浣滀负绗簩涓弬鏁颁紶鍏ャ€?func (c *Comparator) DlcoreOnlyRun(t *testing.T, name string, obj *model.DownloadObject, headers map[string]string, checks ...Check) {
	t.Run(name+"_[dlcore-only]", func(t *testing.T) {
		// 杩愯鏃у疄鐜?		oldObj := copyObject(obj)
		var oldResult DownloadResult
		oldResult.Obj = oldObj
		oldResult.Err = c.oldDL.Download(oldObj, headers)
		collectFileResult(t, c.rootDir, &oldResult)
		t.Logf("dlcore result: err=%v, size=%d, metadata=%v", oldResult.Err, oldResult.FileSize, oldResult.Obj.Metadata)

		// 杩愯鏂板疄鐜拌褰曞弬鑰?		newObj := copyObject(obj)
		var newResult DownloadResult
		newResult.Obj = newObj
		newResult.Err = c.newDL.Download(newObj, headers)
		collectFileResult(t, c.rootDir, &newResult)
		t.Logf("pkg/download reference: err=%v, size=%d, metadata=%v", newResult.Err, newResult.FileSize, newResult.Obj.Metadata)

		// 鎵ц dlcore-only 鏂█
		for i, check := range checks {
			if check == nil {
				continue
			}
			check(t, &oldResult, &newResult)
			if t.Failed() {
				t.Logf("dlcore-only check %d/%d failed", i+1, len(checks))
				return
			}
		}
	})
}

// copyObject 娣卞害鎷疯礉 DownloadObject 鐢ㄤ簬闅旂娴嬭瘯銆?func copyObject(src *model.DownloadObject) *model.DownloadObject {
	dst := &model.DownloadObject{
		TaskID:   src.TaskID,
		URL:      src.URL,
		SavePath: src.SavePath,
		Status:   src.Status,
		Progress: src.Progress,
	}
	if src.Metadata != nil {
		dst.Metadata = make(map[string]string, len(src.Metadata))
		maps.Copy(dst.Metadata, src.Metadata)
	}
	if src.Extra != nil {
		dst.Extra = make(map[string]any, len(src.Extra))
		maps.Copy(dst.Extra, src.Extra)
	}
	return dst
}

// collectFileResult 璇诲彇涓嬭浇鍚庣殑鏂囦欢鍐呭銆?func collectFileResult(t *testing.T, rootDir string, r *DownloadResult) {
	t.Helper()
	path := filepath.Join(rootDir, r.Obj.SavePath)
	data, err := os.ReadFile(path)
	if err == nil {
		r.FileContent = data
		r.FileSize = int64(len(data))
	}
}

// ================================================================
// 棰勭疆 Check 鍑芥暟
// ================================================================

// CheckError 楠岃瘉閿欒绫诲瀷涓€鑷达紙閮?nil / 閮?ErrNoTry / 閮介潪 nil锛夈€?func CheckError() Check {
	return func(t *testing.T, old, new *DownloadResult) {
		t.Helper()
		if (old.Err == nil) != (new.Err == nil) {
			t.Errorf("error presence mismatch: old=%v, new=%v", old.Err, new.Err)
			return
		}
		if old.Err == nil {
			return
		}
		// 閮介潪 nil 鈥?妫€鏌ユ槸鍚﹂兘涓?ErrNoTry
		// dlcore.ErrNoTry 宸插鐢?pkg/download.ErrNoTry锛屽悓涓€ sentinel
		oldNoTry := errors.Is(old.Err, dlcore.ErrNoTry)
		newNoTry := errors.Is(new.Err, dlcore.ErrNoTry)
		if oldNoTry != newNoTry {
			t.Errorf("ErrNoTry mismatch: old.IsNoTry=%v, new.IsNoTry=%v (old=%v, new=%v)", oldNoTry, newNoTry, old.Err, new.Err)
		}
	}
}

// CheckFileBytes 楠岃瘉鏂囦欢鍐呭瀹屽叏涓€鑷淬€?func CheckFileBytes() Check {
	return func(t *testing.T, old, new *DownloadResult) {
		t.Helper()
		if len(old.FileContent) == 0 && len(new.FileContent) == 0 {
			return
		}
		if string(old.FileContent) != string(new.FileContent) {
			t.Errorf("file content mismatch:\n old(%d): %q\n new(%d): %q",
				len(old.FileContent), old.FileContent,
				len(new.FileContent), new.FileContent)
		}
	}
}

// CheckFileSize 楠岃瘉鏂囦欢澶у皬涓€鑷淬€?func CheckFileSize() Check {
	return func(t *testing.T, old, new *DownloadResult) {
		t.Helper()
		if old.FileSize != new.FileSize {
			t.Errorf("file size mismatch: old=%d, new=%d", old.FileSize, new.FileSize)
		}
	}
}

// CheckMetadata 楠岃瘉鎸囧畾 key 鍦?Metadata 涓瓨鍦ㄤ笖鍊间竴鑷淬€?func CheckMetadata(keys ...string) Check {
	return func(t *testing.T, old, new *DownloadResult) {
		t.Helper()
		for _, key := range keys {
			oldVal, oldOK := old.Obj.Metadata[key]
			newVal, newOK := new.Obj.Metadata[key]
			if !oldOK && !newOK {
				continue // 鍙屾柟閮芥病鏈夛紝鍏佽
			}
			if oldVal != newVal {
				t.Errorf("Metadata[%q] mismatch: old=%q, new=%q", key, oldVal, newVal)
			}
		}
	}
}

// CheckProgressEnd 楠岃瘉鏈€缁堣繘搴︿负 100銆?func CheckProgressEnd() Check {
	return func(t *testing.T, old, new *DownloadResult) {
		t.Helper()
		if old.Obj.Progress != new.Obj.Progress {
			t.Errorf("progress mismatch: old=%d, new=%d", old.Obj.Progress, new.Obj.Progress)
			return
		}
		if old.Obj.Progress != 100 {
			t.Errorf("progress not 100 (old=%d, new=%d)", old.Obj.Progress, new.Obj.Progress)
		}
	}
}

// CheckAnyError 楠岃瘉鏂版棫閮借繑鍥?error锛堜笉瑕佹眰鍏蜂綋 error 涓€鑷达級銆?func CheckAnyError() Check {
	return func(t *testing.T, old, new *DownloadResult) {
		t.Helper()
		if old.Err == nil {
			t.Error("old: expected error, got nil")
		}
		if new.Err == nil {
			t.Error("new: expected error, got nil")
		}
	}
}

// CheckBothNil 楠岃瘉鏂版棫閮借繑鍥?nil error锛堥兘鎴愬姛锛夈€?func CheckBothNil() Check {
	return func(t *testing.T, old, new *DownloadResult) {
		t.Helper()
		if old.Err != nil {
			t.Errorf("old: expected nil error, got %v", old.Err)
		}
		if new.Err != nil {
			t.Errorf("new: expected nil error, got %v", new.Err)
		}
	}
}

// CheckErrNoTry 楠岃瘉鍙屾柟閿欒閮藉寘鍚?ErrNoTry銆?func CheckErrNoTry() Check {
	return func(t *testing.T, old, new *DownloadResult) {
		t.Helper()
		// dlcore.ErrNoTry 宸插鐢?pkg/download.ErrNoTry锛屽悓涓€ sentinel
		oldIsNoTry := errors.Is(old.Err, dlcore.ErrNoTry)
		newIsNoTry := errors.Is(new.Err, dlcore.ErrNoTry)
		if !oldIsNoTry {
			t.Errorf("old: expected ErrNoTry, got %v", old.Err)
		}
		if !newIsNoTry {
			t.Errorf("new: expected ErrNoTry, got %v", new.Err)
		}
	}
}

// CheckBothNoTry 楠岃瘉鍙屾柟閮借繑鍥?ErrNoTry 涓旀枃浠朵笉瀛樺湪銆?func CheckBothNoTry() Check {
	base := CheckErrNoTry()
	return func(t *testing.T, old, new *DownloadResult) {
		base(t, old, new)
		if len(old.FileContent) > 0 {
			t.Errorf("old: expected no file on ErrNoTry, got %d bytes", len(old.FileContent))
		}
		if len(new.FileContent) > 0 {
			t.Errorf("new: expected no file on ErrNoTry, got %d bytes", len(new.FileContent))
		}
	}
}

// CheckMetadataAbsent 楠岃瘉鎸囧畾 key 鍦ㄥ弻鏂?Metadata 涓兘涓嶅瓨鍦ㄣ€?func CheckMetadataAbsent(keys ...string) Check {
	return func(t *testing.T, old, new *DownloadResult) {
		t.Helper()
		for _, key := range keys {
			if _, ok := old.Obj.Metadata[key]; ok {
				t.Errorf("old: Metadata[%q] should be absent, got %q", key, old.Obj.Metadata[key])
			}
			if _, ok := new.Obj.Metadata[key]; ok {
				t.Errorf("new: Metadata[%q] should be absent, got %q", key, new.Obj.Metadata[key])
			}
		}
	}
}

// ================================================================
// 娴嬭瘯瀵硅薄宸ュ巶
// ================================================================

// makeTestObject 鍒涘缓娴嬭瘯鐢?DownloadObject銆?func makeTestObject(url, savePath string, metadata map[string]string, extra map[string]any) *model.DownloadObject {
	obj := &model.DownloadObject{
		TaskID:   "test-task",
		URL:      url,
		SavePath: savePath,
		Metadata: metadata,
		Extra:    extra,
	}
	if obj.Metadata == nil {
		obj.Metadata = make(map[string]string)
	}
	return obj
}

// ================================================================
// Beacon 鑷祴
// ================================================================

func TestBeacon_Basic(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/test.txt", "hello", "text/plain")

	resp, err := http.Get(b.URL() + "/test.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello" {
		t.Errorf("got %q, want %q", string(body), "hello")
	}
	if b.RequestCount() != 1 {
		t.Errorf("expected 1 request, got %d", b.RequestCount())
	}
}

func TestBeacon_Range(t *testing.T) {
	b := NewBeacon(t)
	b.HandleRangeContent("GET", "/file.bin", "0123456789")

	// 鏃?Range 璇锋眰
	resp, _ := http.Get(b.URL() + "/file.bin")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "0123456789" {
		t.Errorf("full content: got %q", string(body))
	}

	// Range 璇锋眰
	req, _ := http.NewRequest("GET", b.URL()+"/file.bin", nil)
	req.Header.Set("Range", "bytes=5-")
	resp, _ = http.DefaultClient.Do(req)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "56789" {
		t.Errorf("range content: got %q, want %q", string(body), "56789")
	}
}

func TestBeacon_Error(t *testing.T) {
	b := NewBeacon(t)
	b.HandleError("GET", "/err", http.StatusNotFound)

	resp, err := http.Get(b.URL() + "/err")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestBeacon_Reset(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/a.txt", "a", "text/plain")

	http.Get(b.URL() + "/a.txt")
	if b.RequestCount() != 1 {
		t.Errorf("expected 1 request, got %d", b.RequestCount())
	}

	b.Reset()
	if b.RequestCount() != 0 {
		t.Errorf("expected 0 after reset, got %d", b.RequestCount())
	}
}

func TestComparator_BasicDownload(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/hello.txt", "Hello, World!", "text/plain")

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/hello.txt", "out/hello.txt", nil, nil)
	cmp.Run("basic", obj, nil, CheckBothNil(), CheckFileBytes(), CheckFileSize())
}

func TestComparator_NilHeaders(t *testing.T) {
	b := NewBeacon(t)
	b.HandleFile("GET", "/nil.txt", "data", "text/plain")

	cmp := NewComparator(t, b)
	obj := makeTestObject(b.URL()+"/nil.txt", "nil.txt", nil, nil)
	cmp.Run("nil-headers", obj, nil, CheckBothNil(), CheckFileBytes())
}
