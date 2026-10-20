package tui

import "testing"

func TestExtractAnsi(t *testing.T) {
	cases := []struct {
		name string
		in   string
		at   int
		want int
	}{
		{"empty", "", 0, 0},
		{"non-escape", "hello", 0, 0},
		{"CSI SGR reset", "\x1b[0m", 0, 4},
		{"CSI SGR bold red", "\x1b[1;31m", 0, 7},
		{"CSI cursor column", "\x1b[5G", 0, 4},
		{"CSI erase line", "\x1b[K", 0, 3},
		{"CSI cursor home", "\x1b[H", 0, 3},
		{"CSI erase screen", "\x1b[2J", 0, 4},
		{"CSI cursor up is not an escape", "\x1b[3A", 0, 0},
		{"DEC private mode is not an escape", "\x1b[?2026h", 0, 0},
		{"OSC 8 open", "\x1b]8;;https://e.com\x07", 0, 19},
		{"OSC 8 close", "\x1b]8;;\x07", 0, 6},
		{"OSC with ST", "\x1b]0;title\x1b\\", 0, 11},
		{"APC with BEL", "\x1b_pi:c\x07", 0, 7},
		{"APC with ST", "\x1b_abc\x1b\\", 0, 7},
		{"escape mid-string", "x\x1b[31my", 1, 5},
		{"offset not at an escape", "x\x1b[31my", 0, 0},
		{"unterminated CSI", "\x1b[", 0, 0},
		{"unterminated OSC", "\x1b]abc", 0, 0},
		{"lone ESC at end", "a\x1b", 1, 0},
		{"negative offset", "\x1b[0m", -1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExtractAnsi(tc.in, tc.at); got != tc.want {
				t.Errorf("ExtractAnsi(%q, %d) = %d, want %d", tc.in, tc.at, got, tc.want)
			}
		})
	}
}

func TestExtractAnsiCodeReturnsTheSequence(t *testing.T) {
	code, n := ExtractAnsiCode("ab\x1b[1;31mcd", 2)
	if code != "\x1b[1;31m" || n != 7 {
		t.Fatalf("ExtractAnsiCode = (%q, %d)", code, n)
	}
}

func TestStripAnsi(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"hello", "hello"},
		{"\x1b[1;31mhello\x1b[0m", "hello"},
		{"\x1b]8;;https://e.com\x07link\x1b]8;;\x07", "link"},
		{"\x1b_pi:c\x07prompt", "prompt"},
		{"line\nwith\nnewlines", "line\nwith\nnewlines"},
		{"\x1b[31mred\x1b[0m \x1b[32mgreen\x1b[0m", "red green"},
		{"\x1b[3Aup", "\x1b[3Aup"},
		{"\x1b[31", "\x1b[31"},
	}
	for _, tc := range cases {
		if got := StripAnsi(tc.in); got != tc.want {
			t.Errorf("StripAnsi(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
