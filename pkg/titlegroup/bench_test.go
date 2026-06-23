// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package titlegroup

import "testing"

func BenchmarkTKTVariantFlags(b *testing.B) {
	titles := []string{
		"My Video (1080p)",
		"My Video (720p)",
		"My Video (480p)",
		"My Video (1080p, CC)",
		"Another Video (4K)",
	}
	b.ResetTimer()
	for b.Loop() {
		for _, title := range titles {
			TKTVariantFlags(title)
		}
	}
}
