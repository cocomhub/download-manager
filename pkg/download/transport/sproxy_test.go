// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package transport_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/download/transport"
)

func TestSproxyTransportName(t *testing.T) {
	tr := transport.NewSproxyTunnelTransport("http://localhost:18083")
	if tr.Name() != "sproxy" {
		t.Errorf("expected 'sproxy', got %s", tr.Name())
	}
}

func TestSproxyTransportRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/example.com/file" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("response data"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tr := transport.NewSproxyTunnelTransport(srv.URL)
	resp, err := tr.RoundTrip(t.Context(), &download.TransportRequest{
		URL:    "http://example.com/file",
		Method: "GET",
	})
	if err != nil {
		t.Fatalf("RoundTrip should not error: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if string(data) != "response data" {
		t.Errorf("expected 'response data', got %q", string(data))
	}
}

func TestSproxyTransportWithTunnelKey(t *testing.T) {
	t.Skip("需要 mock tunnel server 来验证隧道行为")
	tr := transport.NewSproxyTunnelTransport("http://localhost:18083",
		transport.WithSproxyTunnelKey("0000000000000000000000000000000000000000000000000000000000000000"),
	)
	if tr.Name() != "sproxy" {
		t.Errorf("expected 'sproxy', got %s", tr.Name())
	}
}

func TestSproxyTransportHealthCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	tr := transport.NewSproxyTunnelTransport(srv.URL)
	err := tr.HealthCheck(t.Context())
	if err != nil {
		t.Fatalf("HealthCheck should not error: %v", err)
	}
}

func TestSproxyTransportRoundTripNoSproxy(t *testing.T) {
	// 验证当目标 URL 不安全时返回错误（isSafeTargetURL 会拒绝私有 IP）
	tr := transport.NewSproxyTunnelTransport("http://127.0.0.1:1")
	resp, err := tr.RoundTrip(t.Context(), &download.TransportRequest{
		URL:    "http://127.0.0.1/file",
		Method: "GET",
	})
	if err == nil {
		t.Error("expected error for blocked unsafe URL")
		if resp != nil {
			resp.Body.Close()
		}
	}
}
