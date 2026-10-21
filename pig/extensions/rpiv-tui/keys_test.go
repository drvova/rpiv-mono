package tui

import "testing"

// withKitty runs the test with the Kitty flag set, restoring legacy mode after.
func withKitty(t *testing.T, active bool) {
	t.Helper()
	SetKittyProtocolActive(active)
	t.Cleanup(func() { SetKittyProtocolActive(false) })
}

func TestParseKeyLegacyAndCSI(t *testing.T) {
	withKitty(t, false)
	t.Setenv("WT_SESSION", "")
	cases := []struct{ data, want string }{
		{"\x1b[27;5;99~", "ctrl+c"},
		{"\x1b[27;5;100~", "ctrl+d"},
		{"\x1b[27;5;122~", "ctrl+z"},
		{"\x1b[27;5;13~", "ctrl+enter"},
		{"\x1b[27;2;13~", "shift+enter"},
		{"\x1b[27;3;13~", "alt+enter"},
		{"\x1b[27;2;9~", "shift+tab"},
		{"\x1b[27;5;9~", "ctrl+tab"},
		{"\x1b[27;3;9~", "alt+tab"},
		{"\x1b[27;1;127~", "backspace"},
		{"\x1b[27;5;127~", "ctrl+backspace"},
		{"\x1b[27;3;127~", "alt+backspace"},
		{"\x1b[27;1;27~", "escape"},
		{"\x1b[27;1;32~", "space"},
		{"\x1b[27;5;32~", "ctrl+space"},
		{"\x1b[27;5;47~", "ctrl+/"},
		{"\x1b[27;5;49~", "ctrl+1"},
		{"\x1b[27;2;49~", "shift+1"},
		{"\x1b[27;2;69~", "shift+e"},
		{"\x1b[27;6;69~", "shift+ctrl+e"},
		{"\x1b[27;6;43~", "ctrl++"},
		{"\x1b[104;7u", "ctrl+alt+h"},
		{"\x1b[116;6u", "shift+ctrl+t"},
		{"\x1b[103;3u", "alt+g"},
		{"\x1b[97;11u", "alt+super+a"},
		{"\x1b[97;10u", "shift+super+a"},
		{"\x1b[", "alt+["},
		{"\x1b[97;66u", "shift+a"},
		{"\x1b[57414u", "enter"},
		{"\x1b[57399u", "0"},
		{"\x1b[57417;5u", "ctrl+left"},
		{"\x1b[1089::99;5u", "ctrl+c"},
		{"\x1b[27;7;104~", "ctrl+alt+h"},
		{"\x03", "ctrl+c"},
		{"\x04", "ctrl+d"},
		{"\x1b", "escape"},
		{"\t", "tab"},
		{"\r", "enter"},
		{"\n", "enter"},
		{"\x00", "ctrl+space"},
		{" ", "space"},
		{"1", "1"},
		{"+", "+"},
		{"\x1c", "ctrl+\\"},
		{"\x1d", "ctrl+]"},
		{"\x1f", "ctrl+-"},
		{"\x1b\x1b", "ctrl+alt+["},
		{"\x1b\x1c", "ctrl+alt+\\"},
		{"\x1b\x1d", "ctrl+alt+]"},
		{"\x1b\x1f", "ctrl+alt+-"},
		{"\x7f", "backspace"},
		{"\x08", "backspace"},
		{"\x1b ", "alt+space"},
		{"\x1b\x08", "alt+backspace"},
		{"\x1b\x03", "ctrl+alt+c"},
		{"\x1bB", "alt+left"},
		{"\x1bF", "alt+right"},
		{"\x1ba", "alt+a"},
		{"\x1b1", "alt+1"},
		{"\x1b,", "alt+,"},
		{"\x1b.", "alt+."},
		{"\x1by", "alt+y"},
		{"\x1bz", "alt+z"},
		{"\x1b[A", "up"},
		{"\x1b[B", "down"},
		{"\x1b[C", "right"},
		{"\x1b[D", "left"},
		{"\x1bOA", "up"},
		{"\x1bOB", "down"},
		{"\x1bOC", "right"},
		{"\x1bOD", "left"},
		{"\x1bOH", "home"},
		{"\x1bOF", "end"},
		{"\x1b[7~", "home"},
		{"\x1b[8~", "end"},
		{"\x1b[7:3~", "home"},
		{"\x1b[8;1:2~", "end"},
		{"\x1b[1;5H", "ctrl+home"},
		{"\x1b[1;5F", "ctrl+end"},
		{"\x1b[1;6D", "shift+ctrl+left"},
		{"\x1b[1;2A", "shift+up"},
		{"\x1b[5;5~", "ctrl+pageUp"},
		{"\x1b[6;5~", "ctrl+pageDown"},
		{"\x1b[3;3~", "alt+delete"},
		{"\x1bOP", "f1"},
		{"\x1b[24~", "f12"},
		{"\x1b[E", "clear"},
		{"\x1b[2^", "ctrl+insert"},
		{"\x1bp", "alt+up"},
		{"\x1b[[5~", "pageUp"},
		{"\x1b[Z", "shift+tab"},
		{"\x1b\r", "alt+enter"},
		{"\x1bOM", "enter"},
	}
	for _, tc := range cases {
		got, ok := ParseKey(tc.data)
		if !ok || got != tc.want {
			t.Errorf("ParseKey(%q) = (%q, %v), want (%q, true)", tc.data, got, ok, tc.want)
		}
	}
}

