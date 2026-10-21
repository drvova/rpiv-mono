package tui

import (
	"strings"
	"testing"
)

func TestTruncateToWidthExactResults(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		width    int
		ellipsis string
		pad      bool
		want     string
	}{
		{"short fits, no pad", "hi", 10, "...", false, "hi"},
		{"short fits, pad", "hi", 5, "...", true, "hi   "},
		{"exactly fits", "hello", 5, "...", false, "hello"},
		{"empty no pad", "", 10, "...", false, ""},
		{"empty pad", "", 5, "...", true, "     "},
		{"zero width", "hello", 0, "...", false, ""},
		{"negative width", "hello", -3, "...", true, ""},
		{"ascii cut gets reset-bracketed ellipsis", "hello world", 8, "...", false, "hello\x1b[0m...\x1b[0m"},
		{"ascii cut padded", "hello world", 10, "...", true, "hello w\x1b[0m...\x1b[0m"},
		{"cut without ellipsis keeps a trailing reset", "hello world", 5, "", false, "hello\x1b[0m"},
		{"styled text keeps its color and resets before the ellipsis", "\x1b[31mhello world\x1b[0m", 8, "...", false, "\x1b[31mhello\x1b[0m...\x1b[0m"},
		{"style right before the cut is dropped", "hello\x1b[31m world\x1b[0m", 8, "...", false, "hello\x1b[0m...\x1b[0m"},
		{"wide chars stop before a partial cell", "你好世界", 6, "...", false, "你\x1b[0m...\x1b[0m"},
		{"wide chars fit exactly", "你好", 4, "...", false, "你好"},
		{"lone skin tone modifier is two columns", "\U0001F3FDx\U0001F3FDx", 2, "", false, "\U0001F3FD\x1b[0m"},
		{"skin tone modifier after a letter adds nothing", "\U0001F3FDx\U0001F3FDx", 3, "", false, "\U0001F3FDx\U0001F3FD\x1b[0m"},
		{"regional indicator with a spacing mark is two columns", "🇺🇸\u093F日本", 4, "", false, "🇺🇸\u093F日\x1b[0m"},
		{"modifier sequence on a text-presentation base is two columns", "☝🏻x", 2, "", false, "☝🏻\x1b[0m"},
		{"modifier sequence padded to its real width", "☝🏻", 4, "", true, "☝🏻  "},
		{"ZWJ sequence with VS16 inside fits in two columns", "❤️‍🔥", 2, "...", false, "❤️‍🔥"},
		{"ZWJ sequence with VS16 inside is kept whole", "❤️‍🔥x", 2, "", false, "❤️‍🔥\x1b[0m"},
		{"ZWJ sequence with VS16 inside is cut whole", "❤️‍🔥", 1, "", false, "\x1b[0m"},
		{"unicode 16 emoji is two columns", "\U0001FAE9x", 2, "", false, "\U0001FAE9\x1b[0m"},
		{"tab counts three", "a\tb", 4, "...", false, "a\x1b[0m...\x1b[0m"},
		{"tab kept when it fits", "a\tbcdef", 8, "...", false, "a\tb\x1b[0m...\x1b[0m"},
		{"BEL hyperlink closed when its label is cut", "\x1b]8;;https://example.com\x07some-longer-label-here\x1b]8;;\x07", 15, "...", false, "\x1b]8;;https://example.com\x07some-longer-\x1b]8;;\x07\x1b[0m...\x1b[0m"},
		{"ST hyperlink closed with ST", "\x1b]8;;https://example.com\x1b\\some-longer-label-here\x1b]8;;\x1b\\", 15, "...", false, "\x1b]8;;https://example.com\x1b\\some-longer-\x1b]8;;\x1b\\\x1b[0m...\x1b[0m"},
		{"malformed BEL OSC 8 is not a link", "\x1b]8;\x07abcdef", 4, "", false, "\x1b]8;\x07abcd\x1b[0m"},
		{"malformed ST OSC 8 is not a link", "\x1b]8;\x1b\\abcdef", 4, "", false, "\x1b]8;\x1b\\abcd\x1b[0m"},
		{"wide ellipsis too wide for width 1", "abcdef", 1, "🙂", false, ""},
		{"wide ellipsis exactly fills width 2", "abcdef", 2, "🙂", false, "\x1b[0m🙂\x1b[0m"},
		{"fits even though ellipsis is too wide", "a", 2, "🙂", false, "a"},
		{"wide char fits even though ellipsis is too wide", "界", 2, "🙂", false, "界"},
		{"contiguous prefix with wide grapheme and tab", "🙂\t界 \x1b_abc\x07", 7, "…", true, "🙂\t\x1b[0m…\x1b[0m "},
		{"pad short text", "hi", 5, "...", true, "hi   "},
		{"unicode ellipsis", "hello world", 6, "…", false, "hello\x1b[0m…\x1b[0m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TruncateToWidth(tc.text, tc.width, tc.ellipsis, tc.pad)
			if got != tc.want {
				t.Errorf("TruncateToWidth(%q, %d, %q, %v)\n got  %q\n want %q", tc.text, tc.width, tc.ellipsis, tc.pad, got, tc.want)
			}
		})
	}
}

func TestTruncateToWidthStaysWithinWidth(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		width    int
		ellipsis string
		pad      bool
	}{
		{"very large unicode input", strings.Repeat("🙂界", 100_000), 40, "…", false},
		{"styled repeated text", "\x1b[31m" + strings.Repeat("hello ", 1000) + "\x1b[0m", 20, "…", false},
		{"malformed escape prefix", "abc\x1bnot-ansi " + strings.Repeat("🙂", 1000), 20, "…", false},
		{"no ellipsis", "\x1b[31m" + strings.Repeat("hello", 100), 10, "", false},
		{"unicode script line", `✔ script to run › dev $ concurrently "vite" "node --import tsx ./server.ts"`, 65, "...", false},
		{"emoji line", "🎉 Celebration! 🚀 Launch 📦 Package ready for deployment now", 38, "...", false},
		{"mixed wide and ascii", "Hello 世界 Test 你好 More text here that is long", 28, "...", false},
		{"padded wide", "🙂界🙂界🙂界", 8, "…", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TruncateToWidth(tc.text, tc.width, tc.ellipsis, tc.pad)
			if w := VisibleWidth(got); w > tc.width || (tc.pad && w != tc.width) {
				t.Errorf("visible width %d of %q, want <= %d (pad=%v)", w, got, tc.width, tc.pad)
			}
			if tc.ellipsis != "" && !strings.HasSuffix(got, tc.ellipsis+"\x1b[0m") && !tc.pad {
				t.Errorf("got %q, want ellipsis %q bracketed by resets", got, tc.ellipsis)
			}
			if tc.ellipsis == "" && !strings.HasSuffix(got, "\x1b[0m") {
				t.Errorf("got %q, want trailing reset", got)
			}
		})
	}
}

func TestTruncateToWidthKeepsStylingOfKeptText(t *testing.T) {
	got := TruncateToWidth("\x1b[31m"+strings.Repeat("hello ", 1000)+"\x1b[0m", 20, "…", false)
	if !strings.HasPrefix(got, "\x1b[31m") || !strings.HasSuffix(got, "\x1b[0m…\x1b[0m") {
		t.Fatalf("got %q", got)
	}
}
