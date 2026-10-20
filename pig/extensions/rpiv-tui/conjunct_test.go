package tui

import (
	"slices"
	"testing"
)

// Indic conjuncts stay one cluster (UAX #29 GB9c), as ICU segments them for
// Pi, so truncation and wrapping never cut between a virama and the consonant
// it joins.
func TestIndicConjunctsAreOneCluster(t *testing.T) {
	for _, tc := range []struct {
		text  string
		want  []string
		width int
	}{
		{"र्क", []string{"र्क"}, 2},
		{"नेटवर्क", []string{"ने", "ट", "व", "र्क"}, 5},
		{"ര്‍ക", []string{"ര്‍ക"}, 2},
		{"क्ष्म", []string{"क्ष्म"}, 3},
		{"हिन्दी", []string{"हि", "न्दी"}, 5},
		{"கஷ", []string{"க", "ஷ"}, 2},
	} {
		var clusters []string
		width := 0
		for g := newGraphemeIter(tc.text); g.Next(); {
			clusters = append(clusters, g.Str())
			width += g.Width()
		}
		if !slices.Equal(clusters, tc.want) {
			t.Errorf("clusters of %q = %q, want %q", tc.text, clusters, tc.want)
		}
		if width != tc.width {
			t.Errorf("width of %q = %d, want %d", tc.text, width, tc.width)
		}
	}
	if got := TruncateToWidth("र्क", 1, "", false); got != "\x1b[0m" {
		t.Errorf("TruncateToWidth(र्क, 1) = %q, want only a reset", got)
	}
	if got := WrapTextWithAnsi("नेटवर्क", 4); !slices.Equal(got, []string{"नेटव", "र्क"}) {
		t.Errorf("WrapTextWithAnsi(नेटवर्क, 4) = %q", got)
	}
	if got := WrapTextWithAnsi("सर्वाधिकार सुरक्षित। ऑर्डर पर क्लिक करें", 4); !slices.Equal(got, []string{"सर्वा", "धिका", "र", "सुर", "क्षित", "।", "ऑर्डर", "पर", "क्लिक", "करें"}) {
		t.Errorf("WrapTextWithAnsi(devanagari, 4) = %q", got)
	}
}

// Code points uniseg measures differently from Pi are overridden to Pi's width.
func TestWidthOverridesMatchPi(t *testing.T) {
	for s, want := range map[string]int{
		"a b": 3, "a b": 3, "ｶﾞ": 2, "ﾊﾟ": 2, "〰": 2, "〽": 2, "㊗": 2, "㊙": 2,
		"\U0001F202": 2, "\U0001F237": 2, "☰": 2, "䷀": 2, "\U0001D300": 2, "\U0001D360": 2, "⸺": 1,
	} {
		if got := VisibleWidth(s); got != want {
			t.Errorf("VisibleWidth(%q) = %d, want %d", s, got, want)
		}
	}
}
