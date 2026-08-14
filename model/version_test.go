// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"encoding/json"
	"testing"
)

func TestVersionGetSet(t *testing.T) {
	obj := &DownloadObject{}
	if obj.GetVersion() != 0 {
		t.Errorf("default version = %d, want 0", obj.GetVersion())
	}
	obj.SetVersion(3)
	if obj.GetVersion() != 3 {
		t.Errorf("version = %d, want 3", obj.GetVersion())
	}
	if obj.Version != 3 {
		t.Errorf("field version = %d, want 3", obj.Version)
	}
}

func TestVersionJSONRoundTrip(t *testing.T) {
	obj := &DownloadObject{URL: "u1", Version: 2}
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back DownloadObject
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Version != 2 {
		t.Errorf("round-trip version = %d, want 2", back.Version)
	}
}
