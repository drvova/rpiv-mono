package workflow

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

// The value guards CheckSchema leans on that TypeBox implements itself rather
// than taking from the JSON Schema specification: string lengths measured
// the way TypeBox's guard/string module measures them, its multipleOf
// tolerance and its built-in `format` checkers. Each routine here follows the
// typebox@1.1.39 original step for step, so the two validators agree on
// every input — including the original's shortcuts.

// utf16Units is the string as JavaScript sees it: UTF-16 code units.
func utf16Units(s string) []uint16 { return utf16.Encode([]rune(s)) }

// codePointAt is String.prototype.codePointAt: the code point starting at
// unit i and how many units it spans (a lone surrogate reads as itself).
func codePointAt(u []uint16, i int) (cp int, n int) {
	c := int(u[i])
	if 0xD800 <= c && c <= 0xDBFF && i+1 < len(u) {
		if d := int(u[i+1]); 0xDC00 <= d && d <= 0xDFFF {
			return 0x10000 + ((c - 0xD800) << 10) + (d - 0xDC00), 2
		}
	}
	return c, 1
}

func isRegionalIndicator(cp int) bool { return 0x1F1E6 <= cp && cp <= 0x1F1FF }

func isVariationSelector(cp int) bool { return 0xFE00 <= cp && cp <= 0xFE0F }

func isCombiningMark(cp int) bool {
	return (0x0300 <= cp && cp <= 0x036F) || (0x1AB0 <= cp && cp <= 0x1AFF) || (0x1DC0 <= cp && cp <= 0x1DFF) || (0xFE20 <= cp && cp <= 0xFE2F)
}

// isGraphemeCodeUnit is the trigger that switches TypeBox from counting
// code units to walking grapheme clusters: a high surrogate, a combining
// diacritical mark or a zero-width joiner.
func isGraphemeCodeUnit(c uint16) bool {
	return (0xD800 <= c && c <= 0xDBFF) || (0x0300 <= c && c <= 0x036F) || c == 0x200D
}

func consumeModifiers(u []uint16, index int) int {
	for index < len(u) {
		cp, n := codePointAt(u, index)
		if !isCombiningMark(cp) && !isVariationSelector(cp) {
			break
		}
		index += n
	}
	return index
}

// nextGraphemeClusterIndex is TypeBox's cluster walker: a base code point,
// its combining marks / variation selectors, any ZWJ-joined sequence, and a
// second regional indicator when the base was one (flags).
func nextGraphemeClusterIndex(u []uint16, start int) int {
	startCP, n := codePointAt(u, start)
	end := consumeModifiers(u, start+n)
	for end < len(u)-1 && u[end] == 0x200D {
		_, m := codePointAt(u, end+1)
		end = consumeModifiers(u, end+1+m)
	}
	if isRegionalIndicator(startCP) && end < len(u) {
		if cp, m := codePointAt(u, end); isRegionalIndicator(cp) {
			end += m
		}
	}
	return end
}

// isMinLength is TypeBox's IsMinLengthFast: code units are counted until a
// grapheme trigger is met (then the cluster walk decides) or the minimum is
// reached, whichever comes first.
func isMinLength(s string, minLength float64) bool {
	if minLength == 0 {
		return true
	}
	u := utf16Units(s)
	for index := 0; index < len(u); {
		if isGraphemeCodeUnit(u[index]) {
			return graphemeMinLength(u, minLength)
		}
		index++
		if float64(index) >= minLength {
			return true
		}
	}
	return false
}

// isMaxLength is TypeBox's IsMaxLengthFast, the mirror of isMinLength.
func isMaxLength(s string, maxLength float64) bool {
	u := utf16Units(s)
	for index := 0; index < len(u); {
		if isGraphemeCodeUnit(u[index]) {
			return graphemeMaxLength(u, maxLength)
		}
		index++
		if float64(index) > maxLength {
			return false
		}
	}
	return true
}

func graphemeMinLength(u []uint16, minLength float64) bool {
	count := 0
	for index := 0; index < len(u); {
		index = nextGraphemeClusterIndex(u, index)
		count++
		if float64(count) >= minLength {
			return true
		}
	}
	return false
}

func graphemeMaxLength(u []uint16, maxLength float64) bool {
	count := 0
	for index := 0; index < len(u); {
		index = nextGraphemeClusterIndex(u, index)
		count++
		if float64(count) > maxLength {
			return false
		}
	}
	return true
}

// isMultipleOf is TypeBox's IsMultipleOf: an integer dividend is a multiple
// of any divisor whose reciprocal is whole; otherwise the truncated
// remainder (or its distance from the divisor) must fall under 1e-10.
func isMultipleOf(dividend, divisor float64) bool {
	const tolerance = 1e-10
	if dividend == math.Trunc(dividend) && math.Mod(1/divisor, 1) == 0 {
		return true
	}
	mod := math.Mod(dividend, divisor)
	return math.Min(math.Abs(mod), math.Abs(mod-divisor)) < tolerance
}

