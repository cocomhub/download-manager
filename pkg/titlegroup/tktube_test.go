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

func TestTKTContentGroupKey(t *testing.T) {
	cases := []struct {
		name  string
		title string
		url   string
		want  string
	}{
		{name: "合法组名", title: "【高画质】CLUB-100C", url: "https://example.com/a", want: "CLUB-100"},
		{name: "未知标题", title: "  随机标题  ", url: "https://example.com/b", want: "unknown+随机标题"},
		{name: "空标题", title: "   ", url: "https://example.com/c", want: "unknown+https://example.com/c"},
		{name: "全空仍非空", title: "", url: "", want: "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TKTContentGroupKey(tc.title, tc.url)
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
			if got == "" {
				t.Fatal("content group key should never be empty")
			}
		})
	}
}
