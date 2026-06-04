package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// ---- compile-time interface checks ----

type mockExtractor struct{}

func (m *mockExtractor) Name() string                                 { return "mock" }
func (m *mockExtractor) Match(_ context.Context, _ string) bool       { return false }
func (m *mockExtractor) Extract(_ context.Context, _ *Request) error  { return nil }

type mockTransport struct{}

func (m *mockTransport) Name() string                                               { return "mock" }
func (m *mockTransport) RoundTrip(_ context.Context, _ *TransportRequest) (*TransportResponse, error) {
	return nil, nil
}

type mockProxySelector struct{}

func (m *mockProxySelector) Select(_ context.Context, _ string, _ *DownloadHint) (string, error) {
	return "", nil
}

type mockSelector struct{}

func (m *mockSelector) MatchExtractor(_ context.Context, _ string, _ *DownloadHint) Extractor { return nil }
func (m *mockSelector) SelectProxy(_ context.Context, _ string, _ *DownloadHint) (string, error) {
	return "", nil
}

var (
	_ Extractor      = (*mockExtractor)(nil)
	_ Transport      = (*mockTransport)(nil)
	_ ProxySelector  = (*mockProxySelector)(nil)
	_ Selector       = (*mockSelector)(nil)
)

// ---- ErrNoTry ----

func TestErrNoTry(t *testing.T) {
	if !IsNoTry(ErrNoTry) {
		t.Error("IsNoTry(ErrNoTry) should be true")
	}
	if IsNoTry(io.EOF) {
		t.Error("IsNoTry(io.EOF) should be false")
	}
}

// ---- Request ----

func TestRequestStruct(t *testing.T) {
	progressCalled := false
	req := &Request{
		URL:           "https://example.com/file.zip",
		SavePath:      "/tmp/file.zip",
		Headers:       map[string]string{"Authorization": "Bearer token"},
		TrackProgress: true,
		OnProgress: func(progress float64, downloaded, total int64) {
			progressCalled = true
		},
		Metadata: map[string]string{"key": "val"},
		Hint:     nil,
	}
	if req.URL != "https://example.com/file.zip" {
		t.Errorf("unexpected URL: %s", req.URL)
	}
	if req.SavePath != "/tmp/file.zip" {
		t.Errorf("unexpected SavePath: %s", req.SavePath)
	}
	if req.Headers["Authorization"] != "Bearer token" {
		t.Errorf("unexpected Header")
	}
	if !req.TrackProgress {
		t.Error("TrackProgress should be true")
	}
	req.OnProgress(50, 500, 1000)
	if !progressCalled {
		t.Error("OnProgress should have been called")
	}
	if req.Metadata["key"] != "val" {
		t.Errorf("unexpected Metadata")
	}
}

func TestRequestProgressCallback(t *testing.T) {
	var p float64
	var d int64
	req := &Request{
		URL:           "https://example.com/bigfile.bin",
		TrackProgress: true,
		OnProgress: func(progress float64, downloaded, _ int64) {
			p = progress
			d = downloaded
		},
	}
	req.OnProgress(0.5, 512, 1024)
	if p != 0.5 {
		t.Errorf("expected progress 0.5, got %f", p)
	}
	if d != 512 {
		t.Errorf("expected downloaded 512, got %d", d)
	}
}

// ---- DownloadHint ----

func TestDownloadHintStruct(t *testing.T) {
	hint := &DownloadHint{
		FileSize:    1024,
		ContentType: "application/zip",
		Extractor:   "native",
		Tags:        map[string]string{"quality": "high"},
	}
	if hint.FileSize != 1024 {
		t.Errorf("unexpected FileSize: %d", hint.FileSize)
	}
	if hint.ContentType != "application/zip" {
		t.Errorf("unexpected ContentType: %s", hint.ContentType)
	}
	if hint.Extractor != "native" {
		t.Errorf("unexpected Extractor: %s", hint.Extractor)
	}
	if hint.Tags["quality"] != "high" {
		t.Errorf("unexpected Tag")
	}
}

// ---- TransportRequest / TransportResponse ----

func TestTransportRequestStruct(t *testing.T) {
	treq := &TransportRequest{
		URL:     "https://cdn.example.com/file.bin",
		Method:  "GET",
		Headers: map[string]string{"Range": "bytes=0-1023"},
		Body:    []byte("hello"),
		Range:   &RangeRequest{Offset: 0},
		ProxyURL: "http://proxy:8080",
	}
	if treq.URL != "https://cdn.example.com/file.bin" {
		t.Errorf("unexpected URL")
	}
	if treq.Method != "GET" {
		t.Errorf("unexpected Method")
	}
	if string(treq.Body) != "hello" {
		t.Errorf("unexpected Body")
	}
	if treq.ProxyURL != "http://proxy:8080" {
		t.Errorf("unexpected ProxyURL")
	}
}

