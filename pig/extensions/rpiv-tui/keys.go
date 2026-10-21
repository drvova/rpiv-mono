package tui

import (
	"math"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
)

// Modifier bits of the Kitty keyboard protocol. Wire values are the bitmask
// plus one; the lock bits are ignored when keys are compared.
const (
	modShift = 1
	modAlt   = 2
	modCtrl  = 4
	modSuper = 8
	lockMask = 64 + 128 // Caps Lock and Num Lock
)

// Sentinel codepoints for keys without a Unicode codepoint (negative), and the
// codepoints of the named keys that have one.
const (
	cpArrowUp    = -1
	cpArrowDown  = -2
	cpArrowRight = -3
	cpArrowLeft  = -4
	cpDelete     = -10
	cpInsert     = -11
	cpPageUp     = -12
	cpPageDown   = -13
	cpHome       = -14
	cpEnd        = -15

	cpTab       = 9
	cpEnter     = 13
	cpEscape    = 27
	cpSpace     = 32
	cpBackspace = 127
	cpKPEnter   = 57414 // the Kitty keypad Enter
)

var kittyProtocolActive atomic.Bool

// SetKittyProtocolActive records whether the terminal has the Kitty keyboard
// protocol enabled, which decides how ambiguous legacy bytes are read: in
// Kitty mode a raw LF is shift+enter rather than enter, and ESC followed by a
// character is no longer alt+character. The host negotiates the protocol and
// an extension subprocess is not told the outcome, so the flag is off (legacy
// semantics) unless a caller sets it.
func SetKittyProtocolActive(active bool) {
	kittyProtocolActive.Store(active)
}

// IsKittyProtocolActive reports the flag SetKittyProtocolActive sets.
func IsKittyProtocolActive() bool {
	return kittyProtocolActive.Load()
}

// IsKeyRelease reports a Kitty key-release event (event type 3). Bracketed
// paste content is never a release, whatever byte patterns it contains.
func IsKeyRelease(data string) bool {
	return hasKittyEventType(data, '3')
}

// IsKeyRepeat reports a Kitty key-repeat event (event type 2). Bracketed paste
// content is never a repeat.
func IsKeyRepeat(data string) bool {
	return hasKittyEventType(data, '2')
}

func hasKittyEventType(data string, event byte) bool {
	if strings.Contains(data, "\x1b[200~") {
		return false
	}
	for _, final := range []byte{'u', '~', 'A', 'B', 'C', 'D', 'H', 'F'} {
		if strings.Contains(data, string([]byte{':', event, final})) {
			return true
		}
	}
	return false
}

// parsedKey is one terminal input decoded to a codepoint and modifier bitmask.
type parsedKey struct {
	codepoint int
	modifier  int
	// baseLayoutKey is the Kitty flag-4 alternate naming the key's position
	// on a standard layout (\x1b[1089::99;5u is Ctrl+С on a Cyrillic layout,
	// base key c); shiftedKey is the alternate naming the key with Shift held.
	baseLayoutKey    int
	hasBaseLayoutKey bool
	shiftedKey       int
	hasShiftedKey    bool
	// inexactModifier marks a modifyOtherKeys modifier too large for its
	// int32 view to equal the number JavaScript compares.
	inexactModifier bool
}

var kittyCSIURegex = regexp.MustCompile(`^\x1b\[(\d+)(?::(\d*))?(?::(\d+))?(?:;(\d+))?(?::(\d+))?u$`)

// parseCSIu parses a Kitty CSI u sequence,
// ESC [ codepoint [: shifted [: base]] [; modifier [: event]] u, or returns nil.
func parseCSIu(data string) *parsedKey {
	if len(data) < 4 || data[0] != 0x1b || data[1] != '[' || data[len(data)-1] != 'u' {
		return nil
	}
	match := kittyCSIURegex.FindStringSubmatch(data)
	if match == nil {
		return nil
	}
	modValue := 1.0
	if match[4] != "" {
		modValue = keyNumber(match[4])
	}
	pk := &parsedKey{codepoint: keyCodepoint(keyNumber(match[1])), modifier: jsInt32(modValue-1) &^ lockMask}
	if match[2] != "" {
		pk.shiftedKey, pk.hasShiftedKey = keyCodepoint(keyNumber(match[2])), true
	}
	if match[3] != "" {
		pk.baseLayoutKey, pk.hasBaseLayoutKey = keyCodepoint(keyNumber(match[3])), true
	}
	return pk
}

// keyNumber is parseInt(digits, 10) for a run of ASCII digits, as a float64 so
// a run too long for an int still parses.
func keyNumber(digits string) float64 {
	value, err := strconv.ParseFloat(digits, 64)
	if err != nil {
		if numErr, ok := err.(*strconv.NumError); !ok || numErr.Err != strconv.ErrRange {
			return math.NaN()
		}
	}
	return value
}

