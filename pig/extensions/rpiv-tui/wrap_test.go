package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestWrapTextWithAnsiExactLines(t *testing.T) {
	const reset = "\x1b[0m"
	cases := []struct {
		name  string
		in    string
		width int
		want  []string
	}{
		{"empty", "", 10, []string{""}},
		{"single short word", "hello", 10, []string{"hello"}},
		{"two words fit", "hello world", 11, []string{"hello world"}},
		{"two words wrap", "hello world", 8, []string{"hello", "world"}},
		{"several words", "a b c d e", 3, []string{"a b", "c d", "e"}},
		{"newlines preserved", "line1\nline2", 20, []string{"line1", "line2"}},
		{"CRLF split", "line1\r\nline2", 20, []string{"line1", "line2"}},
		{"lone CR split", "line1\rline2", 20, []string{"line1", "line2"}},
		{"all line endings", "first\nsecond\r\nthird\rfourth", 80, []string{"first", "second", "third", "fourth"}},
		{"newline then wrap", "hello world\nfoo bar", 8, []string{"hello", "world", "foo bar"}},
		{"long word broken", "abcdefghij", 4, []string{"abcd", "efgh", "ij"}},
		{"long word after short word", "ab cdefghijk", 4, []string{"ab", "cdef", "ghij", "k"}},
		{"color across CR line endings", "\x1b[31mfirst\r\nsecond\rthird" + reset, 80, []string{"\x1b[31mfirst", "\x1b[31msecond", "\x1b[31mthird" + reset}},
		{"color reopened after wrap", "\x1b[31mhello world" + reset, 8, []string{"\x1b[31mhello", "\x1b[31mworld" + reset}},
		{"underline closed and reopened", "\x1b[4munderlined wraps" + reset, 11, []string{"\x1b[4munderlined\x1b[24m", "\x1b[4mwraps" + reset}},
		{"CJK breaks within remaining width", "日本語テスト hello world 你好世界 test", 30, []string{"日本語テスト hello world 你好", "世界 test"}},
		{"CJK run after latin", "This is an example 中文汉字测试段落内容中文汉字测试段落内容.", 40, []string{"This is an example 中文汉字测试段落内容", "中文汉字测试段落内容."}},
		{"CJK run keeps color", "\x1b[31mThis is an example 中文汉字测试段落内容中文汉字测试段落内容." + reset, 40, []string{"\x1b[31mThis is an example 中文汉字测试段落内容", "\x1b[31m中文汉字测试段落内容." + reset}},
		{"wide CJK word broken", "你好世界", 4, []string{"你好", "世界"}},
		{"styled run fits on one line", "\x1b[1mhi" + reset + " world", 10, []string{"\x1b[1mhi" + reset + " world"}},
		{"trailing whitespace trimmed", "  ", 1, []string{""}},
		{"underline not applied before styled text", "read this thread \x1b[4mhttps://example.com/very/long/path/that/will/wrap\x1b[24m", 40, []string{"read this thread", "\x1b[4mhttps://example.com/very/long/path/that/\x1b[24m", "\x1b[4mwill/wrap\x1b[24m"}},
		{"hyperlink closed and reopened", "\x1b]8;;https://example.com\x1b\\0123456789\x1b]8;;\x1b\\", 6, []string{"\x1b]8;;https://example.com\x1b\\012345\x1b]8;;\x1b\\", "\x1b]8;;https://example.com\x1b\\6789\x1b]8;;\x1b\\"}},
		{"hyperlink outside wrapped text is not repeated", "before \x1b]8;;https://example.com\x1b\\link\x1b]8;;\x1b\\ after", 80, []string{"before \x1b]8;;https://example.com\x1b\\link\x1b]8;;\x1b\\ after"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := WrapTextWithAnsi(tc.in, tc.width)
			if !slices.Equal(got, tc.want) {
				t.Errorf("WrapTextWithAnsi(%q, %d)\n got  %q\n want %q", tc.in, tc.width, got, tc.want)
			}
			for _, line := range got {
				if w := VisibleWidth(line); w > tc.width {
					t.Errorf("line %q width %d > %d", line, w, tc.width)
				}
			}
		})
	}
}

