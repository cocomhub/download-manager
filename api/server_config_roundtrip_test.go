// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cocomhub/download-manager/config"
)

// TestAPI_ServerConfig_LogAndDomainLimitsRoundTrip 验证 log 轮转与域名限流经
// POST /api/config/server 真正落到配置（此前只有 Playwright 覆盖，无 Go 层往返断言）。
func TestAPI_ServerConfig_LogAndDomainLimitsRoundTrip(t *testing.T) {
	mgr := newMgrWithMode(config.RunModeFull, true, true)
	srv := NewServer(mgr)
	r := srv.Router()

	rr := doJSONPost(t, r, "/api/config/server", map[string]any{
		"log": map[string]any{
			"filename": "./logs/rt.log", "max_size": 123, "max_backups": 4, "max_age": 5,
		},
		"downloader": map[string]any{
			"domain_limits": map[string]any{"rt.example.com": 3},
		},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/config/server = %d body=%s", rr.Code, rr.Body.String())
	}

	got := mgr.GetConfig()
	if got.Log.Filename != "./logs/rt.log" || got.Log.MaxSize != 123 ||
		got.Log.MaxBackups != 4 || got.Log.MaxAge != 5 {
		t.Fatalf("log 未落配置: %+v", got.Log)
	}
	if got.Downloader.DomainLimits["rt.example.com"] != 3 {
		t.Fatalf("domain_limits 未落配置: %v", got.Downloader.DomainLimits)
	}

	// GET 也应回读同样的嵌套结构（表单绑定依赖它）
	gr := doJSONGet(t, r, "/api/config/server")
	var body map[string]any
	if err := json.Unmarshal(gr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	logSec, _ := body["log"].(map[string]any)
	if logSec["filename"] != "./logs/rt.log" {
		t.Fatalf("GET log.filename 未回读: %v", logSec)
	}
}