func TestParseKeyRejectsUnknownInput(t *testing.T) {
	withKitty(t, false)
	for _, data := range []string{"", "\x1b[200~", "ab", "\x1b[1;1:3", "\x1b[99;257u", "\x1b[é", "é", "\x1b[27;5~"} {
		if got, ok := ParseKey(data); ok {
			t.Errorf("ParseKey(%q) = (%q, true), want no key", data, got)
		}
	}
}

func TestParseKeyKittyMode(t *testing.T) {
	withKitty(t, true)
	cases := []struct {
		data string
		want string
		ok   bool
	}{
		{"\n", "shift+enter", true},
		{"\x1b\r", "shift+enter", true},
		{"\x1b ", "", false},
		{"\x1b\x08", "alt+backspace", true},
		{"\x1b\x03", "", false},
		{"\x1bB", "", false},
		{"\x1bF", "", false},
		{"\x1ba", "", false},
		{"\x1b1", "", false},
		{"\x1b,", "", false},
		{"\x1by", "", false},
		{"\x1bb", "alt+left", true},
		{"\x1b[61:43;2u", "+", true},
		{"\x1b[61:43;6u", "ctrl++", true},
		{"\x1b[122;6:3u", "shift+ctrl+z", true},
	}
	for _, tc := range cases {
		got, ok := ParseKey(tc.data)
		if got != tc.want || ok != tc.ok {
			t.Errorf("ParseKey(%q) = (%q, %v), want (%q, %v)", tc.data, got, ok, tc.want, tc.ok)
		}
	}
}

func TestParseKeyRawBackspaceInWindowsTerminal(t *testing.T) {
	withKitty(t, false)
	t.Setenv("WT_SESSION", "test-session")
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_CLIENT", "")
	t.Setenv("SSH_TTY", "")
	if got, ok := ParseKey("\x08"); !ok || got != "ctrl+backspace" {
		t.Fatalf("ParseKey(BS) = (%q, %v), want ctrl+backspace", got, ok)
	}
	t.Setenv("SSH_CONNECTION", "1 2 3 4")
	if got, ok := ParseKey("\x08"); !ok || got != "backspace" {
		t.Fatalf("ParseKey(BS over SSH) = (%q, %v), want backspace", got, ok)
	}
}

