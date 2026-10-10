// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/cocomhub/download-manager/testutil/assert"
)

const cloudDLObjectURL = "http://mock-download/file-0.bin"

// readObjectCloudDownload 读取任务下指定 URL 的 cloud_download 标志。
// 返回 (found, enabled)：found=false 表示任务/对象尚未就绪（供轮询重试），
// 避免「服务端出错 → 返回 false」使「关闭」断言假绿。
func readObjectCloudDownload(t *testing.T, r http.Handler, taskID, objURL string) (bool, bool) {
	t.Helper()
	rr := doJSONGet(t, r, "/api/tasks/"+taskID)
	if rr.Code != http.StatusOK {
		return false, false
	}
	var result map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		return false, false
	}
	objects, _ := result["objects"].([]any)
	for _, raw := range objects {
		obj, _ := raw.(map[string]any)
		if obj["url"] == objURL {
			v, _ := obj["cloud_download"].(bool)
			return true, v
		}
	}
	return false, false
}

// TestAPI_SetObjectCloudDownload 验证下载项「云端下载」选项开关与回读（POST /api/tasks/{id}/object/cloud_download）。
func TestAPI_SetObjectCloudDownload(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-cloud-dl", 2, true)
	r := srv.Router()
	startAPIManager(t, srv)

	const taskID = "mock-cloud-dl"
	// 必须轮询到「目标对象已就绪」：任务端点 200 不代表对象已 seed 完成
	// （AGENTS：测试不假设 seed 在 startAPIManager 返回时完成）。
	assert.MustEventually(t, func() bool {
		found, _ := readObjectCloudDownload(t, r, taskID, cloudDLObjectURL)
		return found
	}, 5*time.Second, 50*time.Millisecond, "wait for target object to be seeded")

	post := func(enabled bool) {
		t.Helper()
		rr := doJSONPost(t, r, "/api/tasks/"+taskID+"/object/cloud_download",
			map[string]any{"url": cloudDLObjectURL, "enabled": enabled})
		if rr.Code != http.StatusOK {
			t.Fatalf("cloud_download=%v: code=%d body=%s", enabled, rr.Code, rr.Body.String())
		}
	}

	post(true)
	assert.MustEventually(t, func() bool {
		found, enabled := readObjectCloudDownload(t, r, taskID, cloudDLObjectURL)
		return found && enabled
	}, 3*time.Second, 50*time.Millisecond, "cloud_download should become true")

	post(false)
	assert.MustEventually(t, func() bool {
		found, enabled := readObjectCloudDownload(t, r, taskID, cloudDLObjectURL)
		return found && !enabled
	}, 3*time.Second, 50*time.Millisecond, "cloud_download should become false")
}

// TestAPI_SetObjectCloudDownload_BadRequest 验证缺少 url 时返回 400。
func TestAPI_SetObjectCloudDownload_BadRequest(t *testing.T) {
	srv, _ := newAPIServerWithMock(t, "mock-cloud-dl-bad", 1, true)
	r := srv.Router()
	startAPIManager(t, srv)

	rr := doJSONPost(t, r, "/api/tasks/mock-cloud-dl-bad/object/cloud_download", map[string]any{"enabled": true})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("missing url should be 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}
