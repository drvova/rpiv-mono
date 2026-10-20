package tui

import (
	"strings"
	"unicode/utf8"
)

// IsJSSpace reports whether r is in JavaScript's \s class, the set
// String.prototype.trim removes: the ASCII whitespace, NBSP, the Ogham space
// mark, the U+2000 to U+200A spaces, the line and paragraph separators, the
// narrow and medium mathematical spaces, the ideographic space and the BOM.
func IsJSSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x00A0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	return r >= 0x2000 && r <= 0x200A
}

// JSTrim is String.prototype.trim.
func JSTrim(s string) string { return JSTrimStart(JSTrimEnd(s)) }

// JSTrimStart is String.prototype.trimStart.
func JSTrimStart(s string) string {
	for s != "" {
		r, n := utf8.DecodeRuneInString(s)
		if !IsJSSpace(r) {
			break
		}
		s = s[n:]
	}
	return s
}

// JSTrimEnd is String.prototype.trimEnd.
func JSTrimEnd(s string) string {
	for s != "" {
		r, n := utf8.DecodeLastRuneInString(s)
		if !IsJSSpace(r) {
			break
		}
		s = s[:len(s)-n]
	}
	return s
}

// LenUTF16 is JavaScript's s.length: the number of UTF-16 code units, two for
// each code point outside the Basic Multilingual Plane. Each invalid UTF-8
// byte counts one unit, as the replacement character it decodes to.
func LenUTF16(s string) int {
	n := 0
	for _, r := range s {
		n += utf16Units(r)
	}
	return n
}

// SliceUTF16 is JavaScript's s.slice(start, end) with indexes in UTF-16 code
// units: a negative index counts from the end, indexes are clamped to the
// string, and an empty or inverted range yields "". JavaScript can cut a
// surrogate pair in half; here a cut inside an astral code point rounds
// inward, so the result is always valid UTF-8 and never wider than the
// requested range.
func SliceUTF16(s string, start, end int) string {
	length := LenUTF16(s)
	start = clampSliceIndex(start, length)
	end = clampSliceIndex(end, length)
	if start >= end {
		return ""
	}
	byteStart, byteEnd := -1, len(s)
	unit := 0
	for i, r := range s {
		n := utf16Units(r)
		if byteStart < 0 && unit >= start {
			byteStart = i
		}
		if unit+n > end {
			byteEnd = i
			break
		}
		unit += n
	}
	if byteStart < 0 || byteStart >= byteEnd {
		return ""
	}
	return s[byteStart:byteEnd]
}

func clampSliceIndex(index, length int) int {
	if index < 0 {
		return max(length+index, 0)
	}
	return min(index, length)
}

func utf16Units(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}

// Pluralize returns singular when count is exactly one and plural otherwise,
// the rule the TypeScript extensions apply with count === 1 ? "" : "s".
func Pluralize(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

// PluralSuffix returns "" when count is exactly one and "s" otherwise, for
// messages built as fmt.Sprintf("%d item%s", n, PluralSuffix(n)).
func PluralSuffix(count int) string {
	return Pluralize(count, "", "s")
}

// Repeat is strings.Repeat guarded against a negative count, which the
// TypeScript " ".repeat(n) callers never pass but width arithmetic can produce.
func Repeat(s string, count int) string {
	return strings.Repeat(s, max(count, 0))
}
