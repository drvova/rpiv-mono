package workflow

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"strings"
)

// JavaScript value semantics the authoring surface leans on — `Number(...)`
// coercion, `String(...)` / template rendering of numbers, `JSON.stringify`
// for diagnostics, and reading a field off an `output.data` value. Kept in one
// place so every message and route note renders exactly as the TypeScript
// implementation did.

// jsNumber mirrors JavaScript's Number(value) coercion for the values a data
// envelope can carry: nil (undefined) → NaN, booleans → 0/1, numbers as-is,
// strings parsed (blank → 0, unparsable → NaN), everything else → NaN.
func jsNumber(v any) float64 {
	switch x := v.(type) {
	case nil:
		return math.NaN()
	case bool:
		if x {
			return 1
		}
		return 0
	case string:
		return jsNumberFromString(x)
	case []any:
		// [] → 0, [x] → Number(x), longer → NaN (the array-to-primitive rule).
		switch len(x) {
		case 0:
			return 0
		case 1:
			return jsNumber(x[0])
		default:
			return math.NaN()
		}
	}
	if f, ok := asFloat(v); ok {
		return f
	}
	return math.NaN()
}

func jsNumberFromString(s string) float64 {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0
	}
	switch t {
	case "Infinity", "+Infinity":
		return math.Inf(1)
	case "-Infinity":
		return math.Inf(-1)
	}
	lower := strings.ToLower(t)
	for prefix, base := range map[string]int{"0x": 16, "0o": 8, "0b": 2} {
		if strings.HasPrefix(lower, prefix) {
			n, err := strconv.ParseUint(lower[2:], base, 64)
			if err != nil {
				return math.NaN()
			}
			return float64(n)
		}
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return math.NaN()
	}
	return f
}

// formatJSNumber renders a float the way JavaScript's String(number) /
// template interpolation does: integers without a fractional part, NaN and
// Infinity by name, other values in the shortest round-trip form.
func formatJSNumber(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case f == math.Trunc(f) && math.Abs(f) < 1e21:
		if f == 0 {
			return "0"
		}
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	abs := math.Abs(f)
	if abs >= 1e21 || abs < 1e-6 {
		s := strconv.FormatFloat(f, 'e', -1, 64)
		// Go renders e+21 / e-07; JavaScript renders e+21 / e-7.
		if i := strings.IndexAny(s, "e"); i >= 0 {
			mant, exp := s[:i], s[i+1:]
			sign := exp[0]
			digits := strings.TrimLeft(exp[1:], "0")
			if digits == "" {
				digits = "0"
			}
			return mant + "e" + string(sign) + digits
		}
		return s
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// jsString mirrors String(value) for the scalars diagnostics interpolate.
func jsString(v any) string {
	switch x := v.(type) {
	case nil:
		return "undefined"
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	}
	if f, ok := asFloat(v); ok {
		return formatJSNumber(f)
	}
	return FormatError(v)
}

// jsonStringify mirrors JSON.stringify for diagnostics: no HTML escaping, no
// trailing newline, "null" for nil. Values that cannot be encoded render as
// their Go string form so a diagnostic never fails to build.
func jsonStringify(v any) string {
	if v == nil {
		return "null"
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return jsString(v)
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

// jsTypeof mirrors `typeof` for the MatchValue scalars.
func jsTypeof(v any) string {
	switch v.(type) {
	case nil:
		return "undefined"
	case string:
		return "string"
	case bool:
		return "boolean"
	}
	if _, ok := asFloat(v); ok {
		return "number"
	}
	return "object"
}

// fieldOf reads `data[field]` off an output's data value: a map keyed by
// string (any value type), or a struct field matched by its json tag or name.
// The second result is false when the field is absent (JavaScript's
// `undefined`), which callers distinguish from an explicit nil.
func fieldOf(data any, field string) (any, bool) {
	if data == nil {
		return nil, false
	}
	rv := reflect.ValueOf(data)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil, false
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return nil, false
		}
		value := rv.MapIndex(reflect.ValueOf(field).Convert(rv.Type().Key()))
		if !value.IsValid() {
			return nil, false
		}
		return value.Interface(), true
	case reflect.Struct:
		t := rv.Type()
		for i := range t.NumField() {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name := f.Name
			if tag, ok := f.Tag.Lookup("json"); ok {
				if tagName, _, _ := strings.Cut(tag, ","); tagName != "" {
					name = tagName
				}
			}
			if name == field {
				return rv.Field(i).Interface(), true
			}
		}
	}
	return nil, false
}

// jsStrictEqual mirrors `===` between a data value and a MatchValue scalar:
// strings and booleans by value, numbers by value across Go numeric types.
func jsStrictEqual(a, b any) bool {
	if fa, ok := asFloat(a); ok {
		fb, ok := asFloat(b)
		return ok && fa == fb
	}
	switch x := a.(type) {
	case string:
		y, ok := b.(string)
		return ok && x == y
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case nil:
		return b == nil
	}
	return false
}
