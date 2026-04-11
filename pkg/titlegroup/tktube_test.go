package titlegroup

import "testing"

func TestTKTGroupNameFromTitle(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"CLUB-100", "CLUB-100"},
		{"CLUB-100C", "CLUB-100"},
		{"【高画质】CLUB-100", "CLUB-100"},
		{"【高画质】CLUB-100C", "CLUB-100"},
		{"SSIS-123", "SSIS-123"},
		{"ABP-456C", "ABP-456"},
		{"随机标题", ""},
	}
	for _, c := range cases {
		got := TKTGroupNameFromTitle(c.in)
		if got != c.want {
			t.Fatalf("input=%q: want %q, got %q", c.in, c.want, got)
		}
	}
}

