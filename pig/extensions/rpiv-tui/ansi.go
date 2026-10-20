package tui

import "strings"

// ExtractAnsiCode returns the escape sequence beginning at byte offset pos in s
// and its byte length, or ("", 0) when none begins there. It mirrors pi-tui's
// extractAnsiCode: a CSI (ESC [) runs to the first m, G, K, H or J, whatever
// lies between, and an OSC (ESC ]) or APC (ESC _) runs to BEL or ST (ESC \).
// Any other sequence, such as cursor movement (CSI A to D) or an unterminated
// one, is not an escape here: its bytes after ESC count as visible text, as
// they do in Pi, so widths agree byte for byte.
func ExtractAnsiCode(s string, pos int) (code string, length int) {
	if pos < 0 || pos+1 >= len(s) || s[pos] != 0x1B {
		return "", 0
	}
	switch s[pos+1] {
	case '[':
		for j := pos + 2; j < len(s); j++ {
			switch s[j] {
			case 'm', 'G', 'K', 'H', 'J':
				return s[pos : j+1], j + 1 - pos
			}
		}
	case ']', '_':
		for j := pos + 2; j < len(s); j++ {
			if s[j] == 0x07 {
				return s[pos : j+1], j + 1 - pos
			}
			if s[j] == 0x1B && j+1 < len(s) && s[j+1] == '\\' {
				return s[pos : j+2], j + 2 - pos
			}
		}
	}
	return "", 0
}

// ExtractAnsi returns the byte length of the escape sequence beginning at byte
// offset i in s, or 0 when none begins there. See ExtractAnsiCode.
func ExtractAnsi(s string, i int) int {
	_, n := ExtractAnsiCode(s, i)
	return n
}

// StripAnsi removes the SGR, OSC and APC sequences ExtractAnsiCode recognizes.
// Other escape sequences stay in place, as in pi-tui.
func StripAnsi(s string) string {
	if !strings.ContainsRune(s, 0x1B) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if n := ExtractAnsi(s, i); n > 0 {
			i += n
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
