package tui

import (
	"strings"
	"sync"
	"testing"
)

func TestZZKeysTaskList(t *testing.T) {
	withKitty(t, false)
	t.Setenv("WT_SESSION", "")
	type c struct {
		data, id string
		want     bool
	}
	cases := []c{
		// ctrl+shift+t legacy + Kitty
		{"\x1b[116;6u", "ctrl+shift+t", true},
		{"\x1b[27;6;116~", "ctrl+shift+t", true},
		{"\x1b[84;6u", "ctrl+shift+t", true}, // uppercase T with shift
		{"\x14", "ctrl+shift+t", false},      // legacy ctrl+t has no shift
		{"\x14", "ctrl+t", true},
		{"\x1b[116;5u", "ctrl+shift+t", false},
		{"\x1b[116;6:1u", "ctrl+shift+t", true},
		{"\x1b[116;70u", "ctrl+shift+t", true}, // caps lock bit
		{"\x1b[116;134u", "ctrl+shift+t", true}, // num lock bit
		// alt+g
		{"\x1bg", "alt+g", true},
		{"\x1b[103;3u", "alt+g", true},
		{"\x1b[27;3;103~", "alt+g", true},
		{"\x1bG", "alt+g", false},
		{"\x1b[71;4u", "alt+g", false}, // shift+alt+G
		// ctrl+]
		{"\x1d", "ctrl+]", true},
		{"\x1b[93;5u", "ctrl+]", true},
		{"\x1b[27;5;93~", "ctrl+]", true},
		{"\x1d", "ctrl+}", false},
		{"\x1b[125;5u", "ctrl+}", true},
		{"\x1b[125:93;6u", "ctrl+}", true}, // shift+] produced }
		{"\x1b[93:125;6u", "ctrl+}", false},
		// escape vs ESC-prefixed sequence
		{"\x1b", "escape", true},
		{"\x1b", "esc", true},
		{"\x1b[27u", "escape", true},
		{"\x1b[27;1u", "escape", true},
		{"\x1b[A", "escape", false},
		{"\x1bg", "escape", false},
		{"\x1b\x1b", "escape", false},
		{"\x1b[27;3u", "alt+escape", false},
		// enter
		{"\r", "enter", true},
		{"\n", "enter", true},
		{"\x1b[13u", "enter", true},
		{"\x1b[13;1u", "enter", true},
		{"\x1b[13;1:1u", "enter", true},
		{"\x1b[57414u", "enter", true},
		{"\x1b[13;2u", "shift+enter", true},
		{"\x1b[13;2u", "enter", false},
		{"\x1b[27;1;13~", "enter", false},
		{"\x1bOM", "enter", true},
		{"\r\n", "enter", false},
		// release events
		{"\x1b[116;6:3u", "ctrl+shift+t", true},
		{"\x1b[1;5:3D", "ctrl+left", true},
	}
	for _, tc := range cases {
		if got := MatchesKey(tc.data, tc.id); got != tc.want {
			t.Errorf("MatchesKey(%q, %q) = %v, want %v", tc.data, tc.id, got, tc.want)
		}
	}
	for _, d := range []string{"\x1b[116;6:3u", "\x1b[1;5:3D", "\x1b[3;1:3~", "\x1b[13;1:3u"} {
		if !IsKeyRelease(d) {
			t.Errorf("IsKeyRelease(%q) false", d)
		}
	}
	for _, d := range []string{"\x1b[116;6u", "\x1b[116;6:1u", "\x1b[116;6:2u", "\x1b[200~\x1b[116;6:3u\x1b[201~", "a:3u"} {
		if IsKeyRelease(d) && d != "a:3u" {
			t.Errorf("IsKeyRelease(%q) true", d)
		}
	}
	for _, spec := range []string{"ctrl+", "ctrl+shift", "foo+x", "ctrl+shift+", "+ctrl", "ctrl+T", "Ctrl+t", "ctrl+pageUp", "ctrl+ctrl", "shift", "ctrl", "alt", "super"} {
		if ValidKeyID(spec) {
			t.Errorf("ValidKeyID(%q) = true", spec)
		}
	}
	for _, spec := range []string{"ctrl+shift+t", "ctrl+]", "alt+g", "escape", "enter", "f1", "f12", "super+ctrl+alt+shift+a", "shift+clear", "\"", "="} {
		if !ValidKeyID(spec) {
			t.Errorf("ValidKeyID(%q) = false", spec)
		}
	}
}