func TestParseKeyID(t *testing.T) {
	cases := []struct {
		input   string
		baseKey string
		mod     int
		ok      bool
	}{
		{"ctrl+c", "c", modCtrl, true},
		{"ctrl+shift+z", "z", modCtrl | modShift, true},
		{"shift+ctrl+z", "z", modCtrl | modShift, true},
		{"ctrl+shift+left", "left", modCtrl | modShift, true},
		{"shift+pageUp", "pageup", modShift, true},
		{"alt+backspace", "backspace", modAlt, true},
		{"ctrl+alt+x", "x", modCtrl | modAlt, true},
		{"super+a", "a", modSuper, true},
		{"a", "a", 0, true},
		{"A", "a", 0, true},
		{"\u0130", "i\u0307", 0, true},
		{"ctrl+\u0130", "i\u0307", modCtrl, true},
		{"escape", "escape", 0, true},
		{"ctrl++", "+", modCtrl, true},
		{"+", "+", 0, true},
		{"ctrl+ctrl+a", "", 0, false},
		{"meta+a", "", 0, false},
		{"ctrl+", "", 0, false},
		{"", "", 0, false},
		{"ctrl+a+b", "", 0, false},
	}
	for _, tc := range cases {
		baseKey, mod, ok := parseKeyID(tc.input)
		if ok != tc.ok || baseKey != tc.baseKey || mod != tc.mod {
			t.Errorf("parseKeyID(%q) = (%q, %d, %v), want (%q, %d, %v)", tc.input, baseKey, mod, ok, tc.baseKey, tc.mod, tc.ok)
		}
	}
}