func TestTransportResponseStruct(t *testing.T) {
	body := io.NopCloser(strings.NewReader("response body"))
	tresp := &TransportResponse{
		Body:          body,
		StatusCode:    200,
		ContentLength: 13,
		Headers:       map[string]string{"Content-Type": "application/octet-stream"},
		ProxyURL:      "http://proxy:8080",
	}
	if tresp.StatusCode != 200 {
		t.Errorf("unexpected StatusCode: %d", tresp.StatusCode)
	}
	if tresp.ContentLength != 13 {
		t.Errorf("unexpected ContentLength: %d", tresp.ContentLength)
	}
	if tresp.Headers["Content-Type"] != "application/octet-stream" {
		t.Errorf("unexpected Content-Type header")
	}
	// verify body is readable
	data, err := io.ReadAll(tresp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if string(data) != "response body" {
		t.Errorf("unexpected body content: %s", string(data))
	}
	tresp.Body.Close()
}

// ---- RangeRequest ----

func TestRangeRequestStruct(t *testing.T) {
	rr := &RangeRequest{Offset: 1024}
	if rr.Offset != 1024 {
		t.Errorf("unexpected Offset: %d", rr.Offset)
	}
}

// ---- Extractor interface mock usage ----

func TestExtractorInterface(t *testing.T) {
	e := &mockExtractor{}
	if e.Name() != "mock" {
		t.Errorf("unexpected name: %s", e.Name())
	}
	if e.Match(context.Background(), "http://example.com") {
		t.Error("Match should return false")
	}
	if err := e.Extract(context.Background(), &Request{URL: "http://example.com"}); err != nil {
		t.Errorf("Extract should not error: %v", err)
	}
}

// ---- Transport interface mock usage ----

func TestTransportInterface(t *testing.T) {
	tr := &mockTransport{}
	if tr.Name() != "mock" {
		t.Errorf("unexpected name: %s", tr.Name())
	}
	resp, err := tr.RoundTrip(context.Background(), &TransportRequest{URL: "http://example.com"})
	if err != nil {
		t.Errorf("RoundTrip should not error: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response, got %+v", resp)
	}
}

// ---- ProxySelector interface mock usage ----

func TestProxySelectorInterface(t *testing.T) {
	ps := &mockProxySelector{}
	proxy, err := ps.Select(context.Background(), "http://example.com", nil)
	if err != nil {
		t.Errorf("Select should not error: %v", err)
	}
	if proxy != "" {
		t.Errorf("expected empty proxy, got %s", proxy)
	}
}

// ---- Selector interface mock usage ----

func TestSelectorInterface(t *testing.T) {
	s := &mockSelector{}
	ext := s.MatchExtractor(context.Background(), "http://example.com", nil)
	if ext != nil {
		t.Errorf("expected nil extractor, got %+v", ext)
	}
	proxy, err := s.SelectProxy(context.Background(), "http://example.com", nil)
	if err != nil {
		t.Errorf("SelectProxy should not error: %v", err)
	}
	if proxy != "" {
		t.Errorf("expected empty proxy, got %s", proxy)
	}
}

// ---- Edge cases ----

func TestRequestNilOnProgress(t *testing.T) {
	req := &Request{
		URL:           "http://example.com/file",
		TrackProgress: true,
		OnProgress:    nil,
	}
	// Should not panic when OnProgress is nil
	if req.OnProgress != nil {
		t.Error("OnProgress should be nil")
	}
}

func TestTransportRequestNilRange(t *testing.T) {
	treq := &TransportRequest{
		URL:    "http://example.com/file",
		Method: "GET",
		Range:  nil,
	}
	if treq.Range != nil {
		t.Error("Range should be nil")
	}
}

func TestTransportResponseNilBody(t *testing.T) {
	tresp := &TransportResponse{
		Body:          nil,
		StatusCode:    404,
		ContentLength: 0,
	}
	if tresp.Body != nil {
		t.Error("Body should be nil")
	}
	if tresp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", tresp.StatusCode)
	}
}

func TestDownloadHintNilTags(t *testing.T) {
	hint := &DownloadHint{
		FileSize:    0,
		ContentType: "",
		Extractor:   "",
		Tags:        nil,
	}
	if hint.Tags != nil {
		t.Error("Tags should be nil")
	}
}

func TestErrNoTryWrapping(t *testing.T) {
	err := ErrNoTry
	if !errors.Is(err, ErrNoTry) {
		t.Error("errors.Is(ErrNoTry, ErrNoTry) should be true")
	}
	wrapped := fmt.Errorf("download failed after retries: %w", ErrNoTry)
	if !errors.Is(wrapped, ErrNoTry) {
		t.Error("errors.Is(wrapped, ErrNoTry) should be true")
	}
	if !IsNoTry(wrapped) {
		t.Error("IsNoTry(wrapped) should be true")
	}
}