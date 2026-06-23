// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package titlegroup

import "testing"

func TestTKTVariantFlags(t *testing.T) {
	cases := []struct {
		in          string
		hasHQ, hasC bool
	}{
		{"銆愰珮鐢昏川銆慍LUB-100", true, false},
		{"CLUB-100C", false, true},
		{"銆愰珮鐢昏川銆慍LUB-100C", true, true},
		{"ABP-456", false, false},
		{"闅忔満鏍囬", false, false},
	}
	for _, c := range cases {
		hq, cflag := TKTVariantFlags(c.in)
		if hq != c.hasHQ || cflag != c.hasC {
			t.Fatalf("input=%q: want (%v,%v), got (%v,%v)", c.in, c.hasHQ, c.hasC, hq, cflag)
		}
	}
}