// jsInt32 is JavaScript's ToInt32, which its bitwise operators apply to a
// parsed modifier.
func jsInt32(value float64) int {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	wrapped := math.Mod(math.Trunc(value), 1<<32)
	if wrapped < 0 {
		wrapped += 1 << 32
	}
	return int(int32(uint32(wrapped)))
}

// keyCodepoint holds a parsed codepoint as an int that compares and truncates
// (String.fromCharCode) as the JavaScript number does; every number from 2^62
// up names no key, so one sentinel stands for all of them.
func keyCodepoint(value float64) int {
	if value < 1<<62 {
		return int(value)
	}
	return 1 << 62
}

// kittyFunctionalKeyEquivalents maps Kitty keypad codepoints to the keys they
// stand for.
var kittyFunctionalKeyEquivalents = map[int]int{
	57399: '0', 57400: '1', 57401: '2', 57402: '3', 57403: '4',
	57404: '5', 57405: '6', 57406: '7', 57407: '8', 57408: '9',
	57409: '.', 57410: '/', 57411: '*', 57412: '-', 57413: '+',
	57415: '=', 57416: ',',
	57417: cpArrowLeft, 57418: cpArrowRight, 57419: cpArrowUp, 57420: cpArrowDown,
	57421: cpPageUp, 57422: cpPageDown, 57423: cpHome, 57424: cpEnd,
	57425: cpInsert, 57426: cpDelete,
}

func normalizeKittyFunctionalCodepoint(codepoint int) int {
	if mapped, ok := kittyFunctionalKeyEquivalents[codepoint]; ok {
		return mapped
	}
	return codepoint
}

// parseModifyOtherKeys parses xterm modifyOtherKeys, ESC [ 27 ; modifier ; codepoint ~.
// The modifier keeps its lock bits because Pi compares it exactly.
func parseModifyOtherKeys(data string) *parsedKey {
	rest, ok := strings.CutPrefix(data, "\x1b[27;")
	if !ok {
		return nil
	}
	modText, rest := cutDigits(rest)
	if modText == "" || !strings.HasPrefix(rest, ";") {
		return nil
	}
	cpText, rest := cutDigits(rest[1:])
	if cpText == "" || rest != "~" {
		return nil
	}
	modifier := keyNumber(modText) - 1
	pk := &parsedKey{codepoint: keyCodepoint(keyNumber(cpText)), modifier: jsInt32(modifier)}
	pk.inexactModifier = float64(pk.modifier) != modifier
	return pk
}

// cutDigits splits s after its leading run of ASCII digits.
func cutDigits(s string) (digits, rest string) {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	return s[:end], s[end:]
}

// cutEventType removes an optional ":digits" Kitty event type; a ':' without
// digits is malformed.
func cutEventType(s string) (string, bool) {
	if !strings.HasPrefix(s, ":") {
		return s, true
	}
	event, rest := cutDigits(s[1:])
	return rest, event != ""
}

var arrowFinals = map[byte]int{'A': cpArrowUp, 'B': cpArrowDown, 'C': cpArrowRight, 'D': cpArrowLeft, 'H': cpHome, 'F': cpEnd}

var funcKeyCodes = map[int]int{2: cpInsert, 3: cpDelete, 5: cpPageUp, 6: cpPageDown, 7: cpHome, 8: cpEnd}

// parseModifiedArrow parses a modified arrow, Home or End,
// ESC [ 1 ; modifier [: event] final.
func parseModifiedArrow(data string) *parsedKey {
	rest, ok := strings.CutPrefix(data, "\x1b[1;")
	if !ok {
		return nil
	}
	modText, rest := cutDigits(rest)
	if modText == "" {
		return nil
	}
	if rest, ok = cutEventType(rest); !ok || len(rest) != 1 {
		return nil
	}
	cp, ok := arrowFinals[rest[0]]
	if !ok {
		return nil
	}
	return &parsedKey{codepoint: cp, modifier: jsInt32(keyNumber(modText)-1) &^ lockMask}
}

// parseModifiedFunc parses a functional key, ESC [ number [; modifier] [: event] ~.
func parseModifiedFunc(data string) *parsedKey {
	rest, ok := strings.CutPrefix(data, "\x1b[")
	if !ok {
		return nil
	}
	numText, rest := cutDigits(rest)
	if numText == "" {
		return nil
	}
	modValue := 1.0
	if strings.HasPrefix(rest, ";") {
		var modText string
		if modText, rest = cutDigits(rest[1:]); modText == "" {
			return nil
		}
		modValue = keyNumber(modText)
	}
	if rest, ok = cutEventType(rest); !ok || rest != "~" {
		return nil
	}
	num := keyNumber(numText)
	if num > 8 {
		return nil
	}
	cp, ok := funcKeyCodes[int(num)]
	if !ok {
		return nil
	}
	return &parsedKey{codepoint: cp, modifier: jsInt32(modValue-1) &^ lockMask}
}

