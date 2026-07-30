// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package transport_test

import (
	"os"
	"testing"

	"github.com/cocomhub/download-manager/pkg/download"
	"github.com/cocomhub/download-manager/pkg/download/transport"
)

// getTestSproxyURL 返回测试用的 sproxy 地址。
// 优先使用 TEST_SPROXY_URL 环境变量，否则使用默认值。
func getTestSproxyURL() string {
	if v := os.Getenv("TEST_SPROXY_URL"); v != "" {
		return v
	}
	return "http://localhost:18083"
}

func TestSproxyTransportName(t *testing.T) {
	tr := transport.NewSproxyTunnelTransport(getTestSproxyURL())
	if tr.Name() != "sproxy" {
		t.Errorf("expected 'sproxy', got %s", tr.Name())
	}
}

func TestSproxyTransportRoundTripNoSproxy(t *testing.T) {
	tr := transport.NewSproxyTunnelTransport(getTestSproxyURL())
	resp, err := tr.RoundTrip(t.Context(), &download.TransportRequest{
		URL:    "http://example.com/file",
		Method: "GET",
	})
	if err != nil {
		t.Logf("Got expected error: %v", err)
		return
	}
	// 如果 sproxy 正在运行，验证响应
	if resp != nil {
		t.Logf("Got response: %+v", resp)
	}
}

func TestSproxyTransportWithTunnelKey(t *testing.T) {
	tr := transport.NewSproxyTunnelTransport("http://localhost:18083",
		transport.WithSproxyTunnelKey("0000000000000000000000000000000000000000000000000000000000000000"),
	)
	if tr.Name() != "sproxy" {
		t.Errorf("expected 'sproxy', got %s", tr.Name())
	}
}

func TestSproxyTransportHealthCheck(t *testing.T) {
	tr := transport.NewSproxyTunnelTransport(getTestSproxyURL())
	err := tr.HealthCheck(t.Context())
	if err != nil {
		t.Logf("Health check error (expected without sproxy): %v", err)
		return
	}
	t.Log("sproxy running, health check passed")
}
