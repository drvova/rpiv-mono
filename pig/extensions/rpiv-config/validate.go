package config

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"unicode"
)

// ValidateConfig overlays value, which must be a plain JSON object, onto the defaults held in dst. It is the
// TypeBox-free form of validateConfig(schema, value): the destination struct is the schema and the values it
// holds on entry are the defaults.
//
// Contract:
//
//   - dst is a non-nil pointer to a struct. Its exported fields define the schema under encoding/json's
//     naming: the `json` tag name, else the Go field name; a `json:"-"` field is not part of the schema;
//     the exported fields of embedded structs (by value or by pointer) are promoted with encoding/json's
//     precedence rules (a shallower field hides a deeper one of the same name, a tagged field wins over
//     untagged ones at the same depth, any other tie makes the name ambiguous and it is ignored).
//   - value must be a plain object: a map[string]any (what LoadJSONConfig returns), a
//     map[string]json.RawMessage, or any other map with string keys or struct, optionally behind a pointer,
//     through its JSON encoding. nil, a nil pointer, a scalar, a string, a slice (including []byte and
//     json.RawMessage) and an array are not objects.
//   - For every key of value whose name exactly matches a schema field (case-sensitive), the key's value is
//     decoded by encoding/json into a fresh zero value of the field's type and replaces the field wholesale.
//     A nested object therefore does not merge with the field's default: it replaces it, as the shallow
//     spread of the TypeScript version does. Inside the value, encoding/json's own rules apply: unknown
//     nested keys are ignored, a field type's UnmarshalJSON is honored, the `,string` tag option is not.
//   - Keys that match no schema field are dropped. Schema fields without a key keep their defaults.
//   - A key whose value does not decode cleanly into the field's type (wrong JSON type, number overflow,
//     a failing UnmarshalJSON) is dropped as if it were absent, so the field keeps its default. JSON null
//     decodes to the field type's zero value.
//   - ValidateConfig never writes through a pointer it finds in dst: a promoted field reached through an
//     embedded pointer is set on a copy of the pointee (allocated when nil), so defaults shared between
//     calls are never mutated. A field promoted through an unexported embedded pointer cannot be reached
//     that way (nor by encoding/json when the pointer is nil), so its keys are dropped.
//
// It returns false and leaves dst untouched, that is at its defaults, when dst or value does not satisfy the
// contract; otherwise it returns true, also when every key was dropped. (The TypeScript version returns an
// empty object in the false case; for a struct the defaults are the only "nothing was read" result whose
// fields are still meaningful.)
func ValidateConfig(dst any, value any) bool {
	target := reflect.ValueOf(dst)
	if target.Kind() != reflect.Pointer || target.IsNil() || target.Elem().Kind() != reflect.Struct {
		return false
	}
	members, ok := objectMembers(value)
	if !ok {
		return false
	}
	structV := target.Elem()
	fields := jsonFields(structV.Type())
	for key, raw := range members {
		index, known := fields[key]
		if !known {
			continue
		}
		fresh := reflect.New(structV.Type().FieldByIndex(index).Type)
		if err := json.Unmarshal(raw, fresh.Interface()); err != nil {
			continue
		}
		if field := settableField(structV, index); field.IsValid() {
			field.Set(fresh.Elem())
		}
	}
	return true
}

// ValidateConfigAs is the generic, value-returning form of ValidateConfig: it overlays value onto a copy of
// defaults (a struct value carrying the defaults) and returns the result, leaving defaults untouched. When
// value is not a plain object, or T is not a struct type, the result equals defaults.
func ValidateConfigAs[T any](defaults T, value any) T {
	out := defaults
	ValidateConfig(&out, value)
	return out
}

// objectMembers returns the members of value when it is a plain object in the sense of ValidateConfig, each
// as its JSON encoding. A member whose value cannot be encoded (NaN, a channel, …) is left out.
func objectMembers(value any) (map[string]json.RawMessage, bool) {
	switch v := value.(type) {
	case nil:
		return nil, false
	case map[string]json.RawMessage:
		return v, true
	case map[string]any:
		out := make(map[string]json.RawMessage, len(v))
		for k, e := range v {
			raw, err := json.Marshal(e)
			if err != nil {
				continue
			}
			out[k] = raw
		}
		return out, true
	}
	rv := reflect.ValueOf(value)
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
	case reflect.Struct:
	default:
		return nil, false
	}
	data, err := json.Marshal(rv.Interface())
	if err != nil {
		return nil, false
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, false // a struct that encodes as something other than an object, like time.Time
	}
	if out == nil {
		out = map[string]json.RawMessage{} // a nil map encodes as null but is an empty object
	}
	return out, true
}

