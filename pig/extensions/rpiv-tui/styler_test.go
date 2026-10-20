package tui_test

import (
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	tui "github.com/juicesharp/rpiv-mono/pig/extensions/rpiv-tui"
)

// sdk.UITheme satisfies Styler structurally, so host.Theme() can hand the
// real theme to code written against the interface.
var _ tui.Styler = sdk.UITheme{}

func TestUIThemeIsAStyler(t *testing.T) {
	var s tui.Styler = sdk.UITheme{}
	// A theme without a palette colors nothing but still applies modifiers.
	if got := s.Fg("accent", "x"); got != "x" {
		t.Errorf("Fg = %q", got)
	}
	if got := s.Bg("accent", "x"); got != "x" {
		t.Errorf("Bg = %q", got)
	}
	if got := s.Bold("x"); got != "\x1b[1mx\x1b[22m" {
		t.Errorf("Bold = %q", got)
	}
	if got := s.Italic("x"); got != "\x1b[3mx\x1b[23m" {
		t.Errorf("Italic = %q", got)
	}
	if got := s.Underline("x"); got != "\x1b[4mx\x1b[24m" {
		t.Errorf("Underline = %q", got)
	}
	if got := s.Strikethrough("x"); got != "\x1b[9mx\x1b[29m" {
		t.Errorf("Strikethrough = %q", got)
	}
	if got := s.Inverse("x"); got != "\x1b[7mx\x1b[27m" {
		t.Errorf("Inverse = %q", got)
	}
}

func TestPlainStyler(t *testing.T) {
	var s tui.Styler = tui.PlainStyler{}
	for name, got := range map[string]string{
		"Fg": s.Fg("accent", "text"), "Bg": s.Bg("accent", "text"), "Bold": s.Bold("text"), "Italic": s.Italic("text"),
		"Underline": s.Underline("text"), "Strikethrough": s.Strikethrough("text"), "Inverse": s.Inverse("text"),
	} {
		if got != "text" {
			t.Errorf("%s = %q, want text unchanged", name, got)
		}
	}
}

func TestTagStyler(t *testing.T) {
	var s tui.Styler = tui.TagStyler{}
	cases := map[string]struct{ got, want string }{
		"Fg":            {s.Fg("accent", "text"), "<accent>text</accent>"},
		"Bg":            {s.Bg("warning", "text"), "<bg:warning>text</bg:warning>"},
		"Bold":          {s.Bold("text"), "<b>text</b>"},
		"Italic":        {s.Italic("text"), "<i>text</i>"},
		"Underline":     {s.Underline("text"), "<u>text</u>"},
		"Strikethrough": {s.Strikethrough("text"), "<s>text</s>"},
		"Inverse":       {s.Inverse("text"), "<inv>text</inv>"},
		"nested":        {s.Bold(s.Fg("dim", "x")), "<b><dim>x</dim></b>"},
	}
	for name, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", name, tc.got, tc.want)
		}
	}
}
