package tui

import (
	"testing"
	"unicode/utf8"
)

// Every range table is a sorted list of disjoint inclusive pairs, which
// inRanges relies on.
func TestRangeTablesAreSortedPairs(t *testing.T) {
	tables := map[string][]rune{
		"terminalSpacingMarkRanges": terminalSpacingMarkRanges[:],
		"cjkBreakRanges":            cjkBreakRanges[:],
		"incbConsonantRanges":       incbConsonantRanges[:],
		"incbLinkerRanges":          incbLinkerRanges[:],
		"incbExtendRanges":          incbExtendRanges[:],
		"wideRanges":                wideRanges[:],
		"rgiSingleRanges":           rgiSingleRanges[:],
		"defaultIgnorableRanges":    defaultIgnorableRanges[:],
	}
	for name, table := range tables {
		if len(table)%2 != 0 {
			t.Errorf("%s has %d entries, want an even count", name, len(table))
			continue
		}
		for i := 0; i < len(table); i += 2 {
			lo, hi := table[i], table[i+1]
			if lo > hi {
				t.Errorf("%s pair %d: lo %U > hi %U", name, i/2, lo, hi)
			}
			if i > 0 && lo <= table[i-1] {
				t.Errorf("%s pair %d: lo %U not after previous hi %U", name, i/2, lo, table[i-1])
			}
		}
	}
}

// The RGI sequence list holds distinct strings of two or more code points,
// every single code point of the RGI set is East Asian wide, and both halves
// of the set agree with the sequences pi-tui's tests rely on.
func TestRGITablesAreConsistent(t *testing.T) {
	seen := make(map[string]bool, len(rgiSequences))
	for _, s := range rgiSequences {
		if seen[s] {
			t.Errorf("duplicate RGI sequence %q", s)
		}
		seen[s] = true
		if utf8.RuneCountInString(s) < 2 {
			t.Errorf("RGI sequence %q has fewer than two code points", s)
		}
		if !utf8.ValidString(s) {
			t.Errorf("RGI sequence %q is not valid UTF-8", s)
		}
	}
	if len(rgiSequenceSet()) != len(rgiSequences) {
		t.Errorf("rgiSequenceSet has %d members, want %d", len(rgiSequenceSet()), len(rgiSequences))
	}
	for i := 0; i < len(rgiSingleRanges); i += 2 {
		for r := rgiSingleRanges[i]; r <= rgiSingleRanges[i+1]; r++ {
			if runeCells(r) != 2 {
				t.Errorf("RGI code point %U is not East Asian wide", r)
			}
		}
	}
	for _, s := range []string{"#️⃣", "9️⃣", "⚠️", "ℹ️", "❤️", "☝🏻", "👍🏽", "🇺🇸", "👩‍💻", "❤️‍🔥", "🏳️‍🌈", "🏴‍☠️", "🏴󠁧󠁢󠁥󠁮󠁧󠁿"} {
		if !seen[s] {
			t.Errorf("RGI sequence %q (%U) missing from rgiSequences", s, []rune(s))
		}
	}
	for _, s := range []string{"★️", "♡️", "\U0001F000️", "a️", "🧑‍❤️‍🧑", "\U0001F3FD\U0001F3FD"} {
		if seen[s] {
			t.Errorf("%q (%U) is not an RGI emoji but is in rgiSequences", s, []rune(s))
		}
	}
}
