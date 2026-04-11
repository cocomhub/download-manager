package titlegroup

import "testing"

func TestTKTVariantFlags(t *testing.T) {
	cases := []struct {
		in           string
		hasHQ, hasC  bool
	}{
		{"【高画质】CLUB-100", true, false},
		{"CLUB-100C", false, true},
		{"【高画质】CLUB-100C", true, true},
		{"ABP-456", false, false},
		{"随机标题", false, false},
	}
	for _, c := range cases {
		hq, cflag := TKTVariantFlags(c.in)
		if hq != c.hasHQ || cflag != c.hasC {
			t.Fatalf("input=%q: want (%v,%v), got (%v,%v)", c.in, c.hasHQ, c.hasC, hq, cflag)
		}
	}
}

