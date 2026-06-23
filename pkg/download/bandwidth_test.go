// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/pkg/download"
)

func TestCheckHealthOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer ts.Close()

	err := download.CheckHealth(t.Context(), ts.URL+"/healthz", 5*time.Second)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestCheckHealthFail(t *testing.T) {
	err := download.CheckHealth(t.Context(), "http://localhost:1/healthz", time.Second)
	if err == nil {
		t.Error("expected error for unreachable server")
	}
}

func TestCheckHealthNon200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	err := download.CheckHealth(t.Context(), ts.URL+"/healthz", 5*time.Second)
	if err == nil {
		t.Error("expected error for 503 status")
	}
}

func TestCheckBandwidthBasic(t *testing.T) {
	data := make([]byte, 1024*1024) // 1MB of data
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}))
	defer ts.Close()

	bw, err := download.CheckBandwidth(t.Context(), ts.URL, 512*1024, 5*time.Second)
	if err != nil {
		t.Fatalf("CheckBandwidth should not error: %v", err)
	}
	if bw <= 0 {
		t.Errorf("expected positive bandwidth, got %f", bw)
	}
	t.Logf("Bandwidth: %.2f bytes/sec", bw)
}
