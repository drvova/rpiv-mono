package tui

import (
	"strconv"
	"strings"
)

// ansiCodeTracker tracks the active SGR attributes and OSC 8 hyperlink across
// a stream of escape sequences, so styling can be re-established after a line
// break. It mirrors pi-tui's AnsiCodeTracker.
type ansiCodeTracker struct {
	bold, dim, italic, underline   bool
	blink, inverse, hidden, strike bool
	fgColor, bgColor               string // SGR parameters, "" for the default
	// activeHyperlinkOpen is the full re-open sequence of the open OSC 8
	// hyperlink, keeping its original terminator because some terminals only
	// make BEL-terminated links clickable; "" when no hyperlink is open.
	activeHyperlinkOpen  string
	activeHyperlinkClose string
}

// Process applies one complete escape sequence, as ExtractAnsiCode returns it.
func (t *ansiCodeTracker) Process(code string) {
	if strings.HasPrefix(code, "\x1b]8;") {
		body := code[len("\x1b]8;"):]
		terminator := "\x1b\\"
		if strings.HasSuffix(code, "\x07") {
			terminator = "\x07"
		}
		body = strings.TrimSuffix(body, terminator)
		if params, url, ok := strings.Cut(body, ";"); ok {
			if url == "" {
				t.activeHyperlinkOpen, t.activeHyperlinkClose = "", ""
			} else {
				t.activeHyperlinkOpen = "\x1b]8;" + params + ";" + url + terminator
				t.activeHyperlinkClose = "\x1b]8;;" + terminator
			}
		}
		return
	}
	if !strings.HasSuffix(code, "m") {
		return
	}
	params, ok := firstSGRParams(code)
	if !ok {
		return
	}
	if params == "" || params == "0" {
		t.resetSGR()
		return
	}
	parts := strings.Split(params, ";")
	for i := 0; i < len(parts); {
		c, err := strconv.Atoi(parts[i])
		if parts[i] == "" || err != nil {
			i++
			continue
		}
		// 256-color and RGB colors span several parameters; the color is the
		// joined span.
		if (c == 38 || c == 48) && i+1 < len(parts) {
			span := 0
			if parts[i+1] == "5" && i+2 < len(parts) {
				span = 3
			} else if parts[i+1] == "2" && i+4 < len(parts) {
				span = 5
			}
			if span > 0 {
				color := strings.Join(parts[i:i+span], ";")
				if c == 38 {
					t.fgColor = color
				} else {
					t.bgColor = color
				}
				i += span
				continue
			}
		}
		switch {
		case c == 0:
			t.resetSGR()
		case c == 1:
			t.bold = true
		case c == 2:
			t.dim = true
		case c == 3:
			t.italic = true
		case c == 4:
			t.underline = true
		case c == 5:
			t.blink = true
		case c == 7:
			t.inverse = true
		case c == 8:
			t.hidden = true
		case c == 9:
			t.strike = true
		case c == 21:
			t.bold = false
		case c == 22:
			t.bold, t.dim = false, false
		case c == 23:
			t.italic = false
		case c == 24:
			t.underline = false
		case c == 25:
			t.blink = false
		case c == 27:
			t.inverse = false
		case c == 28:
			t.hidden = false
		case c == 29:
			t.strike = false
		case c == 39:
			t.fgColor = ""
		case c == 49:
			t.bgColor = ""
		case (c >= 30 && c <= 37) || (c >= 90 && c <= 97):
			t.fgColor = strconv.Itoa(c)
		case (c >= 40 && c <= 47) || (c >= 100 && c <= 107):
			t.bgColor = strconv.Itoa(c)
		}
		i++
	}
}

// resetSGR clears every attribute; an SGR reset leaves the hyperlink open.
func (t *ansiCodeTracker) resetSGR() {
	t.bold, t.dim, t.italic, t.underline = false, false, false, false
	t.blink, t.inverse, t.hidden, t.strike = false, false, false, false
	t.fgColor, t.bgColor = "", ""
}

// ActiveCodes returns the escape sequences that re-establish the active
// styling and hyperlink, or "" when nothing is active.
func (t *ansiCodeTracker) ActiveCodes() string {
	var codes []string
	for _, attr := range []struct {
		on   bool
		code string
	}{{t.bold, "1"}, {t.dim, "2"}, {t.italic, "3"}, {t.underline, "4"}, {t.blink, "5"}, {t.inverse, "7"}, {t.hidden, "8"}, {t.strike, "9"}} {
		if attr.on {
			codes = append(codes, attr.code)
		}
	}
	if t.fgColor != "" {
		codes = append(codes, t.fgColor)
	}
	if t.bgColor != "" {
		codes = append(codes, t.bgColor)
	}
	var b strings.Builder
	if len(codes) > 0 {
		b.WriteString("\x1b[")
		b.WriteString(strings.Join(codes, ";"))
		b.WriteString("m")
	}
	b.WriteString(t.activeHyperlinkOpen)
	return b.String()
}

// LineEndReset returns what a wrapped line must end with: underline off, so it
// does not bleed into padding, and the close of an open hyperlink, which
// ActiveCodes re-opens on the next line. It mirrors pi-tui's getLineEndReset.
func (t *ansiCodeTracker) LineEndReset() string {
	var b strings.Builder
	if t.underline {
		b.WriteString("\x1b[24m")
	}
	b.WriteString(t.activeHyperlinkClose)
	return b.String()
}

// UpdateFromText processes every escape sequence in text, in order.
func (t *ansiCodeTracker) UpdateFromText(text string) {
	for i := 0; i < len(text); {
		n := ExtractAnsi(text, i)
		if n == 0 {
			i++
			continue
		}
		t.Process(text[i : i+n])
		i += n
	}
}

// firstSGRParams returns the parameters of the first ESC [ digits-or-semicolons m
// inside code, which an extracted CSI may precede with other bytes.
func firstSGRParams(code string) (string, bool) {
	for i := 0; i+2 < len(code); i++ {
		if code[i] != 0x1b || code[i+1] != '[' {
			continue
		}
		j := i + 2
		for j < len(code) && (code[j] == ';' || (code[j] >= '0' && code[j] <= '9')) {
			j++
		}
		if j < len(code) && code[j] == 'm' {
			return code[i+2 : j], true
		}
	}
	return "", false
}