// formatChecks are the built-in formats this validator enforces, under the
// names TypeBox registers them. The remaining TypeBox formats (hostname and
// the idn-*, iri*, uri-reference / uri-template, json-pointer*, duration and
// url families) are not ported: a schema naming one of them constrains
// nothing here (doc.go, Divergences).
var formatChecks = map[string]func(string) bool{
	"date":      isDateFormat,
	"date-time": isDateTimeFormat,
	"email":     isEmailFormat,
	"ipv4":      isIPv4Format,
	"ipv6":      isIPv6Format,
	"regex":     isRegexFormat,
	"time":      isTimeFormat,
	"uri":       isURIFormat,
	"uuid":      isUUIDFormat,
}

// emailLocal / emailDomain are TypeBox's email pattern with its leading
// "no double dot" lookahead lifted into a plain substring test (RE2 has no
// lookaround; the lookahead applies to the whole string).
var emailPattern = regexp.MustCompile("(?i)^[a-z0-9!#$%&'*+/=?^_`{|}~-]+(?:\\.[a-z0-9!#$%&'*+/=?^_`{|}~-]+)*@[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*$")

func isEmailFormat(value string) bool {
	return !strings.Contains(value, "..") && emailPattern.MatchString(value)
}

var datePattern = regexp.MustCompile(`^(\d\d\d\d)-(\d\d)-(\d\d)$`)

var daysInMonth = [...]int{0, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}

func isLeapYear(year int) bool { return year%4 == 0 && (year%100 != 0 || year%400 == 0) }

func isDateFormat(value string) bool {
	m := datePattern.FindStringSubmatch(value)
	if m == nil {
		return false
	}
	year, _ := strconv.Atoi(m[1])
	month, _ := strconv.Atoi(m[2])
	day, _ := strconv.Atoi(m[3])
	if month < 1 || month > 12 || day < 1 {
		return false
	}
	limit := daysInMonth[month]
	if month == 2 && isLeapYear(year) {
		limit = 29
	}
	return day <= limit
}

var timePattern = regexp.MustCompile(`(?i)^(\d\d):(\d\d):(\d\d(?:\.\d+)?)(?:Z|([+-])(\d\d):(\d\d))?$`)

