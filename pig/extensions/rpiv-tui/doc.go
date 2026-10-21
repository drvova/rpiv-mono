// Package tui replaces the pi-tui helpers the rpiv extensions import: ANSI-aware
// text measurement, truncation and wrapping, Pi's key-identifier grammar and
// terminal key matching, the Styler interface the siblings render with, and a
// few JavaScript-compatible string helpers.
//
// The width functions mirror pi-tui's utils.ts (visibleWidth, truncateToWidth,
// wrapTextWithAnsi): a tab is three columns, SGR, OSC and APC escape sequences
// are zero columns, and text is measured per extended grapheme cluster. An RGI
// emoji (a code point or sequence matching Pi's \p{RGI_Emoji}) and an East
// Asian wide character are two columns, combining marks, zero width joiners
// and other non-printing code points are zero, and the spacing marks terminals
// allocate a cell for (pi-tui's terminalSpacingMarkRegex) count one column
// each. Segmentation comes from github.com/rivo/uniseg with UAX #29 rule GB9c
// (Indic conjuncts) applied on top so clusters match ICU's, which Pi uses;
// East Asian width, the terminal spacing marks and the RGI emoji set are the
// Unicode 17 tables pi-tui evaluates at runtime, copied from PiG's tui/widthx
// (unicode_ranges.go, rgi_sequences.go).
//
// The key functions mirror pi-tui's keys.ts as PiG ports it: ParseKey names a
// raw terminal input, MatchesKey compares one with a key identifier such as
// "ctrl+shift+t" across legacy sequences, xterm modifyOtherKeys and the Kitty
// keyboard protocol, IsKeyRelease and IsKeyRepeat classify Kitty event types,
// and ValidKeyID is the collapse-key validator rpiv-todo and
// rpiv-ask-user-question share.
//
// Styler is the structural subset of sdk.UITheme (Fg, Bg, Bold, Italic,
// Underline, Strikethrough, Inverse) that rendering code depends on, so tests
// can render with PlainStyler or TagStyler instead of a host theme.
//
// # Divergences
//
// pi-tui's matchesKey and parseKey consult the terminal's Kitty keyboard
// protocol negotiation. A PiG extension runs in a subprocess and never sees
// that negotiation, so this package keeps its own flag, off by default (legacy
// semantics); a caller that learns the mode sets it with SetKittyProtocolActive.
//
// Pi classifies code points with Unicode 17 tables. East Asian width and the
// RGI emoji set are Unicode 17 here as well, so every RGI emoji sequence (a
// modifier sequence on a text-presentation base, a ZWJ sequence carrying
// U+FE0F) is two columns exactly as in Pi, and everything else is measured by
// its first printing code point plus the marks and forms that follow it. The
// Mark, Control, Format and Default_Ignorable classes come from Go's unicode
// package and grapheme segmentation from uniseg, both at Unicode 15, so a
// combining mark or a cluster-initial letter assigned or reclassified after
// Unicode 15 is classified by the older tables here and by Unicode 17 in Pi. A
// lone UTF-16 surrogate, which Pi measures as zero width, cannot occur in a Go
// string.
package tui
