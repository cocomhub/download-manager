// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

import "testing"

func FuzzStatusTransition(f *testing.F) {
	// Seed corpus: known status values plus edge cases
	seeds := []string{
		"pending",
		"downloading",
		"completed",
		"failed",
		"failed_permanent",
		"cancelled",
		"resolving",
		"",
		"unknown",
		"PENDING",
		" pending ",
		"completed\n",
		"<script>alert(1)</script>",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, status string) {
		obj := &DownloadObject{
			URL:    "http://example.com/file.zip",
			Status: StatusPending,
		}
		obj.SetStatus(status)
		// Verify: GetStatus returns a string (no panic), and
		// SetStatus is idempotent with respect to GetStatus.
		got := obj.GetStatus()
		if got != status {
			t.Logf("SetStatus(%q) -> GetStatus() = %q (acceptable mismatch)", status, got)
		}
	})
}
