// Package tui replaces the pi-tui helpers the rpiv extensions import: ANSI-aware
// text measurement, truncation and wrapping, Pi's key-identifier grammar and
// terminal key matching, the Styler interface the siblings render with, and a
// few JavaScript-compatible string helpers.
//
// The width functions mirror pi-tui's utils.ts (visibleWidth, truncateToWidth,
// wrapTextWithAnsi): a tab is three columns, SGR, OSC and APC escape sequences
// are zero columns, and text is measured per extended grapheme cluster. East
// Asian wide characters and emoji presentation clusters are two columns,
// combining marks, zero width joiners and other non-printing code points are
// zero, and the spacing marks terminals allocate a cell for (pi-tui's
// terminalSpacingMarkRegex) count one column each. Segmentation and East Asian
// width come from github.com/rivo/uniseg.
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
// Grapheme segmentation follows uniseg's Unicode tables without the GB9c Indic
// conjunct rule ICU applies in Pi, so a conjunct such as "र्क" is two clusters
// here and one in Pi; its width (2) agrees. Pi's RGI_Emoji test is approximated
// by uniseg's emoji presentation rules plus the keycap sequences, and a lone
// UTF-16 surrogate, which Pi measures as zero width, cannot occur in a Go
// string.
package tui