// parseTerminalInput decodes the non-Kitty CSI forms.
func parseTerminalInput(data string) *parsedKey {
	if pk := parseModifyOtherKeys(data); pk != nil {
		if pk.modifier&modShift != 0 && pk.codepoint >= 'A' && pk.codepoint <= 'Z' {
			pk.codepoint += 32
		}
		return pk
	}
	if pk := parseModifiedArrow(data); pk != nil {
		return pk
	}
	return parseModifiedFunc(data)
}

// keyNameCodepoints maps lowercase base key names to codepoints; isArrow marks
// the keys that modified legacy input reports as ESC [ 1 ; modifier final.
var keyNameCodepoints = map[string]struct {
	cp      int
	isArrow bool
}{
	"up": {cpArrowUp, true}, "down": {cpArrowDown, true}, "left": {cpArrowLeft, true}, "right": {cpArrowRight, true},
	"delete": {cpDelete, false}, "insert": {cpInsert, false}, "pageup": {cpPageUp, false}, "pagedown": {cpPageDown, false},
	"home": {cpHome, false}, "end": {cpEnd, false},
	"escape": {cpEscape, false}, "esc": {cpEscape, false}, "tab": {cpTab, false}, "enter": {cpEnter, false},
	"return": {cpEnter, false}, "space": {cpSpace, false}, "backspace": {cpBackspace, false},
}

var arrowCPtoFinal = map[int]byte{cpArrowUp: 'A', cpArrowDown: 'B', cpArrowRight: 'C', cpArrowLeft: 'D', cpHome: 'H', cpEnd: 'F'}

var funcCPtoNum = map[int]int{cpDelete: 3, cpInsert: 2, cpPageUp: 5, cpPageDown: 6, cpHome: 7, cpEnd: 8}

// parseKeyID splits a key identifier such as "ctrl+shift+left" into its
// lowercase base key and modifier bitmask, as pi-tui's parseKeyId does: the
// identifier is lowercased, the leading "shift+", "alt+", "ctrl+" and "super+"
// parts are modifiers, and the rest is the base key, which may be "+" but
// holds no other "+". A repeated modifier, an unknown part such as "meta+" or
// an empty base key parses to nothing.
func parseKeyID(keyID string) (baseKey string, modifier int, ok bool) {
	modifierNames := [...]struct {
		prefix string
		bit    int
	}{{"shift+", modShift}, {"alt+", modAlt}, {"ctrl+", modCtrl}, {"super+", modSuper}}
	baseKey = jsToLower(keyID)
	for found := true; found; {
		found = false
		for _, name := range modifierNames {
			if rest, has := strings.CutPrefix(baseKey, name.prefix); has {
				if modifier&name.bit != 0 {
					return "", 0, false
				}
				modifier |= name.bit
				baseKey, found = rest, true
				break
			}
		}
	}
	if baseKey == "" || strings.Contains(baseKey, "+") && baseKey != "+" {
		return "", 0, false
	}
	return baseKey, modifier, true
}

// jsToLower is String.prototype.toLowerCase: the full Unicode lowercase
// mapping, which differs from the simple one strings.ToLower applies at a
// single code point, U+0130 (İ), lowercased to "i" followed by U+0307 rather
// than a plain "i". The Final_Sigma context is not applied; it cannot change
// whether an identifier names a key.
func jsToLower(s string) string {
	if strings.ContainsRune(s, '\u0130') {
		s = strings.ReplaceAll(s, "\u0130", "i\u0307")
	}
	return strings.ToLower(s)
}

// resolveKeyCodepoint converts a base key name to the codepoint the dynamic
// matcher compares; function keys and clear have none.
func resolveKeyCodepoint(baseKey string) (cp int, ok bool) {
	if entry, found := keyNameCodepoints[baseKey]; found {
		return entry.cp, true
	}
	if isPrintableKey(baseKey) {
		return int(baseKey[0]), true
	}
	return 0, false
}

// isPrintableKey reports a base key that is a single letter, digit or symbol.
func isPrintableKey(baseKey string) bool {
	if len(baseKey) != 1 {
		return false
	}
	ch := baseKey[0]
	return ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || isSymbolKey(ch)
}

func isSymbolKey(ch byte) bool {
	return ch < 0x80 && strings.IndexByte("`-=[]\\;',./!@#$%^&*()_+|~{}:<>?", ch) >= 0
}

// shiftedSymbolKeys are the symbols a terminal reports as their produced
// character plus Shift.
const shiftedSymbolKeys = "~!@#$%^&*()_+|{}:<>?"

// isSymbolCodepoint mirrors SYMBOL_KEYS.has(String.fromCharCode(cp)),
// including fromCharCode's truncation to 16 bits.
func isSymbolCodepoint(cp int) bool {
	unit := cp & 0xFFFF
	return unit < 0x80 && isSymbolKey(byte(unit))
}

func isShiftedSymbolCodepoint(cp int) bool {
	unit := cp & 0xFFFF
	return unit < 0x80 && strings.IndexByte(shiftedSymbolKeys, byte(unit)) >= 0
}