func TestMatchesKey(t *testing.T) {
	withKitty(t, false)
	t.Setenv("WT_SESSION", "")
	cases := []struct {
		name  string
		data  string
		keyID string
		want  bool
	}{
		{"ctrl+shift+z CSI u", "\x1b[122;6u", "ctrl+shift+z", true},
		{"ctrl+shift+t CSI u", "\x1b[116;6u", "ctrl+shift+t", true},
		{"ctrl+c CSI u", "\x1b[99;5u", "ctrl+c", true},
		{"ctrl+shift+i CSI u", "\x1b[105;6u", "ctrl+shift+i", true},
		{"alt+g CSI u", "\x1b[103;3u", "alt+g", true},
		{"ctrl+] CSI u", "\x1b[93;5u", "ctrl+]", true},
		{"shift+a reported uppercase", "\x1b[65;2u", "shift+a", true},
		{"caps lock ignored", "\x1b[99;69u", "ctrl+c", true},
		{"key release still matches", "\x1b[99;5:3u", "ctrl+c", true},
		{"key repeat still matches", "\x1b[99;5:2u", "ctrl+c", true},
		{"keypad enter is enter", "\x1b[57414u", "enter", true},
		{"cyrillic layout base key", "\x1b[1089::99;5u", "ctrl+c", true},
		{"dvorak codepoint wins over base key", "\x1b[107::118;5u", "ctrl+v", false},
		{"shifted symbol by produced character", "\x1b[61:43;2u", "+", true},
		{"shifted symbol also matches its physical key", "\x1b[61:43;2u", "shift+=", true},
		{"ctrl+plus kitty", "\x1b[61:43;6u", "ctrl++", true},
		{"ctrl+shift+z modifyOtherKeys", "\x1b[27;6;122~", "ctrl+shift+z", true},
		{"ctrl+c modifyOtherKeys", "\x1b[27;5;99~", "ctrl+c", true},
		{"ctrl+plus modifyOtherKeys", "\x1b[27;6;43~", "ctrl++", true},
		{"modifyOtherKeys enter does not match plain enter", "\x1b[27;1;13~", "enter", false},
		{"modifyOtherKeys escape matches escape", "\x1b[27;1;27~", "escape", true},
		{"ctrl+shift+left", "\x1b[1;6D", "ctrl+shift+left", true},
		{"ctrl+shift+right", "\x1b[1;6C", "ctrl+shift+right", true},
		{"shift+up", "\x1b[1;2A", "shift+up", true},
		{"shift+down", "\x1b[1;2B", "shift+down", true},
		{"ctrl+left", "\x1b[1;5D", "ctrl+left", true},
		{"alt+right", "\x1b[1;3C", "alt+right", true},
		{"ctrl+shift+up", "\x1b[1;6A", "ctrl+shift+up", true},
		{"shift+pageUp", "\x1b[5;2~", "shift+pageUp", true},
		{"shift+pageDown", "\x1b[6;2~", "shift+pagedown", true},
		{"ctrl+delete", "\x1b[3;5~", "ctrl+delete", true},
		{"shift+home", "\x1b[1;2H", "shift+home", true},
		{"shift+end", "\x1b[1;2F", "shift+end", true},
		{"ctrl+c legacy", "\x03", "ctrl+c", true},
		{"ctrl+z legacy", "\x1a", "ctrl+z", true},
		{"ctrl+] legacy", "\x1d", "ctrl+]", true},
		{"ctrl+- legacy", "\x1f", "ctrl+-", true},
		{"escape legacy", "\x1b", "escape", true},
		{"esc alias", "\x1b", "esc", true},
		{"enter legacy", "\r", "enter", true},
		{"return alias", "\r", "return", true},
		{"SS3 enter", "\x1bOM", "enter", true},
		{"tab legacy", "\t", "tab", true},
		{"shift+tab legacy", "\x1b[Z", "shift+tab", true},
		{"space legacy", " ", "space", true},
		{"backspace legacy", "\x7f", "backspace", true},
		{"alt+backspace legacy", "\x1b\x7f", "alt+backspace", true},
		{"alt+delete", "\x1b[3;3~", "alt+delete", true},
		{"up legacy", "\x1b[A", "up", true},
		{"down legacy", "\x1b[B", "down", true},
		{"left legacy", "\x1b[D", "left", true},
		{"right legacy", "\x1b[C", "right", true},
		{"left SS3", "\x1bOD", "left", true},
		{"pageUp legacy", "\x1b[5~", "pageUp", true},
		{"pageDown legacy", "\x1b[6~", "pageDown", true},
		{"delete legacy", "\x1b[3~", "delete", true},
		{"insert legacy", "\x1b[2~", "insert", true},
		{"home legacy", "\x1b[H", "home", true},
		{"home rxvt", "\x1b[7~", "home", true},
		{"end legacy", "\x1b[F", "end", true},
		{"end rxvt", "\x1b[8~", "end", true},
		{"shift+up rxvt", "\x1b[a", "shift+up", true},
		{"shift+down rxvt", "\x1b[b", "shift+down", true},
		{"ctrl+up rxvt", "\x1bOa", "ctrl+up", true},
		{"shift+pageUp rxvt", "\x1b[5$", "shift+pageUp", true},
		{"ctrl+delete rxvt", "\x1b[3^", "ctrl+delete", true},
		{"ctrl+end rxvt", "\x1b[8^", "ctrl+end", true},
		{"alt+x legacy", "\x1bx", "alt+x", true},
		{"alt+5 legacy", "\x1b5", "alt+5", true},
		{"alt+- legacy", "\x1b-", "alt+-", true},
		{"alt+= legacy", "\x1b=", "alt+=", true},
		{"alt+/ legacy", "\x1b/", "alt+/", true},
		{"alt+backtick legacy", "\x1b`", "alt+`", true},
		{"alt+up legacy", "\x1bp", "alt+up", true},
		{"alt+left legacy", "\x1bb", "alt+left", true},
		{"ctrl+alt+c legacy", "\x1b\x03", "ctrl+alt+c", true},
		{"shift+a is uppercase", "A", "shift+a", true},
		{"plain a", "a", "a", true},
		{"plain 5", "5", "5", true},
		{"plain plus", "+", "+", true},
		{"uppercase id", "\x03", "Ctrl+C", true},
		{"f1 SS3", "\x1bOP", "f1", true},
		{"f1 CSI", "\x1b[11~", "f1", true},
		{"f5 linux", "\x1b[[E", "f5", true},
		{"f12", "\x1b[24~", "f12", true},
		{"clear", "\x1b[E", "clear", true},
		{"shift+clear", "\x1b[e", "shift+clear", true},
		{"ctrl+clear", "\x1bOe", "ctrl+clear", true},
		{"ctrl+l is not clear", "\x0c", "clear", false},
		{"alt+f1 has no sequence", "\x1bOP", "alt+f1", false},
		{"wrong modifier", "\x1b[1;2D", "ctrl+shift+left", false},
		{"wrong key", "\x1b[1;6C", "ctrl+shift+left", false},
		{"ctrl+c is not ctrl+v", "\x03", "ctrl+v", false},
		{"unmodified is not shifted", "\x1b[A", "shift+up", false},
		{"escape with modifier matches nothing", "\x1b[27;5u", "ctrl+escape", false},
		{"invalid id", "\x03", "ctr+c", false},
		{"duplicate modifier", "\x03", "ctrl+ctrl+c", false},
		{"empty id", "\x03", "", false},
		{"plain letter is not alt+letter", "x", "alt+x", false},
		{"ctrl+j via CSI u", "\x1b[106;5u", "ctrl+j", true},
		{"raw LF is enter in legacy mode", "\n", "enter", true},
		{"raw LF is not shift+enter in legacy mode", "\n", "shift+enter", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MatchesKey(tc.data, tc.keyID); got != tc.want {
				t.Errorf("MatchesKey(%q, %q) = %v, want %v", tc.data, tc.keyID, got, tc.want)
			}
		})
	}
}

