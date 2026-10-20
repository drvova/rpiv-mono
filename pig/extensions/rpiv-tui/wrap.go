package tui

import "strings"

// WrapTextWithAnsi word-wraps text to width columns per line, mirroring
// pi-tui's wrapTextWithAnsi. Existing line breaks (LF, CRLF or CR) are kept,
// words longer than width are broken between graphemes, a line may break after
// a CJK character without a space, trailing whitespace is trimmed from wrapped
// lines, and active SGR styling and OSC 8 hyperlinks are closed at each break
// and re-opened on the next line. The result has at least one line.
func WrapTextWithAnsi(text string, width int) []string {
	if text == "" {
		return []string{""}
	}
	src := text
	if strings.ContainsRune(src, '\r') {
		src = strings.ReplaceAll(src, "\r\n", "\n")
		src = strings.ReplaceAll(src, "\r", "\n")
	}
	var result []string
	tracker := &ansiCodeTracker{}
	for _, line := range strings.Split(src, "\n") {
		prefix := ""
		if len(result) > 0 {
			prefix = tracker.ActiveCodes()
		}
		result = append(result, wrapSingleLine(prefix+line, width)...)
		tracker.UpdateFromText(line)
	}
	if len(result) == 0 {
		return []string{""}
	}
	return result
}

func wrapSingleLine(line string, width int) []string {
	if line == "" {
		return []string{""}
	}
	if VisibleWidth(line) <= width {
		return []string{line}
	}

	var wrapped []string
	tracker := &ansiCodeTracker{}
	var current strings.Builder
	currentVisible := 0

	for _, token := range splitIntoTokensWithAnsi(line) {
		tokenWidth := VisibleWidth(token)
		isWhitespace := JSTrim(token) == ""

		if tokenWidth > width && !isWhitespace {
			// The token alone is wider than a line: break it between graphemes.
			if current.Len() > 0 {
				wrapped = append(wrapped, current.String()+tracker.LineEndReset())
			}
			broken := breakLongWord(token, width, tracker)
			wrapped = append(wrapped, broken[:len(broken)-1]...)
			last := broken[len(broken)-1]
			current.Reset()
			current.WriteString(last)
			currentVisible = VisibleWidth(last)
			continue
		}

		if currentVisible+tokenWidth > width && currentVisible > 0 {
			wrapped = append(wrapped, JSTrimEnd(current.String())+tracker.LineEndReset())
			current.Reset()
			current.WriteString(tracker.ActiveCodes())
			if isWhitespace {
				// A new line never starts with whitespace.
				currentVisible = 0
			} else {
				current.WriteString(token)
				currentVisible = tokenWidth
			}
		} else {
			current.WriteString(token)
			currentVisible += tokenWidth
		}
		tracker.UpdateFromText(token)
	}

	if current.Len() > 0 {
		wrapped = append(wrapped, current.String())
	}
	if len(wrapped) == 0 {
		return []string{""}
	}
	for i := range wrapped {
		wrapped[i] = JSTrimEnd(wrapped[i])
	}
	return wrapped
}

// splitIntoTokensWithAnsi splits a line into runs of spaces and runs of other
// text, each CJK grapheme being its own token, with every escape sequence
// attached to the token that follows it (or to the last token at the end).
func splitIntoTokensWithAnsi(text string) []string {
	var tokens []string
	var current strings.Builder
	pendingAnsi := ""
	const (
		kindNone = iota
		kindSpace
		kindWord
	)
	currentKind := kindNone
	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, current.String())
		current.Reset()
		currentKind = kindNone
	}

	for i := 0; i < len(text); {
		if n := ExtractAnsi(text, i); n > 0 {
			pendingAnsi += text[i : i+n]
			i += n
			continue
		}
		end := i
		for end < len(text) && ExtractAnsi(text, end) == 0 {
			end++
		}
		for g := newGraphemeIter(text[i:end]); g.Next(); {
			segment := g.Str()
			isSpace := segment == " "
			if !isSpace && isCJKBreak(segment) {
				flush()
				tokens = append(tokens, pendingAnsi+segment)
				pendingAnsi = ""
				continue
			}
			kind := kindWord
			if isSpace {
				kind = kindSpace
			}
			if current.Len() > 0 && currentKind != kind {
				flush()
			}
			current.WriteString(pendingAnsi)
			pendingAnsi = ""
			currentKind = kind
			current.WriteString(segment)
		}
		i = end
	}
	if pendingAnsi != "" {
		if current.Len() == 0 && len(tokens) > 0 {
			tokens[len(tokens)-1] += pendingAnsi
		} else {
			current.WriteString(pendingAnsi)
		}
	}
	flush()
	return tokens
}

// breakLongWord splits a token wider than width into lines of at most width
// columns, breaking between graphemes and carrying the tracker's styling
// across the breaks.
func breakLongWord(word string, width int, tracker *ansiCodeTracker) []string {
	var lines []string
	currentLine := tracker.ActiveCodes()
	currentWidth := 0

	for i := 0; i < len(word); {
		if n := ExtractAnsi(word, i); n > 0 {
			code := word[i : i+n]
			currentLine += code
			tracker.Process(code)
			i += n
			continue
		}
		end := i
		for end < len(word) && ExtractAnsi(word, end) == 0 {
			end++
		}
		for g := newGraphemeIter(word[i:end]); g.Next(); {
			seg := g.Str()
			segWidth := VisibleWidth(seg)
			if currentWidth+segWidth > width {
				lines = append(lines, currentLine+tracker.LineEndReset())
				currentLine = tracker.ActiveCodes()
				currentWidth = 0
			}
			currentLine += seg
			currentWidth += segWidth
		}
		i = end
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}
