// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cocomhub/download-manager/config"
)

func TestSproxyConfigView_RedactsSecrets(t *testing.T) {
	t.Parallel()
	view := sproxyConfigView(config.Sproxy{
		APIURL: "https://sproxy.example.com", APIToken: "secret-token",
		AccessKey: "ak", AccessKeySecret: "secret-sk", AccessKeyID: "skey-1",
		PlayBaseURL: "https://play.example.com",
	})
	b, _ := json.Marshal(view)
	if strings.Contains(string(b), "secret-sk") || strings.Contains(string(b), "secret-token") {
		t.Fatalf("脱敏视图泄露 secret: %s", b)
	}
	if view["has_access_key_secret"] != true || view["has_api_token"] != true {
		t.Fatalf("has_* 标记缺失: %+v", view)
	}
}