func TestMatchesKeyModeAwareLegacyInput(t *testing.T) {
	cases := []struct {
		name  string
		kitty bool
		data  string
		keyID string
		want  bool
	}{
		{"legacy LF is enter", false, "\n", "enter", true},
		{"legacy LF is not shift+enter", false, "\n", "shift+enter", false},
		{"legacy ESC CR is alt+enter", false, "\x1b\r", "alt+enter", true},
		{"legacy ESC CR is not shift+enter", false, "\x1b\r", "shift+enter", false},
		{"Kitty ESC CR is shift+enter", true, "\x1b\r", "shift+enter", true},
		{"Kitty ESC CR is not alt+enter", true, "\x1b\r", "alt+enter", false},
		{"Kitty LF is shift+enter", true, "\n", "shift+enter", true},
		{"Kitty LF is not enter", true, "\n", "enter", false},
		{"Kitty LF keeps ctrl+j", true, "\n", "ctrl+j", true},
		{"legacy ctrl+space", false, "\x00", "ctrl+space", true},
		{"Kitty rejects legacy ctrl+space", true, "\x00", "ctrl+space", false},
		{"legacy alt+space", false, "\x1b ", "alt+space", true},
		{"Kitty rejects legacy alt+space", true, "\x1b ", "alt+space", false},
		{"legacy alt+backspace", false, "\x1b\x08", "alt+backspace", true},
		{"Kitty keeps alt+backspace", true, "\x1b\x08", "alt+backspace", true},
		{"legacy ctrl+alt+c", false, "\x1b\x03", "ctrl+alt+c", true},
		{"Kitty rejects legacy ctrl+alt+c", true, "\x1b\x03", "ctrl+alt+c", false},
		{"legacy uppercase B is alt+left", false, "\x1bB", "alt+left", true},
		{"Kitty rejects uppercase B", true, "\x1bB", "alt+left", false},
		{"Kitty keeps lowercase b alt+left", true, "\x1bb", "alt+left", true},
		{"legacy alt+letter", false, "\x1ba", "alt+a", true},
		{"Kitty rejects legacy alt+letter", true, "\x1ba", "alt+a", false},
		{"legacy alt+digit", false, "\x1b1", "alt+1", true},
		{"Kitty rejects legacy alt+digit", true, "\x1b1", "alt+1", false},
		{"legacy alt+symbol", false, "\x1b,", "alt+,", true},
		{"Kitty rejects legacy alt+symbol", true, "\x1b,", "alt+,", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withKitty(t, tc.kitty)
			if got := MatchesKey(tc.data, tc.keyID); got != tc.want {
				t.Errorf("MatchesKey(%q, %q) with Kitty=%v = %v, want %v", tc.data, tc.keyID, tc.kitty, got, tc.want)
			}
		})
	}
}

func TestMatchesKeyRawBackspaceWindowsTerminal(t *testing.T) {
	withKitty(t, false)
	t.Setenv("WT_SESSION", "")
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_CLIENT", "")
	t.Setenv("SSH_TTY", "")
	assertRawBackspace(t, true, false)
	t.Setenv("WT_SESSION", "test-session")
	assertRawBackspace(t, false, true)
	t.Setenv("SSH_CONNECTION", "1 2 3 4")
	assertRawBackspace(t, true, false)
}

func assertRawBackspace(t *testing.T, wantPlain, wantCtrl bool) {
	t.Helper()
	if got := MatchesKey("\x08", "backspace"); got != wantPlain {
		t.Errorf("BS plain = %v, want %v", got, wantPlain)
	}
	if got := MatchesKey("\x08", "ctrl+backspace"); got != wantCtrl {
		t.Errorf("BS ctrl = %v, want %v", got, wantCtrl)
	}
	if !MatchesKey("\x08", "ctrl+h") {
		t.Error("BS must match ctrl+h")
	}
	if !MatchesKey("\x7f", "backspace") || MatchesKey("\x7f", "ctrl+backspace") {
		t.Error("DEL must match only plain backspace")
	}
}

