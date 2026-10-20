package tui

import "testing"

func TestVisibleWidth(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"ascii", "hello", 5},
		{"tab expands to 3 columns", "a\tb", 5},
		{"two tabs", "\t\t", 6},
		{"styled ascii", "\x1b[1;31mhello\x1b[0m", 5},
		{"256-color styled", "\x1b[38;5;4mhello\x1b[39m world", 11},
		{"osc 8 hyperlink", "\x1b]8;;https://example.com\x07link\x1b]8;;\x07", 4},
		{"osc 133 with ST", "\x1b]133;A\x1b\\prompt", 6},
		{"osc 133 with BEL pair", "\x1b]133;A\x07hello\x1b]133;B\x07", 5},
		{"apc cursor marker", "\x1b_pi:c\x07cursor", 6},
		{"cursor marker between letters", "a\x1b_pi:c\x07b", 2},
		{"tab inside styled text", "\x1b[1ma\tb\x1b[22m", 5},
		{"wide CJK", "你好", 4},
		{"mixed ascii and CJK", "a你b好", 6},
		{"styled CJK", "\x1b[31m日本\x1b[39m ok", 7},
		{"styled wide char", "\x1b[31m世\x1b[0m", 2},
		{"box drawing and arrow", "\x1b[31m─→\x1b[39m", 2},
		{"emoji", "👋", 2},
		{"rocket", "\U0001F680", 2},
		{"check mark with emoji presentation", "✅", 2},
		{"warning sign with VS16", "⚠️", 2},
		{"information source with VS16", "ℹ️", 2},
		{"heart with VS16", "❤️", 2},
		{"bare warning sign keeps text presentation", "⚠", 1},
		{"regional indicator pair", "\U0001F1FA\U0001F1F8", 2},
		{"lone regional indicator", "🇨", 2},
		{"flag", "🇨🇳", 2},
		{"zwj family", "👩‍💻", 2},
		{"skin tone modifier", "👍🏽", 2},
		{"keycap", "1️⃣", 2},
		{"tree row fragment", "1. ✅ fires 2. ⚠️ null-byte", 27},
		{"combining acute is zero width", "é", 1},
		{"latin with diacritics", "čřžůú", 5},
		{"hebrew with points", "שָׁ", 1},
		{"arabic shadda", "بّ", 1},
		{"tibetan subjoined", "རྐ", 1},
		{"tagalog virama", "ᜠ᜴", 1},
		{"hangul with tone mark", "가〮", 2},
		{"hangul with other tone mark", "가〯", 2},
		{"zero width joiner alone", "‍", 0},
		{"zero width space", "​", 0},
		{"soft hyphen", "a­b", 2},
		{"CJK words", "网络", 4},
		{"katakana", "ネットワーク", 12},
		{"hiragana", "が", 2},
		{"hiragana with combining dakuten", "が", 2},
		{"devanagari conjunct", "र्क", 2},
		{"devanagari word", "नेटवर्क", 5},
		{"devanagari sentence", "सर्वाधिकार सुरक्षित। ऑर्डर पर क्लिक करें", 33},
		{"bengali conjunct", "র্ক", 2},
		{"gujarati conjunct", "ર્ક", 2},
		{"oriya conjunct", "ର୍କ", 2},
		{"telugu conjunct", "ర్క", 2},
		{"malayalam chillu", "ര്‍ക", 2},
		{"myanmar aa", "ကာ", 2},
		{"myanmar e", "ကေ", 2},
		{"myanmar asat", "က်", 2},
		{"myanmar medial ya", "ကျ", 2},
		{"myanmar medial ra", "ကြ", 2},
		{"myanmar ii mon", "ကဳ", 2},
		{"myanmar uu mon", "ကဴ", 2},
		{"myanmar e mon", "ကဵ", 2},
		{"myanmar visarga", "ကး", 2},
		{"myanmar i u", "ကို", 1},
		{"myanmar virama", "က္", 1},
		{"thai am alone", "ำ", 1},
		{"lao am alone", "ຳ", 1},
		{"thai ka am", "กำ", 2},
		{"lao ka am", "ກຳ", 2},
		{"unterminated CSI counts its bytes", "\x1b[31", 3},
		{"lone ESC is zero width", "a\x1b", 1},
		{"DEC private mode is visible", "\x1b[?2026h", 7},
		{"CSI swallows bytes up to m", "\x1b[?25lhi\x1b[0m", 0},
		{"control bytes are zero width", "a\x07b\x7f", 2},
		{"CRLF is zero width", "a\r\nb", 2},
		{"two-em dash", "⸺", 1},
		{"tab in CJK text", "\t\x1b[31m界\x1b[0m", 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := VisibleWidth(tc.in); got != tc.want {
				t.Errorf("VisibleWidth(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestPadToWidth(t *testing.T) {
	cases := []struct {
		in    string
		width int
		want  string
	}{
		{"hi", 5, "hi   "},
		{"hello", 5, "hello"},
		{"too long", 3, "too long"},
		{"", 2, "  "},
		{"\x1b[31mred\x1b[0m", 5, "\x1b[31mred\x1b[0m  "},
		{"你好", 6, "你好  "},
		{"x", -1, "x"},
	}
	for _, tc := range cases {
		if got := PadToWidth(tc.in, tc.width); got != tc.want {
			t.Errorf("PadToWidth(%q, %d) = %q, want %q", tc.in, tc.width, got, tc.want)
		}
	}
}

func TestGraphemeIterWalksClusters(t *testing.T) {
	var clusters []string
	var widths []int
	for g := newGraphemeIter("ab界👩‍💻é"); g.Next(); {
		clusters = append(clusters, g.Str())
		widths = append(widths, g.Width())
	}
	wantClusters := []string{"a", "b", "界", "👩‍💻", "é"}
	wantWidths := []int{1, 1, 2, 2, 1}
	if len(clusters) != len(wantClusters) {
		t.Fatalf("clusters = %q, want %q", clusters, wantClusters)
	}
	for i := range clusters {
		if clusters[i] != wantClusters[i] || widths[i] != wantWidths[i] {
			t.Errorf("cluster %d = (%q, %d), want (%q, %d)", i, clusters[i], widths[i], wantClusters[i], wantWidths[i])
		}
	}
}

func TestIsCJKBreak(t *testing.T) {
	for s, want := range map[string]bool{"中": true, "か": true, "カ": true, "한": true, "ㄅ": true, "。": true, "a": false, " ": false, "é": false, "": false} {
		if got := isCJKBreak(s); got != want {
			t.Errorf("isCJKBreak(%q) = %v, want %v", s, got, want)
		}
	}
}
