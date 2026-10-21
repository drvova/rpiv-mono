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
		{"lone skin tone modifier", "\U0001F3FD", 2},
		{"lone skin tone modifier after a reset", "\x1b[0m\U0001F3FD", 2},
		{"skin tone modifier after a mark adds its own width", "文\u0308\U0001F3FD", 4},
		{"skin tone modifier after a letter adds nothing", "a\U0001F3FD", 1},
		{"skin tone modifiers chain into one cluster", "\U0001F3FD\U0001F3FD\U0001F3FD", 2},
		{"keycap", "1️⃣", 2},
		{"keycap pound sign", "#️⃣", 2},
		{"modifier sequence on a text-presentation base", "☝🏻", 2},
		{"victory hand with medium skin tone", "✌🏽", 2},
		{"detective with light skin tone", "🕵🏻", 2},
		{"person bouncing ball with light skin tone", "⛹🏻", 2},
		{"person lifting weights with light skin tone", "🏋🏻", 2},
		{"ZWJ sequence with VS16 inside", "❤️‍🔥", 2},
		{"rainbow flag ZWJ sequence", "🏳️‍🌈", 2},
		{"kiss ZWJ sequence with VS16 inside", "👩‍❤️‍💋‍👨", 2},
		{"eye in speech bubble ZWJ sequence", "👁️‍🗨️", 2},
		{"woman lifting weights ZWJ sequence", "🏋️‍♀️", 2},
		{"family ZWJ sequence", "🧑‍🧑‍🧒", 2},
		{"handshake with two skin tones", "🫱🏻‍🫲🏿", 2},
		{"ZWJ sequence outside the RGI set is measured by its parts", "🧑‍❤️‍🧑", 4},
		{"modifier sequence with a trailing mark is not an RGI emoji", "☝🏻\u0308", 1},
		{"unicode 16 emoji", "\U0001FAE9", 2},
		{"unicode 17 emoji", "\U0001FA8A", 2},
		{"black star with VS16 is not an RGI emoji", "★️", 1},
		{"white heart suit with VS16 is not an RGI emoji", "♡️", 1},
		{"mahjong tile with VS16 is not an RGI emoji", "\U0001F000\uFE0F", 1},
		{"heart with VS16 and a trailing mark", "❤️\u0308", 1},
		{"warning sign with VS16 and a trailing mark", "⚠️\u0308", 1},
		{"letter with VS16", "a️", 1},
		{"regional indicator pair with a spacing mark", "🇺🇸\u093F", 2},
		{"regional indicator pair with thai am", "🇺🇸\u0E33", 2},
		{"lone regional indicator with a spacing mark", "🇺\u093F", 2},
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
		{"\U0001F3FD", 4, "\U0001F3FD  "},
		{"☝🏻", 4, "☝🏻  "},
		{"❤️‍🔥", 3, "❤️‍🔥 "},
		{"\U0001FAE9", 3, "\U0001FAE9 "},
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