func TestMatchesKeyLowercasesLikeJavaScript(t *testing.T) {
	withKitty(t, false)
	// "İ".toLowerCase() is "i\u0307", which names no key, so a capital dotted I
	// never matches the i key.
	for _, data := range []string{"i", "\x1b[105u", "\x1b[27;1;105~", "\u0130"} {
		if MatchesKey(data, "\u0130") {
			t.Errorf("MatchesKey(%q, %q) = true, want false", data, "\u0130")
		}
	}
	if MatchesKey("\x09", "ctrl+\u0130") {
		t.Errorf("MatchesKey(ctrl+i byte, %q) = true, want false", "ctrl+\u0130")
	}
	if !MatchesKey("i", "I") || !MatchesKey("\x09", "ctrl+I") {
		t.Errorf("ASCII identifiers still lowercase to their key")
	}
}

func TestIsKeyReleaseAndRepeat(t *testing.T) {
	cases := []struct {
		data            string
		release, repeat bool
	}{
		{"\x1b[99;5:3u", true, false},
		{"\x1b[99;5:2u", false, true},
		{"\x1b[99;5:1u", false, false},
		{"\x1b[99;5u", false, false},
		{"\x1b[1;1:3B", true, false},
		{"\x1b[1;1:2A", false, true},
		{"\x1b[1;5:3H", true, false},
		{"\x1b[1;5:2F", false, true},
		{"\x1b[3;1:3~", true, false},
		{"\x1b[5;1:2~", false, true},
		{"\x1b[B", false, false},
		{"a", false, false},
		{"\x1b[200~:3u:2u\x1b[201~", false, false},
		{"", false, false},
	}
	for _, tc := range cases {
		if got := IsKeyRelease(tc.data); got != tc.release {
			t.Errorf("IsKeyRelease(%q) = %v, want %v", tc.data, got, tc.release)
		}
		if got := IsKeyRepeat(tc.data); got != tc.repeat {
			t.Errorf("IsKeyRepeat(%q) = %v, want %v", tc.data, got, tc.repeat)
		}
	}
}

func TestValidKeyID(t *testing.T) {
	valid := []string{"ctrl+shift+t", "alt+o", "escape", "f5", "f12", "ctrl+]", "a", "0", "ctrl+alt+super+shift+x", "shift+tab", "pageup", "return", "clear", "`", "\\", "ctrl+-", "alt+_"}
	for _, spec := range valid {
		if !ValidKeyID(spec) {
			t.Errorf("ValidKeyID(%q) = false, want true", spec)
		}
	}
	invalid := []string{"", "+", "+t", "ctrl+", "ctrl++t", "ctrl++", "win+t", "meta+a", "ctrl+ctrl+t", "ctr+t", "Alt+O", "ctrl+A", "f13", "f0", "pageUp", "é", "ctrl+é", "🙂", "ctrl+ ", " ", "ctrl+shift"}
	for _, spec := range invalid {
		if ValidKeyID(spec) {
			t.Errorf("ValidKeyID(%q) = true, want false", spec)
		}
	}
}

// Every identifier ValidKeyID accepts is one MatchesKey can parse, so a
// validated collapse key never silently matches nothing.
func TestValidKeyIDsParse(t *testing.T) {
	for _, spec := range []string{"ctrl+shift+t", "alt+o", "escape", "f5", "ctrl+]", "a", "clear", "shift+clear", "pageup", "super+z"} {
		if _, _, ok := parseKeyID(spec); !ok {
			t.Errorf("parseKeyID(%q) rejected a valid identifier", spec)
		}
	}
}

func TestKittyProtocolFlagDefaultsOff(t *testing.T) {
	withKitty(t, false)
	if IsKittyProtocolActive() {
		t.Fatal("Kitty flag should be off by default")
	}
	SetKittyProtocolActive(true)
	if !IsKittyProtocolActive() {
		t.Fatal("Kitty flag should be on after SetKittyProtocolActive(true)")
	}
}
