// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package transport_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/download/transport"
	"github.com/cocomhub/sproxy/pkg/tunnel"
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
	t.Run("round trip via tunnel", func(t *testing.T) {
		tunnelKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		keyBytes, err := tunnel.ParseKey(tunnelKey)
		if err != nil {
			t.Fatalf("ParseKey: %v", err)
		}

		tunnelHandler := tunnel.NewHandler(keyBytes, slog.Default())
		tunnelSrv := httptest.NewServer(tunnelHandler)
		defer tunnelSrv.Close()

		tr := transport.NewSproxyTunnelTransport(tunnelSrv.URL,
			transport.WithSproxyTunnelKey(tunnelKey),
		)

		resp, err := tr.RoundTrip(t.Context(), &download.TransportRequest{
			URL:    "http://example.com/file",
			Method: "GET",
		})
		if err != nil {
			t.Fatalf("RoundTrip via tunnel should not error: %v", err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		if len(data) == 0 {
			t.Error("expected non-empty response body from tunnel")
		}
	})

	t.Run("invalid key falls back to proxy", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/example.com/file" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("proxy fallback"))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		tr := transport.NewSproxyTunnelTransport(srv.URL,
			transport.WithSproxyTunnelKey("invalid-key"),
		)

		resp, err := tr.RoundTrip(t.Context(), &download.TransportRequest{
			URL:    "http://example.com/file",
			Method: "GET",
		})
		if err != nil {
			t.Fatalf("RoundTrip via proxy should not error: %v", err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		if string(data) != "proxy fallback" {
			t.Errorf("expected 'proxy fallback', got %q", string(data))
		}
	})
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