// rawCtrlChar returns the control byte Ctrl plus key produces, or 0.
func rawCtrlChar(key byte) byte {
	switch {
	case key >= 'a' && key <= 'z', key == '[', key == '\\', key == ']', key == '_':
		return key & 0x1f
	case key == '-':
		return 31
	}
	return 0
}

// normalizeShiftedLetterIdentityCodepoint makes an uppercase letter with Shift
// held name the same key as its lowercase letter.
func normalizeShiftedLetterIdentityCodepoint(codepoint, modifier int) int {
	if (modifier&^lockMask)&modShift != 0 && codepoint >= 'A' && codepoint <= 'Z' {
		return codepoint + 32
	}
	return codepoint
}

// logicalShiftedSymbol reads a symbol reported as Shift plus its physical key
// as the produced symbol without the implicit Shift; allowed says which
// codepoints the caller accepts as symbols.
func logicalShiftedSymbol(codepoint int, present bool, modifier int, allowed func(int) bool) (cp, mod int, ok bool) {
	if !present || modifier&modShift == 0 {
		return 0, 0, false
	}
	normalized := normalizeKittyFunctionalCodepoint(codepoint)
	if !allowed(normalized) {
		return 0, 0, false
	}
	return normalized, modifier &^ modShift, true
}

// matchesKittySequence compares a Kitty key with the expected codepoint and
// modifiers: physically (every modifier equal, codepoint equal after keypad
// and shifted-letter normalization), logically (a symbol reported as Shift
// plus its key names the produced symbol), or by the base-layout key when the
// codepoint is neither a Latin letter nor a known symbol, so Ctrl+С on a
// Cyrillic layout triggers ctrl+c while Dvorak Ctrl+K (base v) never matches
// ctrl+v.
func matchesKittySequence(pk *parsedKey, expectedCodepoint, expectedModifier int) bool {
	actualMod, expectedMod := pk.modifier&^lockMask, expectedModifier&^lockMask
	normalized := normalizeShiftedLetterIdentityCodepoint(normalizeKittyFunctionalCodepoint(pk.codepoint), pk.modifier)
	if actualMod == expectedMod && normalized == normalizeShiftedLetterIdentityCodepoint(normalizeKittyFunctionalCodepoint(expectedCodepoint), expectedModifier) {
		return true
	}
	if cp, mod, ok := logicalShiftedSymbol(pk.shiftedKey, pk.hasShiftedKey, actualMod, isSymbolCodepoint); ok &&
		mod&^lockMask == expectedMod && cp == normalizeKittyFunctionalCodepoint(expectedCodepoint) {
		return true
	}
	if actualMod != expectedMod {
		return false
	}
	if pk.hasBaseLayoutKey && pk.baseLayoutKey == expectedCodepoint {
		isLatinLetter := normalized >= 'a' && normalized <= 'z'
		return !isLatinLetter && !isSymbolCodepoint(normalized)
	}
	return false
}

// matchesPrintableModifyOtherKeys compares modifyOtherKeys input for a letter,
// digit or symbol key, physically or as a produced shifted symbol.
func matchesPrintableModifyOtherKeys(pk *parsedKey, expectedKeycode, expectedModifier int) bool {
	if pk == nil {
		return false
	}
	if pk.modifier == expectedModifier && !pk.inexactModifier &&
		normalizeShiftedLetterIdentityCodepoint(pk.codepoint, pk.modifier) == normalizeShiftedLetterIdentityCodepoint(expectedKeycode, expectedModifier) {
		return true
	}
	cp, mod, ok := logicalShiftedSymbol(pk.codepoint, true, pk.modifier, isShiftedSymbolCodepoint)
	return ok && mod == expectedModifier && cp == expectedKeycode
}

// legacyKeyInputs lists the terminal sequences of the keys that have no
// codepoint in Pi's key vocabulary: a function key matches only without a
// modifier, and clear also with exactly shift or ctrl.
var legacyKeyInputs = map[string][]string{
	"clear":       {"\x1b[E", "\x1bOE"},
	"shift+clear": {"\x1b[e"},
	"ctrl+clear":  {"\x1bOe"},
	"f1":          {"\x1bOP", "\x1b[11~", "\x1b[[A"},
	"f2":          {"\x1bOQ", "\x1b[12~", "\x1b[[B"},
	"f3":          {"\x1bOR", "\x1b[13~", "\x1b[[C"},
	"f4":          {"\x1bOS", "\x1b[14~", "\x1b[[D"},
	"f5":          {"\x1b[15~", "\x1b[[E"},
	"f6":          {"\x1b[17~"},
	"f7":          {"\x1b[18~"},
	"f8":          {"\x1b[19~"},
	"f9":          {"\x1b[20~"},
	"f10":         {"\x1b[21~"},
	"f11":         {"\x1b[23~"},
	"f12":         {"\x1b[24~"},
}

