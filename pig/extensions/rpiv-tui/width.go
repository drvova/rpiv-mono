package tui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// VisibleWidth returns the number of terminal columns s occupies, mirroring
// pi-tui's visibleWidth: a tab is three columns, the escape sequences
// ExtractAnsiCode recognizes are zero, and every other grapheme cluster is
// measured by the rules in the package documentation.
func VisibleWidth(s string) int {
	if s == "" {
		return 0
	}
	if isPrintableASCII(s) {
		return len(s)
	}
	clean := s
	if strings.ContainsRune(clean, '\t') {
		clean = strings.ReplaceAll(clean, "\t", "   ")
	}
	if strings.ContainsRune(clean, 0x1B) {
		clean = StripAnsi(clean)
	}
	if isPrintableASCII(clean) {
		return len(clean)
	}
	width := 0
	for g := newGraphemeIter(clean); g.Next(); {
		width += g.Width()
	}
	return width
}

// PadToWidth appends spaces to s until it occupies width columns. Text already
// at least that wide is returned unchanged; it is never truncated.
func PadToWidth(s string, width int) string {
	pad := max(0, width-VisibleWidth(s))
	return s + strings.Repeat(" ", pad)
}

// isPrintableASCII reports whether every byte of s is a printable ASCII
// character, each of which is one column.
func isPrintableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 || c > 0x7E {
			return false
		}
	}
	return true
}

// graphemeIter walks the extended grapheme clusters of a string, reporting the
// width of each. A printable ASCII byte followed by another ASCII byte, or by
// the end of the string, is always a complete one-column cluster, so those
// bytes skip the Unicode segmentation entirely.
type graphemeIter struct {
	remaining string
	cluster   string
	width     int
	state     int
}

func newGraphemeIter(s string) graphemeIter {
	return graphemeIter{remaining: s, state: -1}
}

// Next advances to the next cluster and reports whether there was one.
func (g *graphemeIter) Next() bool {
	s := g.remaining
	if s == "" {
		g.cluster, g.width = "", 0
		return false
	}
	if c := s[0]; c >= 0x20 && c <= 0x7E && (len(s) == 1 || s[1] < utf8.RuneSelf) {
		g.cluster, g.remaining, g.width, g.state = s[:1], s[1:], 1, -1
		return true
	}
	var clusterWidth int
	g.cluster, g.remaining, clusterWidth, g.state = uniseg.FirstGraphemeClusterInString(s, g.state)
	g.width = graphemeWidth(g.cluster, clusterWidth)
	return true
}

// Str returns the current cluster.
func (g *graphemeIter) Str() string { return g.cluster }

// Width returns the column width of the current cluster.
func (g *graphemeIter) Width() int { return g.width }

// graphemeWidth is pi-tui's graphemeWidth for one cluster, given the width
// uniseg computed for it. uniseg already gives East Asian wide characters,
// regional indicators and emoji presentation sequences two columns and
// combining marks, controls and joiners zero; the remaining rules are Pi's: a
// tab is three columns, the two- and three-em dashes are one, a keycap
// sequence is an emoji, leading non-printing code points do not occupy a cell,
// and a terminal spacing mark that Unicode makes zero width still counts one.
func graphemeWidth(seg string, clusterWidth int) int {
	switch {
	case seg == "":
		return 0
	case seg == "\t":
		return 3
	case len(seg) == 1 && seg[0] < utf8.RuneSelf:
		if seg[0] >= 0x20 && seg[0] <= 0x7E {
			return 1
		}
		return 0
	case seg == "⸺" || seg == "⸻":
		return 1
	case isKeycapSequence(seg):
		return 2
	}
	allSpacingMarks, allZeroWidth, count := true, true, 0
	for _, r := range seg {
		if !isTerminalSpacingMark(r) {
			allSpacingMarks = false
		}
		if !isZeroWidth(r) {
			allZeroWidth = false
		}
		count++
	}
	if allSpacingMarks {
		return count
	}
	if allZeroWidth {
		return 0
	}
	base := seg
	for base != "" {
		r, n := utf8.DecodeRuneInString(base)
		if !isNonPrinting(r) {
			break
		}
		base = base[n:]
	}
	if base == "" {
		return 0
	}
	width := clusterWidth
	if len(base) != len(seg) {
		width = uniseg.StringWidth(base)
	}
	_, n := utf8.DecodeRuneInString(base)
	for _, r := range base[n:] {
		if isTerminalSpacingMark(r) && runeCells(r) == 0 {
			width++
		}
	}
	return width
}

// isKeycapSequence reports an RGI keycap emoji: a digit, # or * followed by
// U+FE0F and U+20E3.
func isKeycapSequence(seg string) bool {
	if len(seg) != 7 || seg[1:] != "️⃣" {
		return false
	}
	c := seg[0]
	return c >= '0' && c <= '9' || c == '#' || c == '*'
}

// runeCells is the width uniseg gives a single code point.
func runeCells(r rune) int {
	return uniseg.StringWidth(string(r))
}

// isTerminalSpacingMark reports a member of pi-tui's terminalSpacingMarkRegex.
func isTerminalSpacingMark(r rune) bool {
	return inRanges(terminalSpacingMarkRanges[:], r)
}

// isCJKBreak reports whether s contains a code point after which a line may
// wrap without a space (pi-tui's cjkBreakRegex).
func isCJKBreak(s string) bool {
	for _, r := range s {
		if inRanges(cjkBreakRanges[:], r) {
			return true
		}
	}
	return false
}

// isZeroWidth is pi-tui's zero-width class: Default_Ignorable_Code_Point,
// Control and Mark.
func isZeroWidth(r rune) bool {
	return isDefaultIgnorable(r) || unicode.Is(unicode.Cc, r) || unicode.Is(unicode.M, r)
}

// isNonPrinting is pi-tui's non-printing class: the zero-width class plus
// Format.
func isNonPrinting(r rune) bool {
	return isZeroWidth(r) || unicode.Is(unicode.Cf, r)
}

// defaultIgnorableRanges is Unicode's Default_Ignorable_Code_Point property as
// inclusive pairs.
var defaultIgnorableRanges = [...]rune{
	0x00AD, 0x00AD, 0x034F, 0x034F, 0x061C, 0x061C, 0x115F, 0x1160,
	0x17B4, 0x17B5, 0x180B, 0x180F, 0x200B, 0x200F, 0x202A, 0x202E,
	0x2060, 0x206F, 0x3164, 0x3164, 0xFE00, 0xFE0F, 0xFEFF, 0xFEFF,
	0xFFA0, 0xFFA0, 0xFFF0, 0xFFF8, 0x1BCA0, 0x1BCA3, 0x1D173, 0x1D17A,
	0xE0000, 0xE0FFF,
}

func isDefaultIgnorable(r rune) bool {
	return inRanges(defaultIgnorableRanges[:], r)
}
