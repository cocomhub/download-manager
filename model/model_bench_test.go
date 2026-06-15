// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

import "testing"

func BenchmarkStatusTransition(b *testing.B) {
	obj := &DownloadObject{
		URL:    "http://example.com/file.zip",
		Status: StatusPending,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		obj.SetStatus(StatusDownloading)
		obj.SetProgress(50)
		obj.SetStatus(StatusCompleted)
		obj.SetProgress(100)
	}
}