// MatchesKey reports whether raw terminal input data is the key keyID names,
// such as "ctrl+c", "shift+tab", "alt+backspace" or "ctrl+shift+left".
// Letters, digits, symbols, arrows, the editing keys and the named keys are
// matched across Kitty CSI u, xterm modifyOtherKeys and legacy sequences, with
// the Kitty flag deciding ambiguous legacy bytes; function keys and clear
// match their legacy sequences only. An identifier the grammar rejects matches
// nothing.
func MatchesKey(data, keyID string) bool {
	baseKey, modifier, ok := parseKeyID(keyID)
	if !ok {
		return false
	}
	if cp, dynamic := resolveKeyCodepoint(baseKey); dynamic {
		return matchesKeyInfo(data, baseKey, modifier, cp)
	}
	legacyKey := baseKey
	switch modifier {
	case 0:
	case modShift:
		legacyKey = "shift+" + baseKey
	case modCtrl:
		legacyKey = "ctrl+" + baseKey
	default:
		return false
	}
	return slices.Contains(legacyKeyInputs[legacyKey], data)
}

// matchesKeyInfo matches input against a base key that has a codepoint.
func matchesKeyInfo(data, baseKey string, expectedMod, expectedCP int) bool {
	// escape with a modifier matches nothing, although ParseKey names such input.
	if expectedCP == cpEscape && expectedMod != 0 {
		return false
	}
	if pk := parseCSIu(data); pk != nil {
		if matchesKittySequence(pk, expectedCP, expectedMod) {
			return true
		}
		return expectedCP == cpEnter && matchesKittySequence(pk, cpKPEnter, expectedMod)
	}
	modifyOtherKeys := parseModifyOtherKeys(data)
	if modifyOtherKeys != nil && isPrintableKey(baseKey) {
		return matchesPrintableModifyOtherKeys(modifyOtherKeys, expectedCP, expectedMod)
	}
	// Without a modifier only escape, space and backspace accept modifyOtherKeys
	// input; enter and tab match their raw byte or a Kitty sequence.
	if expectedMod == 0 && expectedCP != cpEscape && expectedCP != cpSpace && expectedCP != cpBackspace && modifyOtherKeys != nil {
		return false
	}
	if pk := parseTerminalInput(data); pk != nil {
		return pk.codepoint == expectedCP && pk.modifier == expectedMod && !pk.inexactModifier
	}
	return matchesLegacy(data, baseKey, expectedMod)
}

// matchesLegacy handles the pre-CSI u sequences of each key.
func matchesLegacy(data, baseKey string, expectedMod int) bool {
	kitty := IsKittyProtocolActive()
	switch baseKey {
	case "escape", "esc":
		return expectedMod == 0 && data == "\x1b"
	case "enter", "return":
		switch expectedMod {
		case 0:
			return data == "\r" || !kitty && data == "\n" || data == "\x1bOM"
		case modShift:
			return kitty && (data == "\x1b\r" || data == "\n")
		case modAlt:
			return !kitty && data == "\x1b\r"
		}
		return false
	case "tab":
		switch expectedMod {
		case 0:
			return data == "\t"
		case modShift:
			return data == "\x1b[Z"
		}
		return false
	case "space":
		switch expectedMod {
		case 0:
			return data == " "
		case modCtrl:
			return !kitty && data == "\x00"
		case modAlt:
			return !kitty && data == "\x1b "
		}
		return false
	case "backspace":
		switch expectedMod {
		case 0, modCtrl:
			return matchesRawBackspace(data, expectedMod)
		case modAlt:
			return data == "\x1b\x7f" || data == "\x1b\x08"
		}
		return false
	case "delete":
		if expectedMod == 0 {
			return data == "\x1b[3~"
		}
		return matchesLegacyModSeq(data, baseKey, expectedMod)
	case "insert":
		if expectedMod == 0 {
			return data == "\x1b[2~"
		}
		return matchesLegacyModSeq(data, baseKey, expectedMod)
	case "home":
		if expectedMod == 0 {
			return data == "\x1b[H" || data == "\x1bOH" || data == "\x1b[1~" || data == "\x1b[7~"
		}
		return matchesLegacyModSeq(data, baseKey, expectedMod)
	case "end":
		if expectedMod == 0 {
			return data == "\x1b[F" || data == "\x1bOF" || data == "\x1b[4~" || data == "\x1b[8~"
		}
		return matchesLegacyModSeq(data, baseKey, expectedMod)
	case "pageup":
		if expectedMod == 0 {
			return data == "\x1b[5~" || data == "\x1b[[5~"
		}
		return matchesLegacyModSeq(data, baseKey, expectedMod)
	case "pagedown":
		if expectedMod == 0 {
			return data == "\x1b[6~" || data == "\x1b[[6~"
		}
		return matchesLegacyModSeq(data, baseKey, expectedMod)
	case "up":
		switch expectedMod {
		case 0:
			return data == "\x1b[A" || data == "\x1bOA"
		case modAlt:
			return data == "\x1bp" || data == "\x1b[1;3A"
		}
		return matchesLegacyModSeq(data, baseKey, expectedMod)
	case "down":
		switch expectedMod {
		case 0:
			return data == "\x1b[B" || data == "\x1bOB"
		case modAlt:
			return data == "\x1bn" || data == "\x1b[1;3B"
		}
		return matchesLegacyModSeq(data, baseKey, expectedMod)
	case "left":
		switch expectedMod {
		case 0:
			return data == "\x1b[D" || data == "\x1bOD"
		case modAlt:
			return data == "\x1bb" || !kitty && data == "\x1bB" || data == "\x1b[1;3D"
		}
		return matchesLegacyModSeq(data, baseKey, expectedMod)
	case "right":
		switch expectedMod {
		case 0:
			return data == "\x1b[C" || data == "\x1bOC"
		case modAlt:
			return data == "\x1bf" || !kitty && data == "\x1bF" || data == "\x1b[1;3C"
		}
		return matchesLegacyModSeq(data, baseKey, expectedMod)
	}

	if len(baseKey) != 1 {
		return false
	}
	ch := baseKey[0]
	switch expectedMod {
	case 0:
		return len(data) == 1 && data[0] == ch
	case modCtrl:
		if rc := rawCtrlChar(ch); rc != 0 {
			return len(data) == 1 && data[0] == rc
		}
	case modAlt:
		return !kitty && len(data) == 2 && data[0] == 0x1b && data[1] == ch
	case modCtrl | modAlt:
		if rc := rawCtrlChar(ch); rc != 0 && !kitty {
			return len(data) == 2 && data[0] == 0x1b && data[1] == rc
		}
	case modShift:
		if ch >= 'a' && ch <= 'z' {
			return len(data) == 1 && data[0] == ch-32
		}
	}
	return false
}

