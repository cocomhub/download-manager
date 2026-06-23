// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
)

// responseCheckHelper creates an httptest.Server returning given status + content-type + body.
func responseCheckHelper(t *testing.T, status int, ct, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", ct)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

// TestHTTPExtractorResponseCheck verifies that response check functions are invoked
// and can reject downloads based on response inspection.
func TestHTTPExtractorResponseCheck(t *testing.T) {
	ts := responseCheckHelper(t, 200, "application/octet-stream", "some data")
	defer ts.Close()

	dir := t.TempDir()
	dest := dir + "/check.bin"

	ext := newHTTPExtractor(t)
	called := false
	ext.AddResponseCheck(func(req *download.Request, tresp *download.TransportResponse) error {
		called = true
		return nil
	})

	err := ext.Extract(t.Context(), &download.Request{
		URL:      ts.URL,
		SavePath: dest,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !called {
		t.Error("response check was not called")
	}
}

// TestHTTPExtractorResponseCheckReject verifies a check returning ErrNoTry stops the download.
func TestHTTPExtractorResponseCheckReject(t *testing.T) {
	ts := responseCheckHelper(t, 200, "application/octet-stream", "data")
	defer ts.Close()

	dir := t.TempDir()
	dest := dir + "/reject.bin"

	ext := newHTTPExtractor(t)
	ext.AddResponseCheck(func(req *download.Request, tresp *download.TransportResponse) error {
		return download.ErrNoTry
	})

	err := ext.Extract(t.Context(), &download.Request{
		URL:      ts.URL,
		SavePath: dest,
	})
	if !download.IsNoTry(err) {
		t.Fatalf("expected ErrNoTry, got: %v", err)
	}
}

// TestHTTPExtractorResponseCheckMultiple verifies multiple checks run in order
// and the first rejection short-circuits remaining checks.
func TestHTTPExtractorResponseCheckMultiple(t *testing.T) {
	ts := responseCheckHelper(t, 200, "application/octet-stream", "data")
	defer ts.Close()

	dir := t.TempDir()
	dest := dir + "/multi.bin"

	ext := newHTTPExtractor(t)
	order := []int{}
	ext.AddResponseCheck(func(req *download.Request, tresp *download.TransportResponse) error {
		order = append(order, 1)
		return nil
	})
	ext.AddResponseCheck(func(req *download.Request, tresp *download.TransportResponse) error {
		order = append(order, 2)
		return download.ErrNoTry
	})
	ext.AddResponseCheck(func(req *download.Request, tresp *download.TransportResponse) error {
		order = append(order, 3) // should not be called
		return nil
	})

	err := ext.Extract(t.Context(), &download.Request{
		URL:      ts.URL,
		SavePath: dest,
	})
	if !download.IsNoTry(err) {
		t.Fatalf("expected ErrNoTry, got: %v", err)
	}
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Errorf("expected checks 1,2 to run, got order: %v", order)
	}
}
