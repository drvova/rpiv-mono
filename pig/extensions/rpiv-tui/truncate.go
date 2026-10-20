package tui

import "strings"

// TruncateToWidth truncates s to at most width columns, appending ellipsis
// when text was cut, and pads the result with spaces to exactly width columns
// when pad is true. It mirrors pi-tui's truncateToWidth: escape sequences
// before the cut are kept and a reset follows the kept text, so styling never
// leaks into the ellipsis or beyond; an OSC 8 hyperlink left open by the cut
// is closed; an ellipsis wider than width is clipped; and the kept text is a
// contiguous prefix, so a wide grapheme that does not fit ends the prefix
// rather than being skipped.
func TruncateToWidth(s string, width int, ellipsis string, pad bool) string {
	if width <= 0 {
		return ""
	}
	if s == "" {
		if pad {
			return strings.Repeat(" ", width)
		}
		return ""
	}

	ellipsisWidth := VisibleWidth(ellipsis)
	if ellipsisWidth >= width {
		textWidth := VisibleWidth(s)
		if textWidth <= width {
			if pad {
				return s + strings.Repeat(" ", width-textWidth)
			}
			return s
		}
		clipped := truncateFragmentToWidth(ellipsis, width)
		if clipped.width == 0 {
			if pad {
				return strings.Repeat(" ", width)
			}
			return ""
		}
		return finalizeTruncatedResult("", 0, clipped.text, clipped.width, width, pad)
	}

	if isPrintableASCII(s) {
		if len(s) <= width {
			if pad {
				return s + strings.Repeat(" ", width-len(s))
			}
			return s
		}
		targetWidth := width - ellipsisWidth
		return finalizeTruncatedResult(s[:targetWidth], targetWidth, ellipsis, ellipsisWidth, width, pad)
	}

	targetWidth := width - ellipsisWidth
	var result strings.Builder
	pendingAnsi := ""
	visibleSoFar := 0
	keptWidth := 0
	keepContiguous := true
	overflowed := false

	// keep consumes one visible unit of w columns whose text is seg, keeping
	// it while the kept prefix is still contiguous and fits, and reports
	// whether the whole text has overflowed width.
	keep := func(seg string, w int) bool {
		if keepContiguous && keptWidth+w <= targetWidth {
			result.WriteString(pendingAnsi)
			pendingAnsi = ""
			result.WriteString(seg)
			keptWidth += w
		} else {
			keepContiguous = false
			pendingAnsi = ""
		}
		visibleSoFar += w
		return visibleSoFar > width
	}

	hasAnsi := strings.ContainsRune(s, 0x1B)
	hasTabs := strings.ContainsRune(s, '\t')
	if !hasAnsi && !hasTabs {
		for g := newGraphemeIter(s); g.Next(); {
			if keep(g.Str(), g.Width()) {
				overflowed = true
				break
			}
		}
	} else {
		for i := 0; i < len(s) && !overflowed; {
			if n := ExtractAnsi(s, i); n > 0 {
				pendingAnsi += s[i : i+n]
				i += n
				continue
			}
			if s[i] == '\t' {
				overflowed = keep("\t", 3)
				i++
				continue
			}
			end := i
			for end < len(s) && s[end] != '\t' && ExtractAnsi(s, end) == 0 {
				end++
			}
			for g := newGraphemeIter(s[i:end]); g.Next(); {
				if keep(g.Str(), g.Width()) {
					overflowed = true
					break
				}
			}
			i = end
		}
	}

	if !overflowed {
		if pad {
			return s + strings.Repeat(" ", max(width-visibleSoFar, 0))
		}
		return s
	}
	return finalizeTruncatedResult(result.String(), keptWidth, ellipsis, ellipsisWidth, width, pad)
}

type fragment struct {
	text  string
	width int
}

// truncateFragmentToWidth keeps the longest prefix of text that fits in
// maxWidth columns, with its escape sequences; mirrors pi-tui's
// truncateFragmentToWidth.
func truncateFragmentToWidth(text string, maxWidth int) fragment {
	if maxWidth <= 0 || text == "" {
		return fragment{}
	}
	if isPrintableASCII(text) {
		if len(text) > maxWidth {
			return fragment{text: text[:maxWidth], width: maxWidth}
		}
		return fragment{text: text, width: len(text)}
	}
	var b strings.Builder
	width := 0
	pendingAnsi := ""
	// keep appends one visible unit when it fits and reports whether it did.
	keep := func(seg string, w int) bool {
		if width+w > maxWidth {
			return false
		}
		b.WriteString(pendingAnsi)
		pendingAnsi = ""
		b.WriteString(seg)
		width += w
		return true
	}
	for i := 0; i < len(text); {
		if n := ExtractAnsi(text, i); n > 0 {
			pendingAnsi += text[i : i+n]
			i += n
			continue
		}
		if text[i] == '\t' {
			if !keep("\t", 3) {
				break
			}
			i++
			continue
		}
		end := i
		for end < len(text) && text[end] != '\t' && ExtractAnsi(text, end) == 0 {
			end++
		}
		broken := false
		for g := newGraphemeIter(text[i:end]); g.Next(); {
			if !keep(g.Str(), g.Width()) {
				broken = true
				break
			}
		}
		if broken {
			break
		}
		i = end
	}
	return fragment{text: b.String(), width: width}
}

// finalizeTruncatedResult assembles the kept prefix, the close of any open
// hyperlink, a reset, the ellipsis bracketed by resets, and the padding.
func finalizeTruncatedResult(prefix string, prefixWidth int, ellipsis string, ellipsisWidth, maxWidth int, pad bool) string {
	const reset = "\x1b[0m"
	result := prefix + activeOsc8Close(prefix) + reset
	if ellipsis != "" {
		result += ellipsis + reset
	}
	if pad {
		result += strings.Repeat(" ", max(maxWidth-prefixWidth-ellipsisWidth, 0))
	}
	return result
}

// activeOsc8Close returns the close sequence, with the opener's terminator,
// of an OSC 8 hyperlink that prefix leaves open; "" otherwise. Mirrors pi-tui's
// getActiveOsc8Close, which ignores an OSC 8 without a params;url separator.
func activeOsc8Close(prefix string) string {
	if !strings.Contains(prefix, "\x1b]8;") {
		return ""
	}
	closing := ""
	for i := 0; i < len(prefix); {
		code, n := ExtractAnsiCode(prefix, i)
		if n == 0 {
			i++
			continue
		}
		i += n
		if !strings.HasPrefix(code, "\x1b]8;") {
			continue
		}
		terminator := "\x1b\\"
		if strings.HasSuffix(code, "\x07") {
			terminator = "\x07"
		}
		body := code[4 : len(code)-len(terminator)]
		if _, url, ok := strings.Cut(body, ";"); ok {
			if url == "" {
				closing = ""
			} else {
				closing = "\x1b]8;;" + terminator
			}
		}
	}
	return closing
}
