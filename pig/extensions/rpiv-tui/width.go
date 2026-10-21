package tui

import (
	"strings"
	"sync"
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
// width of each. Clusters come from uniseg with UAX #29 rule GB9c (Indic
// conjuncts) applied on top, as ICU's segmenter does for Pi. A printable ASCII
// byte followed by another ASCII byte, or by the end of the string, is always
// a complete one-column cluster, so those bytes skip the Unicode segmentation
// entirely.
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
	g.cluster, g.remaining, _, g.state = uniseg.FirstGraphemeClusterInString(s, g.state)
	// GB9c: a consonant, linker chain joins the consonant that follows it,
	// however many clusters uniseg made of the conjunct.
	for endsWithConjunctLinker(g.cluster) && startsWithConjunctConsonant(g.remaining) {
		var next string
		next, g.remaining, _, g.state = uniseg.FirstGraphemeClusterInString(g.remaining, g.state)
		g.cluster = s[:len(g.cluster)+len(next)]
	}
	g.width = graphemeWidth(g.cluster)
	return true
}

// endsWithConjunctLinker reports whether cluster ends in
// Consonant (Extend | Linker)* Linker (Extend | Linker)*, the left side of
// rule GB9c.
func endsWithConjunctLinker(cluster string) bool {
	sawLinker := false
	for cluster != "" {
		r, n := utf8.DecodeLastRuneInString(cluster)
		cluster = cluster[:len(cluster)-n]
		switch {
		case inRanges(incbLinkerRanges[:], r):
			sawLinker = true
		case inRanges(incbExtendRanges[:], r):
		default:
			return sawLinker && inRanges(incbConsonantRanges[:], r)
		}
	}
	return false
}

// startsWithConjunctConsonant reports whether s begins with an
// Indic_Conjunct_Break=Consonant code point, the right side of rule GB9c.
func startsWithConjunctConsonant(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return s != "" && inRanges(incbConsonantRanges[:], r)
}

// Str returns the current cluster.
func (g *graphemeIter) Str() string { return g.cluster }

// Width returns the column width of the current cluster.
func (g *graphemeIter) Width() int { return g.width }

// graphemeWidth is pi-tui's graphemeWidth for one cluster. A tab is three
// columns and a printable ASCII byte one. A cluster made only of terminal
// spacing marks is one column per mark, and one made only of zero-width code
// points is zero. An RGI emoji (Pi's \p{RGI_Emoji}: an emoji code point on its
// own, or a keycap, emoji presentation, modifier, flag, tag or ZWJ sequence
// from the RGI set) is two columns whatever its parts would measure.
// Otherwise leading non-printing code points are skipped and the first
// printing code point sets the width: a regional indicator is two columns
// whatever follows it, and any other code point is its East Asian width. Each
// terminal spacing mark after it adds one, an ordinary mark adds nothing, a
// printing code point after a mark (or a halfwidth and fullwidth form) adds
// its own East Asian width, and a Thai or Lao vowel AM adds one.
func graphemeWidth(seg string) int {
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
	if couldBeEmoji(seg) && isRGIEmoji(seg) {
		return 2
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
	cp, n := utf8.DecodeRuneInString(base)
	if cp >= 0x1F1E6 && cp <= 0x1F1FF {
		// A regional indicator is two columns even when a spacing mark or a
		// vowel AM follows it in the same cluster.
		return 2
	}
	width := runeCells(cp)
	followsMark := false
	for _, c := range base[n:] {
		switch {
		case isTerminalSpacingMark(c):
			width++
			followsMark = false
		case unicode.Is(unicode.M, c):
			followsMark = true
		case !isNonPrinting(c):
			if followsMark || (c >= 0xFF00 && c <= 0xFFEF) {
				width += runeCells(c)
			} else if c == 0x0E33 || c == 0x0EB3 {
				width++
			}
			followsMark = false
		}
	}
	return width
}

// couldBeEmoji is pi-tui's prefilter for the RGI_Emoji test, so the set is
// consulted only for a cluster that starts in one of the emoji blocks (Emoji
// and Pictographs, Miscellaneous Technical, Miscellaneous Symbols and
// Dingbats, the stars and circles at U+2B50), contains U+FE0F, or is longer
// than two UTF-16 code units (a ZWJ, modifier or flag sequence).
func couldBeEmoji(seg string) bool {
	cp, _ := utf8.DecodeRuneInString(seg)
	if cp >= 0x1F000 && cp <= 0x1FBFF || cp >= 0x2300 && cp <= 0x23FF ||
		cp >= 0x2600 && cp <= 0x27BF || cp >= 0x2B50 && cp <= 0x2B55 {
		return true
	}
	units := 0
	for _, r := range seg {
		if r == 0xFE0F {
			return true
		}
		if r >= 0x10000 {
			units += 2
		} else {
			units++
		}
	}
	return units > 2
}

// isRGIEmoji is pi-tui's /^\p{RGI_Emoji}$/v test for one cluster: a single
// code point is looked up in rgiSingleRanges, a longer cluster must be one of
// rgiSequences exactly. A cluster with extra code points (an RGI sequence
// followed by a combining mark, say) is not an RGI emoji and keeps the width
// of its first printing code point.
func isRGIEmoji(seg string) bool {
	r, n := utf8.DecodeRuneInString(seg)
	if n == len(seg) {
		return inRanges(rgiSingleRanges[:], r)
	}
	_, ok := rgiSequenceSet()[seg]
	return ok
}

// rgiSequenceSet is rgiSequences as a set. Only a cluster of several code
// points consults it, so it is built on first use rather than at process
// start.
var rgiSequenceSet = sync.OnceValue(func() map[string]struct{} {
	set := make(map[string]struct{}, len(rgiSequences))
	for _, s := range &rgiSequences {
		set[s] = struct{}{}
	}
	return set
})

// runeCells is pi-tui's eastAsianWidth for one printing code point: two for
// East_Asian_Width Fullwidth and Wide (wideRanges, Unicode 17), one for every
// other code point, so the two- and three-em dashes, the line and paragraph
// separators and the halfwidth katakana voiced sound marks are one column and
// the emoji modifiers U+1F3FB to U+1F3FF are two.
func runeCells(r rune) int {
	if inRanges(wideRanges[:], r) {
		return 2
	}
	return 1
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
