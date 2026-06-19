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
	dlcore "github.com/cocomhub/download-manager/pkg/dlcore"        //nolint:staticcheck // SA1019: needed for ErrNoTry comparison
	pkgdownload "github.com/cocomhub/download-manager/pkg/download" // needed for ErrNoTry comparison (new path)
)

// ================================================================
// Beacon: 可编程 HTTP 测试服务器
// ================================================================

// beaconHandler 定义单个端点的响应行为
type beaconHandler struct {
	statusCode int
	headers    map[string]string
	bodyFunc   func(r *http.Request) (int, map[string]string, []byte)
	body       []byte
}

// Beacon 是一个基于 httptest.Server 的可编程 HTTP 服务器。
// 支持注册预配置的处理器，自动记录所有收到的请求。
type Beacon struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	handlers map[string]beaconHandler
	requests []*http.Request
}

// NewBeacon 创建并启动一个测试 HTTP 服务器。
func NewBeacon(t *testing.T) *Beacon {
	t.Helper()
	b := &Beacon{
		t:        t,
		handlers: make(map[string]beaconHandler),
	}
	b.srv = httptest.NewServer(http.HandlerFunc(b.ServeHTTP))
	t.Cleanup(b.srv.Close)
	return b
}

// ServeHTTP 实现 http.Handler。
func (b *Beacon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 记录请求
	reqCopy := r.Clone(context.Background())
	b.mu.Lock()
	b.requests = append(b.requests, reqCopy)
	b.mu.Unlock()

	// 匹配 handler
	key := r.Method + " " + r.URL.Path
	b.mu.Lock()
	h, ok := b.handlers[key]
	b.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// 动态响应
	if h.bodyFunc != nil {
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

	// 静态响应
	for k, v := range h.headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(h.statusCode)
	if h.body != nil {
		w.Write(h.body)
	}
}

// URL 返回服务器基础 URL。
func (b *Beacon) URL() string { return b.srv.URL }

// Close 关闭服务器。
func (b *Beacon) Close() { b.srv.Close() }

// Reset 清空请求记录。
func (b *Beacon) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.requests = nil
}

// Requests 返回所有收到的请求的副本。
func (b *Beacon) Requests() []*http.Request {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make([]*http.Request, len(b.requests))
	for i, r := range b.requests {
		result[i] = r.Clone(context.Background())
	}
	return result
}

// RequestCount 返回收到的请求数量。
func (b *Beacon) RequestCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.requests)
}

// —————— Handler 工厂 ——————

