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

func TestCheckBandwidthFailure(t *testing.T) {
	t.Run("server error 500", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		_, err := download.CheckBandwidth(t.Context(), ts.URL, 512*1024, 5*time.Second)
		if err == nil {
			t.Error("expected error for 500 status, got nil")
		}
	})

	t.Run("unreachable server", func(t *testing.T) {
		_, err := download.CheckBandwidth(t.Context(), "http://127.0.0.1:1", 512*1024, time.Second)
		if err == nil {
			t.Error("expected error for unreachable server, got nil")
		}
	})
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
	if bw < 10*1024*1024 {
		t.Errorf("expected bandwidth > 10MB/s on loopback, got %f bytes/sec", bw)
	}
	t.Logf("Bandwidth: %.2f bytes/sec", bw)
}