// Code points Unicode 15.1 to 17.0 made East Asian wide, or that uniseg's
// Unicode 15.0 tables measure differently from Pi, take Pi's width from
// wideRanges. The ranges are every one where uniseg.StringWidth disagrees
// with Pi's eastAsianWidth for a printing code point.
func TestWideRangesFollowUnicode17(t *testing.T) {
	cases := []struct {
		lo, hi rune
		want   int
	}{
		{0x2028, 0x2029, 1},   // line and paragraph separator
		{0x2630, 0x2637, 2},   // Yijing trigrams (Unicode 16)
		{0x268A, 0x268F, 2},   // Yijing monograms and digrams (Unicode 16)
		{0x2E3A, 0x2E3B, 1},   // two- and three-em dash
		{0x2FFC, 0x2FFF, 2},   // ideographic description characters (Unicode 15.1)
		{0x3030, 0x3030, 2},   // wavy dash
		{0x303D, 0x303D, 2},   // part alternation mark
		{0x31E4, 0x31E5, 2},   // CJK strokes (Unicode 15.1)
		{0x31EF, 0x31EF, 2},   // CJK stroke (Unicode 15.1)
		{0x3297, 0x3297, 2},   // circled ideograph congratulation
		{0x3299, 0x3299, 2},   // circled ideograph secret
		{0x4DC0, 0x4DFF, 2},   // Yijing hexagrams (Unicode 16)
		{0xFF9E, 0xFF9F, 1},   // halfwidth katakana voiced sound marks
		{0x16FF2, 0x16FF6, 2}, // ideographic symbols (Unicode 17)
		{0x187F8, 0x187FF, 2}, // Tangut (Unicode 17)
		{0x18CFF, 0x18CFF, 2}, // Khitan small script (Unicode 16)
		{0x18D09, 0x18D1E, 2}, // Tangut supplement (Unicode 17)
		{0x18D80, 0x18DF2, 2}, // Tangut components supplement (Unicode 17)
		{0x1D300, 0x1D356, 2}, // Tai Xuan Jing symbols (Unicode 16)
		{0x1D360, 0x1D376, 2}, // counting rod numerals (Unicode 16)
		{0x1F202, 0x1F202, 2}, // squared katakana sa
		{0x1F237, 0x1F237, 2}, // squared CJK unified ideograph-6708
		{0x1F260, 0x1F265, 2}, // rounded symbols for fu, lu, shou, xi, shuangxi, cai
		{0x1F3FB, 0x1F3FF, 2}, // emoji modifiers
		{0x1F6D8, 0x1F6D8, 2}, // Unicode 17 emoji
		{0x1FA89, 0x1FA8A, 2}, // Unicode 16 and 17 emoji
		{0x1FA8E, 0x1FA8F, 2}, // Unicode 17 and 16 emoji
		{0x1FABE, 0x1FABE, 2}, // Unicode 16 emoji
		{0x1FAC6, 0x1FAC6, 2}, // Unicode 16 emoji
		{0x1FAC8, 0x1FAC8, 2}, // Unicode 17 emoji
		{0x1FACD, 0x1FACD, 2}, // Unicode 17 emoji
		{0x1FADC, 0x1FADC, 2}, // Unicode 16 emoji
		{0x1FADF, 0x1FADF, 2}, // Unicode 16 emoji
		{0x1FAE9, 0x1FAEA, 2}, // Unicode 16 and 17 emoji
		{0x1FAEF, 0x1FAEF, 2}, // Unicode 17 emoji
	}
	for _, tc := range cases {
		for r := tc.lo; r <= tc.hi; r++ {
			if got := VisibleWidth(string(r)); got != tc.want {
				t.Errorf("VisibleWidth(%U) = %d, want %d", r, got, tc.want)
			}
		}
	}
}

func TestCouldBeEmoji(t *testing.T) {
	for s, want := range map[string]bool{
		"☝🏻": true, "⌚": true, "⭐": true, "😀": true, "é": false, "a": false, "界": false,
		"★️": true, "a\u200Db": true, "कि": false, "🇺🇸": true, "ab": false, "\u0E33": false,
	} {
		if got := couldBeEmoji(s); got != want {
			t.Errorf("couldBeEmoji(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestIsRGIEmoji(t *testing.T) {
	for s, want := range map[string]bool{
		"😀": true, "⌚": true, "❤️": true, "❤": false, "★️": false, "a": false, "\U0001F3FD": true,
		"🇺": false, "1️⃣": true, "☝🏻": true, "❤️‍🔥": true, "🧑‍❤️‍🧑": false, "☝🏻\u0308": false,
		"🏴󠁧󠁢󠁥󠁮󠁧󠁿": true, "\U0001FAE9": true, "\U0001FA8A": true,
	} {
		if got := isRGIEmoji(s); got != want {
			t.Errorf("isRGIEmoji(%q) = %v, want %v", s, got, want)
		}
	}
}
