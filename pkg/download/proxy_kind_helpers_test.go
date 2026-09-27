// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import "testing"

// TestRedactProxyURL_Prefix 验证脱敏兼容类型前缀：gateway:http://u:p@h → gateway:http://h。
func TestRedactProxyURL_Prefix(t *testing.T) {
	t.Parallel()
	got := redactProxyURL("gateway:http://user:pass@127.0.0.1:1080")
	want := "gateway:http://127.0.0.1:1080"
	if got != want {
		t.Errorf("redact = %q, want %q", got, want)
	}
	// 无前缀标准代理正常脱敏。
	got2 := redactProxyURL("http://user:pass@127.0.0.1:1080")
	want2 := "http://127.0.0.1:1080"
	if got2 != want2 {
		t.Errorf("redact standard = %q, want %q", got2, want2)
	}
}

// TestGetProxyBandwidth_Prefix 验证带宽探测兼容类型前缀（剥前缀后探测真实 URL）。
func TestGetProxyBandwidth_Prefix(t *testing.T) {
	t.Parallel()
	// 探测目标 = 代理 URL + /bandwidth。gateway: 前缀应被剥离。
	srv := mockBandwidthServer(t, "42.5")
	got := getProxyBandwidth(t.Context(), "gateway:"+srv.URL, "/bandwidth", 2)
	if got != 42.5 {
		t.Errorf("bandwidth = %v, want 42.5", got)
	}
}
