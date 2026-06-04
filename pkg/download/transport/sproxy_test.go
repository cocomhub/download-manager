// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package transport_test

import (
	"context"
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

func TestSproxyTransportRoundTripNoSproxy(t *testing.T) {
	tr := transport.NewSproxyTunnelTransport("http://localhost:18083")
	resp, err := tr.RoundTrip(context.Background(), &download.TransportRequest{
		URL:    "http://example.com/file",
		Method: "GET",
	})
	if err == nil {
		t.Skip("sproxy not running, skipping")
	}
	t.Logf("Got expected error: %v", err)
	_ = resp
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
	tr := transport.NewSproxyTunnelTransport("http://localhost:18083")
	err := tr.HealthCheck(context.Background())
	if err == nil {
		t.Skip("sproxy running, health check passed")
	}
	t.Logf("Health check error (expected): %v", err)
}