// isTimeFormat is TypeBox's IsTime with its default strict time zone: a
// "Z" or an explicit offset is required, and a leap second is accepted only
// at 23:59 UTC.
func isTimeFormat(value string) bool {
	m := timePattern.FindStringSubmatch(value)
	if m == nil {
		return false
	}
	number := func(s string) float64 {
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
	hr, min, sec := number(m[1]), number(m[2]), number(m[3])
	tzSign := 1.0
	if m[4] == "-" {
		tzSign = -1
	}
	tzH, tzM := number(m[5]), number(m[6])
	if tzH > 23 || tzM > 59 {
		return false
	}
	if m[4] == "" && !strings.Contains(strings.ToLower(value), "z") {
		return false
	}
	if hr <= 23 && min <= 59 && sec < 60 {
		return true
	}
	utcMin := min - tzM*tzSign
	utcHr := hr - tzH*tzSign
	if utcMin < 0 {
		utcHr--
	}
	return (utcHr == 23 || utcHr == -1) && (utcMin == 59 || utcMin == -1) && sec < 61
}

var dateTimeSeparator = regexp.MustCompile("[Tt]")

func isDateTimeFormat(value string) bool {
	parts := dateTimeSeparator.Split(value, -1)
	return len(parts) == 2 && isDateFormat(parts[0]) && isTimeFormat(parts[1])
}

var uuidPattern = regexp.MustCompile(`(?i)^(?:urn:uuid:)?[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$`)

func isUUIDFormat(value string) bool { return uuidPattern.MatchString(value) }

// isIPv4Range is TypeBox's IsIPv4Internal over value[start:end]: four
// dotted decimal octets, no leading zeros.
func isIPv4Range(value string, start, end int) bool {
	dots, num, digits, leading := 0, 0, 0, byte(0)
	for i := start; i < end; i++ {
		ch := value[i]
		switch {
		case ch == '.':
			if digits == 0 || num > 255 || (leading == '0' && digits > 1) {
				return false
			}
			dots++
			num, digits, leading = 0, 0, 0
		case '0' <= ch && ch <= '9':
			if digits == 0 {
				leading = ch
			}
			num = num*10 + int(ch-'0')
			digits++
		default:
			return false
		}
	}
	return dots == 3 && digits > 0 && num <= 255 && !(leading == '0' && digits > 1)
}

func isIPv4Format(value string) bool { return isIPv4Range(value, 0, len(value)) }

func isHexByte(ch int) bool {
	return ('0' <= ch && ch <= '9') || ('A' <= ch && ch <= 'F') || ('a' <= ch && ch <= 'f')
}

// isIPv6Format is TypeBox's IsIPv6: up to eight hex groups, one "::"
// compression, an optional embedded IPv4 tail.
func isIPv6Format(value string) bool {
	length := len(value)
	if length == 0 {
		return false
	}
	at := func(i int) int {
		if i < length {
			return int(value[i])
		}
		return -1
	}
	groups, compressed, i := 0, false, 0
	if at(0) == ':' && at(1) == ':' {
		if length == 2 {
			return true
		}
		compressed = true
		i = 2
	}
	for i < length {
		digits, start := 0, i
		for i < length && isHexByte(at(i)) {
			i++
			digits++
		}
		if digits == 0 {
			return false
		}
		next := at(i)
		if next == '.' {
			if !isIPv4Range(value, start, length) {
				return false
			}
			groups += 2
			break
		}
		if digits > 4 {
			return false
		}
		groups++
		if i == length {
			break
		}
		if next != ':' {
			return false
		}
		i++
		if at(i) == ':' {
			if compressed || at(i+1) == ':' {
				return false
			}
			compressed = true
			i++
			if i == length {
				break
			}
		}
	}
	if compressed {
		return groups <= 7
	}
	return groups == 8
}

func isAlphaByte(ch int) bool { return ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') }

func isAlphaNumericByte(ch int) bool { return isAlphaByte(ch) || ('0' <= ch && ch <= '9') }

func isSchemeByte(ch int) bool { return isAlphaNumericByte(ch) || ch == '+' || ch == '-' || ch == '.' }

func isUnreservedByte(ch int) bool {
	return isAlphaNumericByte(ch) || ch == '-' || ch == '.' || ch == '_' || ch == '~'
}

func isSubDelimByte(ch int) bool {
	switch ch {
	case '!', '$', '&', '\'', '(', ')', '*', '+', ',', ';', '=':
		return true
	}
	return false
}

func isPcharByte(ch int) bool {
	return isUnreservedByte(ch) || isSubDelimByte(ch) || ch == ':' || ch == '@'
}

// isURIFormat is TypeBox's IsUri: a scheme, an optional authority (userinfo,
// host or IP literal, port) and an ASCII path / query / fragment with valid
// percent-encoding. Non-ASCII is tolerated in the host only.
func isURIFormat(value string) bool {
	length := len(value)
	if length == 0 {
		return false
	}
	at := func(i int) int {
		if i < length {
			return int(value[i])
		}
		return -1
	}
	if !isAlphaByte(at(0)) {
		return false
	}
	i := 1
	for i < length {
		ch := at(i)
		if ch == ':' {
			break
		}
		if !isSchemeByte(ch) {
			return false
		}
		i++
	}
	if at(i) != ':' {
		return false
	}
	i++
	if at(i) == '/' && at(i+1) == '/' {
		i += 2
		authorityStart, atPos := i, -1
		for j := i; j < length; j++ {
			ch := at(j)
			if ch == '@' {
				atPos = j
				break
			}
			if ch == '/' || ch == '?' || ch == '#' {
				break
			}
		}
		if atPos != -1 {
			for j := authorityStart; j < atPos; j++ {
				ch := at(j)
				switch {
				case ch == '[' || ch == ']':
					return false
				case ch == '%':
					if j+2 >= atPos || !isHexByte(at(j+1)) || !isHexByte(at(j+2)) {
						return false
					}
					j += 2
				case !isUnreservedByte(ch) && !isSubDelimByte(ch) && ch != ':':
					return false
				}
			}
			i = atPos + 1
		}
		if at(i) == '[' {
			i++
			for i < length && at(i) != ']' {
				i++
			}
			if at(i) != ']' {
				return false
			}
			i++
		} else {
			for i < length {
				ch := at(i)
				if ch == '/' || ch == '?' || ch == '#' || ch == ':' {
					break
				}
				if ch < 128 && !isUnreservedByte(ch) && !isSubDelimByte(ch) {
					return false
				}
				i++
			}
		}
		if at(i) == ':' {
			i++
			for i < length {
				ch := at(i)
				if ch == '/' || ch == '?' || ch == '#' {
					break
				}
				if ch < '0' || ch > '9' {
					return false
				}
				i++
			}
		}
	}
	for i < length {
		ch := at(i)
		switch {
		case ch == '%':
			if i+2 >= length || !isHexByte(at(i+1)) || !isHexByte(at(i+2)) {
				return false
			}
			i += 2
		case ch > 127:
			return false
		case !(isPcharByte(ch) || ch == '/' || ch == '?' || ch == '#'):
			return false
		}
		i++
	}
	return true
}

// isRegexFormat accepts a non-empty pattern RE2 compiles; TypeBox compiles
// with the ECMAScript engine, so lookaround and backreferences differ.
func isRegexFormat(value string) bool {
	if value == "" {
		return false
	}
	_, err := regexp.Compile(value)
	return err == nil
}
