// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package logutil

import "testing"

func TestLogConfig_Defaults(t *testing.T) {
	lc := LogConfig{}
	if lc.MaxSize != 0 {
		t.Errorf("MaxSize = %d, want 0", lc.MaxSize)
	}
	if lc.Level != "" {
		t.Errorf("Level = %q, want empty", lc.Level)
	}
	if lc.Filename != "" {
		t.Errorf("Filename = %q, want empty", lc.Filename)
	}
	if lc.Console {
		t.Error("Console = true, want false")
	}
	if lc.Compress {
		t.Error("Compress = true, want false")
	}
}

func TestLogConfig_ValidValues(t *testing.T) {
	lc := LogConfig{
		Level:      "info",
		Filename:   "/tmp/test.log",
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     7,
		Console:    true,
		Compress:   true,
	}
	if lc.Level != "info" {
		t.Errorf("Level = %q, want info", lc.Level)
	}
	if lc.Filename != "/tmp/test.log" {
		t.Errorf("Filename = %q, want /tmp/test.log", lc.Filename)
	}
	if lc.MaxSize != 10 {
		t.Errorf("MaxSize = %d, want 10", lc.MaxSize)
	}
	if lc.MaxBackups != 3 {
		t.Errorf("MaxBackups = %d, want 3", lc.MaxBackups)
	}
	if lc.MaxAge != 7 {
		t.Errorf("MaxAge = %d, want 7", lc.MaxAge)
	}
	if !lc.Console {
		t.Errorf("Console = false, want true")
	}
	if !lc.Compress {
		t.Errorf("Compress = false, want true")
	}
}
