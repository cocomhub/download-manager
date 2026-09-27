// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package download

import "testing"

func TestParseProxyKind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		kind ProxyKind
		url  string
	}{
		{"无前缀默认 standard", "http://127.0.0.1:1080", ProxyKindStandard, "http://127.0.0.1:1080"},
		{"gateway 前缀", "gateway:http://127.0.0.1:1080", ProxyKindGateway, "http://127.0.0.1:1080"},
		{"空串", "", ProxyKindStandard, ""},
		{"未知前缀按 standard 不误伤", "weird:http://x", ProxyKindStandard, "weird:http://x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			kind, u := ParseProxyKind(tc.in)
			if kind != tc.kind {
				t.Errorf("kind = %v, want %v", kind, tc.kind)
			}
			if u != tc.url {
				t.Errorf("url = %q, want %q", u, tc.url)
			}
		})
	}
}

func TestProxyKindString(t *testing.T) {
	t.Parallel()
	if ProxyKindStandard.String() != "standard" {
		t.Errorf("standard.String() = %q", ProxyKindStandard.String())
	}
	if ProxyKindGateway.String() != "gateway" {
		t.Errorf("gateway.String() = %q", ProxyKindGateway.String())
	}
}