// matchesLegacyModSeq matches the modified navigation keys: xterm's
// ESC [ 1 ; modifier final and ESC [ number ; modifier ~, and the rxvt shift
// and ctrl variants.
func matchesLegacyModSeq(data, baseKey string, mod int) bool {
	if entry, ok := keyNameCodepoints[baseKey]; ok {
		if final, ok := arrowCPtoFinal[entry.cp]; ok && data == "\x1b[1;"+strconv.Itoa(mod+1)+string(final) {
			return true
		}
		if num, ok := funcCPtoNum[entry.cp]; ok && data == "\x1b["+strconv.Itoa(num)+";"+strconv.Itoa(mod+1)+"~" {
			return true
		}
	}
	rxvt := map[string]string{"up": "a", "down": "b", "right": "c", "left": "d", "insert": "2", "delete": "3", "pageup": "5", "pagedown": "6", "home": "7", "end": "8"}
	suffix, ok := rxvt[baseKey]
	if !ok {
		return false
	}
	isArrow := len(suffix) == 1 && suffix[0] >= 'a'
	switch mod {
	case modShift:
		if isArrow {
			return data == "\x1b["+suffix
		}
		return data == "\x1b["+suffix+"$"
	case modCtrl:
		if isArrow {
			return data == "\x1bO"+suffix
		}
		return data == "\x1b["+suffix+"^"
	}
	return false
}

// matchesRawBackspace reads DEL as backspace and BS as backspace, or as
// ctrl+backspace inside a local Windows Terminal session.
func matchesRawBackspace(data string, expectedModifier int) bool {
	switch data {
	case "\x7f":
		return expectedModifier == 0
	case "\x08":
		if isLocalWindowsTerminalSession() {
			return expectedModifier == modCtrl
		}
		return expectedModifier == 0
	}
	return false
}

func isLocalWindowsTerminalSession() bool {
	return os.Getenv("WT_SESSION") != "" && os.Getenv("SSH_CONNECTION") == "" && os.Getenv("SSH_CLIENT") == "" && os.Getenv("SSH_TTY") == ""
}

// parsedLegacySequenceKeyIDs names the legacy sequences ParseKey recognizes
// as a whole.
var parsedLegacySequenceKeyIDs = map[string]string{
	"\x1bOA": "up", "\x1bOB": "down", "\x1bOC": "right", "\x1bOD": "left", "\x1bOH": "home", "\x1bOF": "end",
	"\x1b[E": "clear", "\x1bOE": "clear", "\x1bOe": "ctrl+clear", "\x1b[e": "shift+clear",
	"\x1b[2~": "insert", "\x1b[2$": "shift+insert", "\x1b[2^": "ctrl+insert", "\x1b[3$": "shift+delete", "\x1b[3^": "ctrl+delete",
	"\x1b[[5~": "pageUp", "\x1b[[6~": "pageDown",
	"\x1b[a": "shift+up", "\x1b[b": "shift+down", "\x1b[c": "shift+right", "\x1b[d": "shift+left",
	"\x1bOa": "ctrl+up", "\x1bOb": "ctrl+down", "\x1bOc": "ctrl+right", "\x1bOd": "ctrl+left",
	"\x1b[5$": "shift+pageUp", "\x1b[6$": "shift+pageDown", "\x1b[7$": "shift+home", "\x1b[8$": "shift+end",
	"\x1b[5^": "ctrl+pageUp", "\x1b[6^": "ctrl+pageDown", "\x1b[7^": "ctrl+home", "\x1b[8^": "ctrl+end",
	"\x1bOP": "f1", "\x1bOQ": "f2", "\x1bOR": "f3", "\x1bOS": "f4",
	"\x1b[11~": "f1", "\x1b[12~": "f2", "\x1b[13~": "f3", "\x1b[14~": "f4",
	"\x1b[[A": "f1", "\x1b[[B": "f2", "\x1b[[C": "f3", "\x1b[[D": "f4", "\x1b[[E": "f5",
	"\x1b[15~": "f5", "\x1b[17~": "f6", "\x1b[18~": "f7", "\x1b[19~": "f8", "\x1b[20~": "f9",
	"\x1b[21~": "f10", "\x1b[23~": "f11", "\x1b[24~": "f12",
	"\x1bb": "alt+left", "\x1bf": "alt+right", "\x1bp": "alt+up", "\x1bn": "alt+down",
}

