// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestValidateAndClamp_SproxyLegacyFallback(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Downloader: Downloader{SproxyCloud: SproxyCloudConfig{
			APIURL: "https://legacy.example.com", APIToken: "tok",
			AccessKey: "ak", AccessKeySecret: "sk", AccessKeyID: "skey-1",
			InsecureTLS: true,
		}},
	}
	cfg.ValidateAndClamp()
	if cfg.Sproxy.APIURL != "https://legacy.example.com" || cfg.Sproxy.AccessKey != "ak" {
		t.Fatalf("legacy 未回填: %+v", cfg.Sproxy)
	}
	if cfg.Sproxy.PlayTTL != time.Hour {
		t.Fatalf("play_ttl 默认应为 1h: %v", cfg.Sproxy.PlayTTL)
	}
	if cfg.RemoteSave.Concurrency != 2 || cfg.RemoteSave.ReconcileInterval != 10*time.Minute {
		t.Fatalf("remote_save 默认值: %+v", cfg.RemoteSave)
	}
	if cfg.RemoteSave.VerifyChecksum {
		t.Fatal("verify_checksum 必须默认 false")
	}
}

func TestValidateAndClamp_SproxyWinsLegacy(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Sproxy: Sproxy{APIURL: "https://new.example.com", AccessKey: "new-ak"},
		Downloader: Downloader{SproxyCloud: SproxyCloudConfig{
			APIURL: "https://legacy.example.com", AccessKey: "old-ak",
		}},
	}
	cfg.ValidateAndClamp()
	if cfg.Sproxy.APIURL != "https://new.example.com" || cfg.Sproxy.AccessKey != "new-ak" {
		t.Fatalf("新 sproxy 应优先: %+v", cfg.Sproxy)
	}
	if cfg.Downloader.SproxyCloud.APIURL != "https://new.example.com" {
		t.Fatalf("旧字段应同步为新值: %+v", cfg.Downloader.SproxyCloud)
	}
}

func TestValidateAndClamp_RemoteSaveClamp(t *testing.T) {
	t.Parallel()
	cfg := &Config{RemoteSave: RemoteSaveSettings{
		Concurrency: 99, ReconcileInterval: -1, DeepReconcileInterval: -1,
		SettleSeconds: -1, MaxRetries: -1, MaxFilesPerObject: -1,
	}}
	cfg.ValidateAndClamp()
	if cfg.RemoteSave.Concurrency != 8 {
		t.Fatalf("concurrency 应钳到 8: %d", cfg.RemoteSave.Concurrency)
	}
	if cfg.RemoteSave.ReconcileInterval != 10*time.Minute ||
		cfg.RemoteSave.DeepReconcileInterval != 24*time.Hour ||
		cfg.RemoteSave.SettleSeconds != 30 ||
		cfg.RemoteSave.MaxRetries != 3 ||
		cfg.RemoteSave.MaxFilesPerObject != 1000 {
		t.Fatalf("默认值: %+v", cfg.RemoteSave)
	}
}

func TestDiff_SproxySecretRedacted(t *testing.T) {
	t.Parallel()
	a := Config{Sproxy: Sproxy{APIURL: "a", AccessKeySecret: "sk-a"}}
	b := Config{Sproxy: Sproxy{APIURL: "b", AccessKeySecret: "sk-b"}}
	changes := a.Diff(b)
	var found bool
	for _, c := range changes {
		if c.Path != "sproxy" {
			continue
		}
		found = true
		raw, _ := json.Marshal(c)
		if strings.Contains(string(raw), "sk-a") || strings.Contains(string(raw), "sk-b") {
			t.Fatalf("diff 泄露 secret: %s", raw)
		}
	}
	if !found {
		t.Fatal("missing sproxy diff")
	}
}
