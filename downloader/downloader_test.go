// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package downloader

import (
	"context"
	"testing"

	"github.com/cocomhub/download-manager/config"
)

func TestNew_Native(t *testing.T) {
	cfg := config.Downloader{Type: "native"}
	d := New(cfg)
	if d == nil {
		t.Fatal("New() with type=native returned nil")
	}
	if got := d.Name(); got == "" {
		t.Error("Name() returned empty")
	}
}

func TestNew_Default(t *testing.T) {
	d := New(config.Downloader{})
	if d == nil {
		t.Fatal("New() with empty type returned nil")
	}
	if got := d.Name(); got == "" {
		t.Error("Name() returned empty")
	}
}

func TestNew_UnknownType(t *testing.T) {
	cfg := config.Downloader{Type: "nonexistent"}
	d := New(cfg)
	if d == nil {
		t.Fatal("New() with unknown type returned nil")
	}
}

func TestNew_NativeOld(t *testing.T) {
	cfg := config.Downloader{Type: "native_old"}
	d := New(cfg)
	if d == nil {
		t.Fatal("New() with type=native_old returned nil")
	}
	if got := d.Name(); got == "" {
		t.Error("Name() returned empty for native_old")
	}
}

func TestNew_WithContext(t *testing.T) {
	d := New(config.Downloader{Type: "native"})
	if wc, ok := d.(interface{ SetContext(context.Context) }); ok {
		wc.SetContext(t.Context())
		// 涓?panic 鍗冲彲
	} else {
		t.Log("Downloader does not implement SetContext, skipping")
	}
}

func TestNew_NameValues(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Downloader
	}{
		{"native", config.Downloader{Type: "native"}},
		{"native_old", config.Downloader{Type: "native_old"}},
		{"wget", config.Downloader{Type: "wget"}},
		{"empty_default", config.Downloader{Type: ""}},
	}
	for _, tt := range tests {
		d := New(tt.cfg)
		if d == nil {
			t.Errorf("New(%q) returned nil", tt.name)
			continue
		}
		if got := d.Name(); got == "" {
			t.Errorf("New(%q).Name() returned empty", tt.name)
		}
	}
}

func TestAdapter_ExtendedInterfaces(t *testing.T) {
	d := New(config.Downloader{Type: "native"})
	// 妫€鏌?Cancelable 鎺ュ彛
	if canceler, ok := d.(interface{ Cancel(string) error }); ok {
		err := canceler.Cancel("http://test.url/file")
		if err != nil {
			t.Logf("Cancel on idle adapter returned: %v (acceptable)", err)
		}
	}
	// 妫€鏌?Name 涓€鑷存€?	if d.Name() == "" {
		t.Error("Name() should not be empty")
	}
}

func TestAdapter_DomainLimits(t *testing.T) {
	d := New(config.Downloader{Type: "native"})
	if dl, ok := d.(interface{ ApplyDomainLimits(map[string]int) }); ok {
		dl.ApplyDomainLimits(map[string]int{"example.com": 3})
		// 涓?panic 鍗冲彲
	} else {
		t.Log("Downloader does not implement ApplyDomainLimits, skipping")
	}
}