// settableField walks index on v, an addressable struct, and returns the settable field at its end, or an
// invalid Value when the path crosses a pointer that cannot be set (an unexported embedded pointer). Every
// pointer on the way is replaced by a copy of its pointee, or a fresh zero when nil, so the write never
// reaches memory shared with the caller's defaults.
func settableField(v reflect.Value, index []int) reflect.Value {
	for _, i := range index {
		for v.Kind() == reflect.Pointer {
			if !v.CanSet() {
				return reflect.Value{}
			}
			fresh := reflect.New(v.Type().Elem())
			if !v.IsNil() {
				fresh.Elem().Set(v.Elem())
			}
			v.Set(fresh)
			v = fresh.Elem()
		}
		v = v.Field(i)
	}
	if !v.CanSet() {
		return reflect.Value{}
	}
	return v
}

// jsonFields maps every JSON object key encoding/json would fill on the struct type t to the index path of
// the field it fills, following encoding/json's typeFields: breadth-first over embedded structs, the `json`
// tag name (when valid) else the Go field name, `-` excluded, unexported fields ignored unless they are
// embedded structs whose exported fields are promoted, and the dominance rules described on ValidateConfig.
// Keys that encoding/json would only accept by case-insensitive matching are not included.
func jsonFields(t reflect.Type) map[string][]int {
	type candidate struct {
		index  []int
		depth  int
		tagged bool
	}
	type queued struct {
		typ   reflect.Type
		index []int
	}
	byName := map[string][]candidate{}
	visited := map[reflect.Type]bool{}
	next := []queued{{typ: t}}
	var count, nextCount map[reflect.Type]int
	for depth := 0; len(next) > 0; depth++ {
		current := next
		next = nil
		count, nextCount = nextCount, map[reflect.Type]int{}
		for _, q := range current {
			if visited[q.typ] {
				continue
			}
			visited[q.typ] = true
			for i := range q.typ.NumField() {
				sf := q.typ.Field(i)
				ft := sf.Type
				if ft.Kind() == reflect.Pointer {
					ft = ft.Elem()
				}
				if sf.Anonymous {
					if !sf.IsExported() && ft.Kind() != reflect.Struct {
						continue
					}
				} else if !sf.IsExported() {
					continue
				}
				tag := sf.Tag.Get("json")
				if tag == "-" {
					continue
				}
				name, _, _ := strings.Cut(tag, ",")
				if !isValidTag(name) {
					name = ""
				}
				index := append(slices.Clone(q.index), i)
				if name == "" && sf.Anonymous && ft.Kind() == reflect.Struct {
					// An embedded struct without a tag name: its fields are promoted at the next depth.
					nextCount[ft]++
					if nextCount[ft] == 1 {
						next = append(next, queued{typ: ft, index: index})
					}
					continue
				}
				c := candidate{index: index, depth: depth, tagged: name != ""}
				if name == "" {
					name = sf.Name
				}
				byName[name] = append(byName[name], c)
				if count[q.typ] > 1 {
					// The same embedded type reached twice at this depth: its fields are ambiguous, so the
					// candidate is doubled for the tie-break below, exactly as encoding/json does.
					byName[name] = append(byName[name], c)
				}
			}
		}
	}
	out := make(map[string][]int, len(byName))
	for name, cands := range byName {
		// Candidates were appended in depth order, so the first one is at the shallowest depth.
		var shallowest, tagged []candidate
		for _, c := range cands {
			if c.depth != cands[0].depth {
				break
			}
			shallowest = append(shallowest, c)
			if c.tagged {
				tagged = append(tagged, c)
			}
		}
		switch {
		case len(shallowest) == 1:
			out[name] = shallowest[0].index
		case len(tagged) == 1:
			out[name] = tagged[0].index
		}
	}
	return out
}

// isValidTag is encoding/json's test for a usable tag name: letters, digits and a fixed set of
// punctuation; anything else makes encoding/json fall back to the Go field name.
func isValidTag(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		switch {
		case strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", c):
			// Backslash and quote chars are reserved, but otherwise any punctuation chars are allowed in a tag name.
		case !unicode.IsLetter(c) && !unicode.IsDigit(c):
			return false
		}
	}
	return true
}