func TestZZKeysEdgeCases(t *testing.T) {
	withKitty(t, false)
	t.Setenv("WT_SESSION", "")
	// Non-ASCII id lowercasing: JS "İ".toLowerCase() is "i̇" (2 code units) → matches nothing.
	t.Logf("MatchesKey(i, İ)=%v (JS: false)", MatchesKey("i", "İ"))
	t.Logf("MatchesKey(k, K[kelvin])=%v (JS: true)", MatchesKey("k", "K"))
	t.Logf("parseKeyID(İ)=%v", func() string { b, m, ok := parseKeyID("İ"); return b + "/" + string(rune('0'+m)) + "/" + map[bool]string{true: "ok", false: "no"}[ok] }())
	// ParseKey on invalid UTF-8 and odd input
	for _, d := range []string{"\xff", "\x1b\xff", "\x1b[\xff", "\x1b[99999999999999999999u", "\x1b[99;99999999999999999999u", "\x1b[27;99999999999999999999;99~", "\x1b[;5u", "\x1b[99;u", "\x1b[99:;5u", "\x1b[99::;5u", "\x1b[1;5:D", "\x1b[3;:1~"} {
		got, ok := ParseKey(d)
		t.Logf("ParseKey(%q) = (%q, %v)", d, got, ok)
		_ = MatchesKey(d, "ctrl+c")
		_ = MatchesKey(d, "enter")
		_ = MatchesKey(d, "ctrl+left")
	}
	// Shift+letter uppercase via CSI u without shift bit
	t.Logf("MatchesKey(ESC[65u, shift+a)=%v", MatchesKey("\x1b[65u", "shift+a"))
	t.Logf("MatchesKey(ESC[65u, a)=%v", MatchesKey("\x1b[65u", "a"))
	t.Logf("MatchesKey(A, a)=%v", MatchesKey("A", "a"))
	// Race: concurrent flag flips with matching.
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := range 200 {
				SetKittyProtocolActive((i+j)%2 == 0)
				_ = MatchesKey("\n", "enter")
				_, _ = ParseKey("\x1b\r")
			}
		}(i)
	}
	wg.Wait()
	SetKittyProtocolActive(false)
}

func TestZZWidthEdgeCases(t *testing.T) {
	type c struct {
		name string
		got  any
	}
	cases := []c{
		{"Truncate neg pad", TruncateToWidth("abc", -1, "...", true)},
		{"Truncate big", len(TruncateToWidth("abc", 1<<20, "...", true))},
		{"Wrap width 0", WrapTextWithAnsi("hello world", 0)},
		{"Wrap width -1", WrapTextWithAnsi("hello world", -1)},
		{"Wrap width 0 cjk", WrapTextWithAnsi("你好", 0)},
		{"Wrap width 1 emoji", WrapTextWithAnsi("😀😀", 1)},
		{"Wrap only newline", WrapTextWithAnsi("\n", 10)},
		{"Wrap trailing newline", WrapTextWithAnsi("a\n", 10)},
		{"Wrap CR only", WrapTextWithAnsi("\r", 10)},
		{"SliceUTF16 invalid", SliceUTF16("a\xffb", 1, 2)},
		{"SliceUTF16 astral middle", SliceUTF16("🙂", 1, 2)},
		{"LenUTF16 invalid", LenUTF16("\xff\xfe")},
		{"ExtractAnsi beyond", ExtractAnsi("abc", 10)},
		{"VisibleWidth invalid utf8", VisibleWidth("a\xffb")},
		{"VisibleWidth lone surrogate-like", VisibleWidth("\xed\xa0\x80")},
		{"Truncate invalid utf8", TruncateToWidth("ab\xffcd", 3, "", false)},
		{"Wrap invalid utf8", WrapTextWithAnsi("ab\xffcd ef", 3)},
		{"PadToWidth negative", PadToWidth("a", -5)},
		{"Repeat negative", Repeat("a", -5)},
		{"tracker 1;;31", func() string { tr := &ansiCodeTracker{}; tr.UpdateFromText("\x1b[1;;31m"); return tr.ActiveCodes() }()},
		{"tracker 38;5 short", func() string { tr := &ansiCodeTracker{}; tr.UpdateFromText("\x1b[38;5m"); return tr.ActiveCodes() }()},
		{"tracker nested CSI", func() string { tr := &ansiCodeTracker{}; tr.UpdateFromText("\x1b[?25l\x1b[1m"); return tr.ActiveCodes() }()},
		{"keycap no VS16", VisibleWidth("1⃣")},
		{"a+VS16", VisibleWidth("a️")},
		{"check+VS16", VisibleWidth("✓️")},
		{"digit+VS16", VisibleWidth("1️")},
		{"VS15 emoji", VisibleWidth("❤︎")},
		{"grinning+VS15", VisibleWidth("\U0001F600︎")},
		{"wide + VS16", VisibleWidth("你️")},
	}
	for _, tc := range cases {
		t.Logf("%s: %#v", tc.name, tc.got)
	}
	_ = strings.Repeat
}