func TestWrapTextWithAnsiStyleInvariants(t *testing.T) {
	const underlineOn, underlineOff, reset = "\x1b[4m", "\x1b[24m", "\x1b[0m"
	t.Run("no whitespace before an underline reset", func(t *testing.T) {
		got := WrapTextWithAnsi(underlineOn+"underlined text here "+underlineOff+"more", 18)
		if len(got) == 0 || strings.Contains(got[0], " "+underlineOff) {
			t.Fatalf("lines = %q", got)
		}
	})
	t.Run("underline does not bleed into padding", func(t *testing.T) {
		got := WrapTextWithAnsi("prefix "+underlineOn+"https://example.com/very/long/path/that/will/definitely/wrap"+underlineOff+" suffix", 30)
		for i := 1; i < len(got)-1; i++ {
			line := got[i]
			if strings.Contains(line, underlineOn) && (!strings.HasSuffix(line, underlineOff) || strings.HasSuffix(line, reset)) {
				t.Errorf("line = %q", line)
			}
		}
	})
	t.Run("background preserved without a full reset", func(t *testing.T) {
		got := WrapTextWithAnsi("\x1b[44mhello world this is blue background text"+reset, 15)
		if len(got) < 2 {
			t.Fatalf("lines = %q", got)
		}
		for i, line := range got {
			if !strings.Contains(line, "\x1b[44m") || (i < len(got)-1 && strings.HasSuffix(line, reset)) {
				t.Errorf("line = %q", line)
			}
		}
	})
	t.Run("underline reset but background preserved", func(t *testing.T) {
		got := WrapTextWithAnsi("\x1b[41mprefix "+underlineOn+"UNDERLINED_CONTENT_THAT_WRAPS"+underlineOff+" suffix"+reset, 20)
		for i, line := range got {
			if !strings.Contains(line, "41") {
				t.Errorf("background missing: %q", line)
			}
			on := strings.Contains(line, "[4m") || strings.Contains(line, "[4;") || strings.Contains(line, ";4m")
			if i < len(got)-1 && on && !strings.HasSuffix(line, underlineOff) {
				t.Errorf("underline reset missing: %q", line)
			}
		}
	})
	t.Run("color reopened on every continuation line", func(t *testing.T) {
		got := WrapTextWithAnsi("\x1b[31mhello world this is red"+reset, 10)
		if len(got) < 2 {
			t.Fatalf("lines = %q", got)
		}
		for i, line := range got {
			if i > 0 && !strings.HasPrefix(line, "\x1b[31m") {
				t.Errorf("red prefix missing: %q", line)
			}
			if i < len(got)-1 && strings.HasSuffix(line, reset) {
				t.Errorf("full reset: %q", line)
			}
		}
	})
	t.Run("BEL terminators preserved when reopening hyperlinks", func(t *testing.T) {
		url := "https://example.com/oauth/" + strings.Repeat("a", 32)
		lines := WrapTextWithAnsi("\x1b]8;;"+url+"\x07"+url+"\x1b]8;;\x07", 20)
		if len(lines) <= 1 {
			t.Fatal(lines)
		}
		for i, line := range lines {
			if !strings.Contains(line, "\x1b]8;;"+url+"\x07") || strings.Contains(line, "\x1b]8;;"+url+"\x1b\\") {
				t.Errorf("wrong opener: %q", line)
			}
			if i < len(lines)-1 && !strings.HasSuffix(line, "\x1b]8;;\x07") {
				t.Errorf("wrong close: %q", line)
			}
		}
	})
	t.Run("plain text lines stay within width", func(t *testing.T) {
		got := WrapTextWithAnsi("hello world this is a test", 10)
		if len(got) <= 1 {
			t.Fatal(got)
		}
		for _, line := range got {
			if VisibleWidth(line) > 10 {
				t.Fatal(line)
			}
		}
	})
}