// ParseKey names raw terminal input with Pi's canonical key identifier, such
// as "ctrl+c", "shift+enter", "pageUp" or "a"; ok is false when the input is
// not one recognized key. Modifiers are listed in the order shift, ctrl, alt,
// super.
func ParseKey(data string) (keyID string, ok bool) {
	if kitty := parseCSIu(data); kitty != nil {
		codepoint, modifier := kitty.codepoint, kitty.modifier
		if cp, mod, ok := logicalShiftedSymbol(kitty.shiftedKey, kitty.hasShiftedKey, kitty.modifier, isSymbolCodepoint); ok {
			codepoint, modifier = cp, mod
		}
		return formatParsedKey(codepoint, modifier, kitty.baseLayoutKey, kitty.hasBaseLayoutKey)
	}
	if modified := parseModifyOtherKeys(data); modified != nil {
		codepoint, modifier := modified.codepoint, modified.modifier
		if cp, mod, ok := logicalShiftedSymbol(modified.codepoint, true, modified.modifier, isShiftedSymbolCodepoint); ok {
			codepoint, modifier = cp, mod
		}
		return formatParsedKey(codepoint, modifier, 0, false)
	}
	if modified := parseModifiedArrow(data); modified != nil {
		return formatParsedKey(modified.codepoint, modified.modifier, 0, false)
	}
	if modified := parseModifiedFunc(data); modified != nil {
		return formatParsedKey(modified.codepoint, modified.modifier, 0, false)
	}

	kitty := IsKittyProtocolActive()
	if kitty && (data == "\x1b\r" || data == "\n") {
		return "shift+enter", true
	}
	if keyID, ok := parsedLegacySequenceKeyIDs[data]; ok {
		return keyID, true
	}

	switch data {
	case "\x1b":
		return "escape", true
	case "\x1c":
		return "ctrl+\\", true
	case "\x1d":
		return "ctrl+]", true
	case "\x1f":
		return "ctrl+-", true
	case "\x1b\x1b":
		return "ctrl+alt+[", true
	case "\x1b\x1c":
		return "ctrl+alt+\\", true
	case "\x1b\x1d":
		return "ctrl+alt+]", true
	case "\x1b\x1f":
		return "ctrl+alt+-", true
	case "\t":
		return "tab", true
	case "\r", "\x1bOM":
		return "enter", true
	case "\n":
		if !kitty {
			return "enter", true
		}
	case "\x00":
		return "ctrl+space", true
	case " ":
		return "space", true
	case "\x7f":
		return "backspace", true
	case "\x08":
		if isLocalWindowsTerminalSession() {
			return "ctrl+backspace", true
		}
		return "backspace", true
	case "\x1b[Z":
		return "shift+tab", true
	case "\x1b\r":
		if !kitty {
			return "alt+enter", true
		}
	case "\x1b ":
		if !kitty {
			return "alt+space", true
		}
	case "\x1b\x7f", "\x1b\x08":
		return "alt+backspace", true
	case "\x1bB":
		if !kitty {
			return "alt+left", true
		}
	case "\x1bF":
		if !kitty {
			return "alt+right", true
		}
	case "\x1b[A":
		return "up", true
	case "\x1b[B":
		return "down", true
	case "\x1b[C":
		return "right", true
	case "\x1b[D":
		return "left", true
	case "\x1b[H":
		return "home", true
	case "\x1b[F":
		return "end", true
	case "\x1b[3~":
		return "delete", true
	case "\x1b[5~":
		return "pageUp", true
	case "\x1b[6~":
		return "pageDown", true
	}

	if !kitty && len(data) == 2 && data[0] == 0x1b {
		c := data[1]
		if c >= 1 && c <= 26 {
			return "ctrl+alt+" + string(rune(c+96)), true
		}
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || isSymbolKey(c) {
			return "alt+" + string(rune(c)), true
		}
	}
	if len(data) == 1 {
		c := data[0]
		if c >= 1 && c <= 26 {
			return "ctrl+" + string(rune(c+96)), true
		}
		if c >= 32 && c <= 126 {
			return data, true
		}
	}
	return "", false
}

