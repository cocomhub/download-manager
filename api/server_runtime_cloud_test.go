// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cocomhub/download-manager/config"
)

// runtimeFeatures 读取 GET /api/runtime 的 features 映射。
func runtimeFeatures(t *testing.T, r http.Handler) map[string]any {
	t.Helper()
	rr := doJSONGet(t, r, "/api/runtime")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/runtime = %d", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal runtime: %v", err)
	}
	f, _ := body["features"].(map[string]any)
	return f
}

// TestAPI_Runtime_CloudDownloadFeatureBit 验证能力位真的下发（UI 置灰依赖它）：
// 未配置 sproxy_cloud 时为 false，配置后为 true。
func TestAPI_Runtime_CloudDownloadFeatureBit(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-rt-cloud", 1, true)
	if got, ok := runtimeFeatures(t, srv.Router())["cloud_download"]; !ok || got != false {
		t.Fatalf("未配置时应下发 cloud_download=false, got %v (present=%v)", got, ok)
	}

	cfg := &config.Config{}
	cfg.ValidateAndClamp()
	cfg.Downloader.SproxyCloud.APIURL = "http://127.0.0.1:8080/api/cloud/download"
	srv2 := NewServer(newTestManager(cfg))
	if got, ok := runtimeFeatures(t, srv2.Router())["cloud_download"]; !ok || got != true {
		t.Fatalf("已配置时应下发 cloud_download=true, got %v (present=%v)", got, ok)
	}
}