// Long words break between graphemes measured in cells, with SGR and OSC 8
// state carried across every break.
func TestWrapTextWithAnsiGraphemeCellWidths(t *testing.T) {
	for _, glyph := range []struct {
		name, text string
		cells      int
	}{
		{"warning", "⚠️", 2},
		{"flag", "🇺🇸", 2},
		{"cjk", "界", 2},
		{"zwj", "👩‍💻", 2},
		{"combining", "é", 1},
		{"ascii", "a", 1},
	} {
		for _, style := range []struct{ name, open, lineEnd, close string }{
			{"plain", "", "", ""},
			{"sgr", "\x1b[4;31;44m", "\x1b[24m", "\x1b[0m"},
			{"bel-link", "\x1b[4;31;44m\x1b]8;id=wrap;https://example.test\x07", "\x1b[24m\x1b]8;;\x07", "\x1b]8;;\x07\x1b[0m"},
			{"st-link", "\x1b[4;31;44m\x1b]8;id=wrap;https://example.test\x1b\\", "\x1b[24m\x1b]8;;\x1b\\", "\x1b]8;;\x1b\\\x1b[0m"},
		} {
			for _, perRow := range []int{1, 2, 3} {
				t.Run(fmt.Sprintf("%s/%s/%d", glyph.name, style.name, perRow), func(t *testing.T) {
					text := style.open + strings.Repeat(glyph.text, 3) + style.close
					if got := VisibleWidth(text); got != 3*glyph.cells {
						t.Fatalf("source width = %d, want %d", got, 3*glyph.cells)
					}
					var want []string
					for remaining := 3; remaining > 0; remaining -= perRow {
						n := min(remaining, perRow)
						end := style.lineEnd
						if remaining <= perRow {
							end = style.close
						}
						want = append(want, style.open+strings.Repeat(glyph.text, n)+end)
					}
					got := WrapTextWithAnsi(text, perRow*glyph.cells)
					if !slices.Equal(got, want) {
						t.Fatalf("wrapped = %q, want %q", got, want)
					}
					if plain := StripAnsi(strings.Join(got, "")); plain != strings.Repeat(glyph.text, 3) {
						t.Fatalf("text changed: %q", plain)
					}
				})
			}
		}
	}
}

func TestAnsiCodeTracker(t *testing.T) {
	cases := []struct {
		name        string
		codes       string
		wantActive  string
		wantLineEnd string
	}{
		{"nothing", "", "", ""},
		{"bold red", "\x1b[1;31m", "\x1b[1;31m", ""},
		{"reset clears", "\x1b[1;31m\x1b[0m", "", ""},
		{"empty params reset", "\x1b[1m\x1b[m", "", ""},
		{"256 color", "\x1b[38;5;208m", "\x1b[38;5;208m", ""},
		{"rgb background", "\x1b[48;2;1;2;3m", "\x1b[48;2;1;2;3m", ""},
		{"bold off keeps color", "\x1b[1;32m\x1b[22m", "\x1b[32m", ""},
		{"underline needs a line end reset", "\x1b[4m", "\x1b[4m", "\x1b[24m"},
		{"hyperlink survives SGR reset", "\x1b]8;;https://e.com\x07\x1b[0m", "\x1b]8;;https://e.com\x07", "\x1b]8;;\x07"},
		{"hyperlink closed", "\x1b]8;;https://e.com\x1b\\\x1b]8;;\x1b\\", "", ""},
		{"malformed hyperlink ignored", "\x1b]8;\x07", "", ""},
		{"default foreground", "\x1b[31m\x1b[39m", "", ""},
		{"bright colors", "\x1b[95;104m", "\x1b[95;104m", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tracker := &ansiCodeTracker{}
			tracker.UpdateFromText(tc.codes)
			if got := tracker.ActiveCodes(); got != tc.wantActive {
				t.Errorf("ActiveCodes = %q, want %q", got, tc.wantActive)
			}
			if got := tracker.LineEndReset(); got != tc.wantLineEnd {
				t.Errorf("LineEndReset = %q, want %q", got, tc.wantLineEnd)
			}
		})
	}
}

// An RGI emoji sequence is one two-column cluster when a line is wrapped: a
// modifier sequence on a text-presentation base and a ZWJ sequence carrying
// U+FE0F are neither split nor measured by their parts.
func TestWrapTextWithAnsiRGIEmojiSequences(t *testing.T) {
	cases := []struct {
		text  string
		width int
		want  []string
	}{
		{"❤️‍🔥", 2, []string{"❤️‍🔥"}},
		{"☝🏻कक", 2, []string{"☝🏻", "कक"}},
		{"☝🏻☝🏻☝🏻", 4, []string{"☝🏻☝🏻", "☝🏻"}},
		{"\U0001FAE9\U0001FAE9", 2, []string{"\U0001FAE9", "\U0001FAE9"}},
		{"a ❤️‍🔥 b", 4, []string{"a ❤️‍🔥", "b"}},
	}
	for _, tc := range cases {
		got := WrapTextWithAnsi(tc.text, tc.width)
		if !slices.Equal(got, tc.want) {
			t.Errorf("WrapTextWithAnsi(%q, %d) = %q, want %q", tc.text, tc.width, got, tc.want)
		}
		for _, line := range got {
			if w := VisibleWidth(line); w > tc.width {
				t.Errorf("line %q of %q is %d columns, want <= %d", line, tc.text, w, tc.width)
			}
		}
	}
}