func formatParsedKey(codepoint, modifier int, baseLayoutKey int, hasBaseLayoutKey bool) (string, bool) {
	normalized := normalizeKittyFunctionalCodepoint(codepoint)
	identity := normalizeShiftedLetterIdentityCodepoint(normalized, modifier)
	isLatinLetter := identity >= 'a' && identity <= 'z'
	isDigit := identity >= '0' && identity <= '9'
	effective := identity
	if !isLatinLetter && !isDigit && !isSymbolCodepoint(identity) && hasBaseLayoutKey {
		effective = baseLayoutKey
	}
	keyName, ok := parsedKeyName(effective)
	if !ok {
		return "", false
	}
	return formatKeyNameWithModifiers(keyName, modifier)
}

func parsedKeyName(codepoint int) (string, bool) {
	switch codepoint {
	case cpEscape:
		return "escape", true
	case cpTab:
		return "tab", true
	case cpEnter, cpKPEnter:
		return "enter", true
	case cpSpace:
		return "space", true
	case cpBackspace:
		return "backspace", true
	case cpDelete:
		return "delete", true
	case cpInsert:
		return "insert", true
	case cpHome:
		return "home", true
	case cpEnd:
		return "end", true
	case cpPageUp:
		return "pageUp", true
	case cpPageDown:
		return "pageDown", true
	case cpArrowUp:
		return "up", true
	case cpArrowDown:
		return "down", true
	case cpArrowLeft:
		return "left", true
	case cpArrowRight:
		return "right", true
	}
	if codepoint >= '0' && codepoint <= '9' || codepoint >= 'a' && codepoint <= 'z' {
		return string(rune(codepoint)), true
	}
	if isSymbolCodepoint(codepoint) {
		return string(rune(codepoint & 0xFFFF)), true
	}
	return "", false
}

func formatKeyNameWithModifiers(keyName string, modifier int) (string, bool) {
	effective := modifier &^ lockMask
	if effective&^(modShift|modCtrl|modAlt|modSuper) != 0 {
		return "", false
	}
	var modifiers []string
	for _, m := range []struct {
		bit  int
		name string
	}{{modShift, "shift"}, {modCtrl, "ctrl"}, {modAlt, "alt"}, {modSuper, "super"}} {
		if effective&m.bit != 0 {
			modifiers = append(modifiers, m.name)
		}
	}
	if len(modifiers) == 0 {
		return keyName, true
	}
	return strings.Join(modifiers, "+") + "+" + keyName, true
}

// validKeySpecialNames are the named keys MatchesKey accepts as a base key,
// in the lowercase spelling parseKeyID produces.
var validKeySpecialNames = map[string]bool{
	"escape": true, "esc": true, "enter": true, "return": true, "tab": true, "space": true, "backspace": true,
	"delete": true, "insert": true, "clear": true, "home": true, "end": true, "pageup": true, "pagedown": true,
	"up": true, "down": true, "left": true, "right": true,
	"f1": true, "f2": true, "f3": true, "f4": true, "f5": true, "f6": true, "f7": true, "f8": true, "f9": true, "f10": true, "f11": true, "f12": true,
}

var validKeyModifiers = map[string]bool{"ctrl": true, "shift": true, "alt": true, "super": true}

// validKeySingleChars are the single-character base keys the grammar accepts.
const validKeySingleChars = "abcdefghijklmnopqrstuvwxyz0123456789_-!@#$%^&*()|~`'\":;,./<>?[]{}=\\"

// ValidKeyID reports whether spec is a well-formed key identifier for a
// user-configured key such as a collapse key: zero or more distinct modifiers
// from ctrl, shift, alt and super, then a base key that is a single lowercase
// letter, digit or symbol, or a named key. The check is strict because
// MatchesKey takes the last "+" part as the key and ignores unknown parts, so
// a typo like "ctr+]" would otherwise match every bare "]". It mirrors the
// TypeScript isValidCollapseKeySpec, which expects the lowercased spec:
// "Alt+O" is rejected, "alt+o" accepted.
func ValidKeyID(spec string) bool {
	if spec == "" || strings.HasPrefix(spec, "+") || strings.HasSuffix(spec, "+") || strings.Contains(spec, "++") {
		return false
	}
	parts := strings.Split(spec, "+")
	base := parts[len(parts)-1]
	seen := make(map[string]bool, len(parts))
	for _, modifier := range parts[:len(parts)-1] {
		if seen[modifier] || !validKeyModifiers[modifier] {
			return false
		}
		seen[modifier] = true
	}
	if LenUTF16(base) == 1 {
		return len(base) == 1 && strings.IndexByte(validKeySingleChars, base[0]) >= 0
	}
	return validKeySpecialNames[base]
}