// HandleFile 注册返回固定内容的 200 OK。
func (b *Beacon) HandleFile(method, path, content, contentType string) {
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

// HandleRangeContent 注册支持 Range 请求的文件处理器。
func (b *Beacon) HandleRangeContent(method, path, content string) {
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
			// 解析 "bytes=N-"
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

// HandleError 注册返回指定状态码的错误处理器。
func (b *Beacon) HandleError(method, path string, statusCode int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[method+" "+path] = beaconHandler{
		statusCode: statusCode,
		body:       []byte(http.StatusText(statusCode)),
	}
}

// HandleWithMD5 注册带 MD5 响应头的文件处理器。
// md5Source: "X-Amz-Meta-Md5chksum" / "Etag" / "Content-MD5"
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

// HandleTextContent 注册返回 text/html 的处理器，用于测试 Content-Type 检测。
func (b *Beacon) HandleTextContent(method, path string) {
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

// HandleSlow 注册有延迟的处理器。
func (b *Beacon) HandleSlow(method, path, content string, delay time.Duration) {
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

// HandleDynamic 注册一个自定义 bodyFunc 处理器。
func (b *Beacon) HandleDynamic(method, path string, fn func(r *http.Request) (int, map[string]string, []byte)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[method+" "+path] = beaconHandler{
		bodyFunc: fn,
	}
}

// ================================================================
// DownloadResult: 一次下载的记录结果
// ================================================================

// DownloadResult 记录一次下载的结果，包括错误、对象状态和文件内容。
type DownloadResult struct {
	Err         error
	Obj         *model.DownloadObject
	FileContent []byte
	FileSize    int64
}

// ================================================================
// Comparator: 双实现对比运行器
// ================================================================

// ComparatorOptions 配置 Comparator 的选项函数。
type ComparatorOptions struct {
	MaxRetries           int
	RootDir              string
	LogDir               string
	InjectBrowserHeaders bool
}

// ComparatorOption 是配置 Comparator 的选项函数。
type ComparatorOption func(*ComparatorOptions)

func WithMaxRetries(n int) ComparatorOption {
	return func(o *ComparatorOptions) { o.MaxRetries = n }
}

func WithInjectBrowserHeaders(v bool) ComparatorOption {
	return func(o *ComparatorOptions) { o.InjectBrowserHeaders = v }
}

// Comparator 对比运行器，同时使用旧（dlcore）和新（pkg/download）实现
// 执行下载并对比行为。
type Comparator struct {
	t       *testing.T
	beacon  *Beacon
	oldDL   core.Downloader
	newDL   core.Downloader
	rootDir string
}

// NewComparator 创建对比运行器，同时构建旧（dlcore）和新（pkg/download）下载器。
func NewComparator(t *testing.T, beacon *Beacon, opts ...ComparatorOption) *Comparator {
	t.Helper()
	var o ComparatorOptions
	for _, opt := range opts {
		opt(&o)
	}

	rootDir := o.RootDir
	if rootDir == "" {
		rootDir = t.TempDir()
	}

	// 基础配置
	// 注意：不设置 LogDir。NativeHTTPDownloader 会将 LogDir 通过 filepath.Join(rootDir, logDir) 拼接，
	// 当两个都是 Windows 绝对路径时会产生非法路径。
	// 需要使用日志的测试应跳过或直接构造 NativeHTTPDownloader。
	baseCfg := config.Downloader{
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

	// 旧路径：native_old → dlcore
	cfgOld := baseCfg
	cfgOld.Type = "native_old"
	oldDL := NewNativeHTTPDownloader(cfgOld)

	// 新路径：native → pkg/download → DownloaderAdapter
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

// Check 是对比断言函数。
type Check func(t *testing.T, old, new *DownloadResult)

// Run 用旧实现和新实现分别执行下载，然后运行所有 check 断言。
func (c *Comparator) Run(name string, obj *model.DownloadObject, headers map[string]string, checks ...Check) {
	c.t.Run(name, func(t *testing.T) {
		// 为每个实现创建独立的 obj 副本，避免共享状态
		oldObj := copyObject(obj)
		newObj := copyObject(obj)

		// 运行旧实现
		var oldResult DownloadResult
		oldResult.Obj = oldObj
		oldResult.Err = c.oldDL.Download(oldObj, headers)
		collectFileResult(t, c.rootDir, &oldResult)

		// 运行新实现
		var newResult DownloadResult
		newResult.Obj = newObj
		newResult.Err = c.newDL.Download(newObj, headers)
		collectFileResult(t, c.rootDir, &newResult)

		// 执行所有断言
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

// DlcoreOnlyRun 仅运行旧实现（dlcore）的下载，记录新实现的参考行为。
// name 是测试名，会自动添加 "[dlcore-only]" 后缀。
// checks 使用既有 Check 类型，在内部将 newResult 作为第二个参数传入。
func (c *Comparator) DlcoreOnlyRun(t *testing.T, name string, obj *model.DownloadObject, headers map[string]string, checks ...Check) {
	t.Run(name+"_[dlcore-only]", func(t *testing.T) {
		// 运行旧实现
		oldObj := copyObject(obj)
		var oldResult DownloadResult
		oldResult.Obj = oldObj
		oldResult.Err = c.oldDL.Download(oldObj, headers)
		collectFileResult(t, c.rootDir, &oldResult)
		t.Logf("dlcore result: err=%v, size=%d, metadata=%v", oldResult.Err, oldResult.FileSize, oldResult.Obj.Metadata)

		// 运行新实现记录参考
		newObj := copyObject(obj)
		var newResult DownloadResult
		newResult.Obj = newObj
		newResult.Err = c.newDL.Download(newObj, headers)
		collectFileResult(t, c.rootDir, &newResult)
		t.Logf("pkg/download reference: err=%v, size=%d, metadata=%v", newResult.Err, newResult.FileSize, newResult.Obj.Metadata)

		// 执行 dlcore-only 断言
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

// copyObject 深度拷贝 DownloadObject 用于隔离测试。
func copyObject(src *model.DownloadObject) *model.DownloadObject {
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

// collectFileResult 读取下载后的文件内容。
func collectFileResult(t *testing.T, rootDir string, r *DownloadResult) {
	t.Helper()
	path := filepath.Join(rootDir, r.Obj.SavePath)
	data, err := os.ReadFile(path)
	if err == nil {
		r.FileContent = data
		r.FileSize = int64(len(data))
	}
}

// ================================================================
// 预置 Check 函数
// ================================================================

// CheckError 验证错误类型一致（都 nil / 都 ErrNoTry / 都非 nil）。
func CheckError() Check {
	return func(t *testing.T, old, new *DownloadResult) {
		t.Helper()
		if (old.Err == nil) != (new.Err == nil) {
			t.Errorf("error presence mismatch: old=%v, new=%v", old.Err, new.Err)
			return
		}
		if old.Err == nil {
			return
		}
		// 都非 nil — 检查是否都为 ErrNoTry
		// 注意：旧路径（dlcore）使用 dlcore.ErrNoTry，新路径（pkg/download）使用 pkgdownload.ErrNoTry
		// 分别检查各自的 sentinel。
		oldNoTry := errors.Is(old.Err, pkgdownload.ErrNoTry) || errors.Is(old.Err, dlcore.ErrNoTry)
		newNoTry := errors.Is(new.Err, pkgdownload.ErrNoTry) || errors.Is(new.Err, dlcore.ErrNoTry)
		if oldNoTry != newNoTry {
			t.Errorf("ErrNoTry mismatch: old.IsNoTry=%v, new.IsNoTry=%v (old=%v, new=%v)", oldNoTry, newNoTry, old.Err, new.Err)
		}
	}
}

// CheckFileBytes 验证文件内容完全一致。
func CheckFileBytes() Check {
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

// CheckFileSize 验证文件大小一致。
func CheckFileSize() Check {
	return func(t *testing.T, old, new *DownloadResult) {
		t.Helper()
		if old.FileSize != new.FileSize {
			t.Errorf("file size mismatch: old=%d, new=%d", old.FileSize, new.FileSize)
		}
	}
}

// CheckMetadata 验证指定 key 在 Metadata 中存在且值一致。
func CheckMetadata(keys ...string) Check {
	return func(t *testing.T, old, new *DownloadResult) {
		t.Helper()
		for _, key := range keys {
			oldVal, oldOK := old.Obj.Metadata[key]
			newVal, newOK := new.Obj.Metadata[key]
			if !oldOK && !newOK {
				continue // 双方都没有，允许
			}
			if oldVal != newVal {
				t.Errorf("Metadata[%q] mismatch: old=%q, new=%q", key, oldVal, newVal)
			}
		}
	}
}

// CheckProgressEnd 验证最终进度为 100。
func CheckProgressEnd() Check {
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

// CheckAnyError 验证新旧都返回 error（不要求具体 error 一致）。
func CheckAnyError() Check {
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

// CheckBothNil 验证新旧都返回 nil error（都成功）。
func CheckBothNil() Check {
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

// CheckErrNoTry 验证双方错误都包含 ErrNoTry。
func CheckErrNoTry() Check {
	return func(t *testing.T, old, new *DownloadResult) {
		t.Helper()
		// 检查旧路径：可能是 dlcore.ErrNoTry 或 pkgdownload.ErrNoTry（旧路径 native.go 也使用 dlcore.ErrNoTry）
		oldIsNoTry := errors.Is(old.Err, dlcore.ErrNoTry)
		newIsNoTry := errors.Is(new.Err, pkgdownload.ErrNoTry) || errors.Is(new.Err, dlcore.ErrNoTry)
		if !oldIsNoTry {
			t.Errorf("old: expected ErrNoTry, got %v", old.Err)
		}
		if !newIsNoTry {
			t.Errorf("new: expected ErrNoTry, got %v", new.Err)
		}
	}
}

// CheckBothNoTry 验证双方都返回 ErrNoTry 且文件不存在。
func CheckBothNoTry() Check {
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

// CheckMetadataAbsent 验证指定 key 在双方 Metadata 中都不存在。
func CheckMetadataAbsent(keys ...string) Check {
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
// 测试对象工厂
// ================================================================

// makeTestObject 创建测试用 DownloadObject。
func makeTestObject(url, savePath string, metadata map[string]string, extra map[string]any) *model.DownloadObject {
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
// Beacon 自测
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

	// 无 Range 请求
	resp, _ := http.Get(b.URL() + "/file.bin")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "0123456789" {
		t.Errorf("full content: got %q", string(body))
	}

	// Range 请求
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
