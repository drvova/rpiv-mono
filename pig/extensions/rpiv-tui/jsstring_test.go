package tui

import "testing"

func TestJSTrim(t *testing.T) {
	cases := []struct{ in, want, wantStart, wantEnd string }{
		{"", "", "", ""},
		{"  a b  ", "a b", "a b  ", "  a b"},
		{"\t\n\v\f\r x \r\n", "x", "x \r\n", "\t\n\v\f\r x"},
		{" nbsp ", "nbsp", "nbsp ", " nbsp"},
		{"\ufeffbom\ufeff", "bom", "bom\ufeff", "\ufeffbom"},
		{"　ideographic　", "ideographic", "ideographic　", "　ideographic"},
		{"   x     ", "x", "x     ", "   x"},
		{"​zwsp​", "​zwsp​", "​zwsp​", "​zwsp​"},
		{"   ", "", "", ""},
		{"\x1b[31m x \x1b[0m", "\x1b[31m x \x1b[0m", "\x1b[31m x \x1b[0m", "\x1b[31m x \x1b[0m"},
	}
	for _, tc := range cases {
		if got := JSTrim(tc.in); got != tc.want {
			t.Errorf("JSTrim(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if got := JSTrimStart(tc.in); got != tc.wantStart {
			t.Errorf("JSTrimStart(%q) = %q, want %q", tc.in, got, tc.wantStart)
		}
		if got := JSTrimEnd(tc.in); got != tc.wantEnd {
			t.Errorf("JSTrimEnd(%q) = %q, want %q", tc.in, got, tc.wantEnd)
		}
	}
}

func TestIsJSSpace(t *testing.T) {
	for r, want := range map[rune]bool{' ': true, '\t': true, 0x00a0: true, 0x2003: true, 0xfeff: true, 0x3000: true, 0x200b: false, 'a': false, 0x0085: false, 0x180e: false} {
		if got := IsJSSpace(r); got != want {
			t.Errorf("IsJSSpace(%U) = %v, want %v", r, got, want)
		}
	}
}

func TestLenUTF16(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"é", 1},
		{"界", 1},
		{"🙂", 2},
		{"a🙂b", 4},
		{"👩‍💻", 5},
		{"\xff", 1},
	}
	for _, tc := range cases {
		if got := LenUTF16(tc.in); got != tc.want {
			t.Errorf("LenUTF16(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestSliceUTF16(t *testing.T) {
	cases := []struct {
		in         string
		start, end int
		want       string
	}{
		{"hello", 0, 5, "hello"},
		{"hello", 1, 3, "el"},
		{"hello", 0, 100, "hello"},
		{"hello", 3, 2, ""},
		{"hello", 5, 5, ""},
		{"hello", -3, -1, "ll"},
		{"hello", -100, 2, "he"},
		{"hello", 2, -100, ""},
		{"a🙂b", 0, 3, "a🙂"},
		{"a🙂b", 1, 3, "🙂"},
		{"a🙂b", 3, 4, "b"},
		{"a🙂b", 0, 2, "a"},
		{"a🙂b", 2, 4, "b"},
		{"界面", 1, 2, "面"},
		{"", 0, 1, ""},
	}
	for _, tc := range cases {
		if got := SliceUTF16(tc.in, tc.start, tc.end); got != tc.want {
			t.Errorf("SliceUTF16(%q, %d, %d) = %q, want %q", tc.in, tc.start, tc.end, got, tc.want)
		}
	}
}

func TestPluralize(t *testing.T) {
	cases := []struct {
		count int
		want  string
	}{{0, "lanes"}, {1, "lane"}, {2, "lanes"}, {-1, "lanes"}}
	for _, tc := range cases {
		if got := Pluralize(tc.count, "lane", "lanes"); got != tc.want {
			t.Errorf("Pluralize(%d) = %q, want %q", tc.count, got, tc.want)
		}
		wantSuffix := "s"
		if tc.count == 1 {
			wantSuffix = ""
		}
		if got := PluralSuffix(tc.count); got != wantSuffix {
			t.Errorf("PluralSuffix(%d) = %q, want %q", tc.count, got, wantSuffix)
		}
	}
}

func TestRepeat(t *testing.T) {
	if got := Repeat("ab", 2); got != "abab" {
		t.Errorf("Repeat = %q", got)
	}
	if got := Repeat("ab", -1); got != "" {
		t.Errorf("Repeat negative = %q", got)
	}
}
