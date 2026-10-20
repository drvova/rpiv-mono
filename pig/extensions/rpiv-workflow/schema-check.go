package workflow

import (
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// CheckSchema — the validator behind every StageSchema this package builds,
// with the semantics the TypeScript implementation got from TypeBox's
// Value.Check / Value.Errors over the JSON Schema subset the workflow code
// relies on: type (string or array of strings), properties, required,
// additionalProperties (bool or schema), items, enum, const, anyOf / oneOf /
// allOf / not, minimum / maximum / exclusiveMinimum / exclusiveMaximum,
// multipleOf, minLength / maxLength, pattern, minItems / maxItems,
// uniqueItems, minProperties / maxProperties. Unknown keywords are ignored.

// SchemaError is one validation failure: the failing keyword, the JSON
// pointer of the offending value (empty at the root) and a message.
type SchemaError struct {
	Keyword      string
	InstancePath string
	SchemaPath   string
	Message      string
}

// CheckSchema validates value against schema and returns every failure
// (nil when valid). Values are the shapes encoding/json produces
// (map[string]any, []any, float64, string, bool, nil) plus Go's native
// numerics, Schema maps, typed slices and structs, so authored data and
// decoded data validate identically.
func CheckSchema(schema JsonSchemaObject, value any) []SchemaError {
	var errs []SchemaError
	check(schema, value, "", "#", &errs)
	return errs
}

func check(schema JsonSchemaObject, value any, path, schemaPath string, errs *[]SchemaError) {
	if schema == nil {
		return
	}
	v := normalize(value)
	fail := func(keyword, message string) {
		*errs = append(*errs, SchemaError{Keyword: keyword, InstancePath: path, SchemaPath: schemaPath + "/" + keyword, Message: message})
	}

	if t, ok := schema["type"]; ok {
		if !matchesType(t, v) {
			fail("type", "must be "+typeLabel(t))
			return
		}
	}
	if c, ok := schema["const"]; ok && !DeepEqual(normalize(c), v) {
		fail("const", "must be equal to constant")
	}
	if e, ok := schema["enum"]; ok {
		if values, ok := normalize(e).([]any); ok {
			found := false
			for _, candidate := range values {
				if DeepEqual(normalize(candidate), v) {
					found = true
					break
				}
			}
			if !found {
				fail("enum", "must be equal to one of the allowed values")
			}
		}
	}

	switch x := v.(type) {
	case float64:
		checkNumber(schema, x, fail)
	case string:
		checkString(schema, x, fail)
	case []any:
		checkArray(schema, x, path, schemaPath, errs, fail)
	case map[string]any:
		checkObject(schema, x, path, schemaPath, errs, fail)
	}

	if raw, ok := schema["anyOf"]; ok {
		if members := schemaList(raw); members != nil {
			matched := false
			for _, m := range members {
				if len(CheckSchema(m, v)) == 0 {
					matched = true
					break
				}
			}
			if !matched {
				fail("anyOf", "must match a schema in anyOf")
			}
		}
	}
	if raw, ok := schema["oneOf"]; ok {
		if members := schemaList(raw); members != nil {
			matches := 0
			for _, m := range members {
				if len(CheckSchema(m, v)) == 0 {
					matches++
				}
			}
			if matches != 1 {
				fail("oneOf", "must match exactly one schema in oneOf")
			}
		}
	}
	if raw, ok := schema["allOf"]; ok {
		if members := schemaList(raw); members != nil {
			for i, m := range members {
				check(m, v, path, fmt.Sprintf("%s/allOf/%d", schemaPath, i), errs)
			}
		}
	}
	if raw, ok := schema["not"]; ok {
		if m := toSchemaObject(raw); m != nil && len(CheckSchema(m, v)) == 0 {
			fail("not", "must NOT be valid")
		}
	}
}

func checkNumber(schema JsonSchemaObject, x float64, fail func(string, string)) {
	if m, ok := numberKeyword(schema, "minimum"); ok && x < m {
		fail("minimum", "must be >= "+formatJSNumber(m))
	}
	if m, ok := numberKeyword(schema, "maximum"); ok && x > m {
		fail("maximum", "must be <= "+formatJSNumber(m))
	}
	if m, ok := numberKeyword(schema, "exclusiveMinimum"); ok && x <= m {
		fail("exclusiveMinimum", "must be > "+formatJSNumber(m))
	}
	if m, ok := numberKeyword(schema, "exclusiveMaximum"); ok && x >= m {
		fail("exclusiveMaximum", "must be < "+formatJSNumber(m))
	}
	if m, ok := numberKeyword(schema, "multipleOf"); ok && m != 0 {
		q := x / m
		if math.Abs(q-math.Round(q)) > 1e-9 {
			fail("multipleOf", "must be multiple of "+formatJSNumber(m))
		}
	}
}

func checkString(schema JsonSchemaObject, x string, fail func(string, string)) {
	length := utf16Length(x)
	if m, ok := numberKeyword(schema, "minLength"); ok && float64(length) < m {
		fail("minLength", fmt.Sprintf("must NOT have fewer than %s characters", formatJSNumber(m)))
	}
	if m, ok := numberKeyword(schema, "maxLength"); ok && float64(length) > m {
		fail("maxLength", fmt.Sprintf("must NOT have more than %s characters", formatJSNumber(m)))
	}
	if p, ok := schema["pattern"].(string); ok {
		re, err := compilePattern(p)
		if err != nil || !re.MatchString(x) {
			fail("pattern", fmt.Sprintf("must match pattern \"%s\"", p))
		}
	}
}

func checkArray(schema JsonSchemaObject, x []any, path, schemaPath string, errs *[]SchemaError, fail func(string, string)) {
	if m, ok := numberKeyword(schema, "minItems"); ok && float64(len(x)) < m {
		fail("minItems", fmt.Sprintf("must NOT have fewer than %s items", formatJSNumber(m)))
	}
	if m, ok := numberKeyword(schema, "maxItems"); ok && float64(len(x)) > m {
		fail("maxItems", fmt.Sprintf("must NOT have more than %s items", formatJSNumber(m)))
	}
	if unique, ok := schema["uniqueItems"].(bool); ok && unique {
		for i := range x {
			for j := range i {
				if DeepEqual(x[i], x[j]) {
					fail("uniqueItems", fmt.Sprintf("must NOT have duplicate items (items ## %d and %d are identical)", j, i))
					i = len(x)
					break
				}
			}
		}
	}
	if items := toSchemaObject(schema["items"]); items != nil {
		for i, item := range x {
			check(items, item, fmt.Sprintf("%s/%d", path, i), schemaPath+"/items", errs)
		}
	}
}

func checkObject(schema JsonSchemaObject, x map[string]any, path, schemaPath string, errs *[]SchemaError, fail func(string, string)) {
	props := propsOf(schema)
	for _, key := range requiredOf(schema) {
		if _, ok := x[key]; !ok {
			fail("required", fmt.Sprintf("must have required property '%s'", key))
		}
	}
	keys := make([]string, 0, len(x))
	for k := range x {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if sub, ok := props[key]; ok {
			check(sub, x[key], path+"/"+key, schemaPath+"/properties/"+key, errs)
			continue
		}
		switch ap := schema["additionalProperties"].(type) {
		case bool:
			if !ap {
				*errs = append(*errs, SchemaError{Keyword: "additionalProperties", InstancePath: path, SchemaPath: schemaPath + "/additionalProperties", Message: "must NOT have additional properties"})
			}
		default:
			if sub := toSchemaObject(ap); sub != nil {
				check(sub, x[key], path+"/"+key, schemaPath+"/additionalProperties", errs)
			}
		}
	}
	if m, ok := numberKeyword(schema, "minProperties"); ok && float64(len(x)) < m {
		fail("minProperties", fmt.Sprintf("must NOT have fewer than %s properties", formatJSNumber(m)))
	}
	if m, ok := numberKeyword(schema, "maxProperties"); ok && float64(len(x)) > m {
		fail("maxProperties", fmt.Sprintf("must NOT have more than %s properties", formatJSNumber(m)))
	}
}

func numberKeyword(schema JsonSchemaObject, keyword string) (float64, bool) {
	raw, ok := schema[keyword]
	if !ok {
		return 0, false
	}
	return asFloat(raw)
}

func schemaList(raw any) []JsonSchemaObject {
	var out []JsonSchemaObject
	switch list := raw.(type) {
	case []any:
		for _, m := range list {
			if s := toSchemaObject(m); s != nil {
				out = append(out, s)
			}
		}
	case []Schema:
		for _, m := range list {
			out = append(out, m)
		}
	case []map[string]any:
		for _, m := range list {
			out = append(out, Schema(m))
		}
	}
	return out
}

func typeLabel(t any) string {
	switch x := t.(type) {
	case string:
		return x
	case []string:
		return strings.Join(x, ",")
	case []any:
		parts := make([]string, 0, len(x))
		for _, s := range x {
			parts = append(parts, jsString(s))
		}
		return strings.Join(parts, ",")
	}
	return jsString(t)
}

func matchesType(t any, v any) bool {
	switch x := t.(type) {
	case string:
		return isType(x, v)
	case []string:
		for _, s := range x {
			if isType(s, v) {
				return true
			}
		}
		return false
	case []any:
		for _, s := range x {
			if name, ok := s.(string); ok && isType(name, v) {
				return true
			}
		}
		return false
	}
	return true
}

func isType(name string, v any) bool {
	switch name {
	case "null":
		return v == nil
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "string":
		_, ok := v.(string)
		return ok
	case "number":
		_, ok := v.(float64)
		return ok
	case "integer":
		f, ok := v.(float64)
		return ok && f == math.Trunc(f) && !math.IsInf(f, 0)
	case "array":
		_, ok := v.([]any)
		return ok
	case "object":
		_, ok := v.(map[string]any)
		return ok
	}
	return false
}

// normalize converts a Go value to the JSON value model the checker walks:
// numerics → float64, string-keyed maps → map[string]any, slices/arrays →
// []any, structs → map[string]any by json name, pointers dereferenced.
func normalize(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case bool, string, float64:
		return v
	case map[string]any:
		return v
	case []any:
		return v
	case Schema:
		return map[string]any(v)
	case []byte:
		return string(v)
	}
	if f, ok := asFloat(value); ok {
		return f
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			return nil
		}
		return normalize(rv.Elem().Interface())
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return value
		}
		out := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			out[iter.Key().String()] = normalize(iter.Value().Interface())
		}
		return out
	case reflect.Slice, reflect.Array:
		out := make([]any, rv.Len())
		for i := range rv.Len() {
			out[i] = normalize(rv.Index(i).Interface())
		}
		return out
	case reflect.Struct:
		out := map[string]any{}
		t := rv.Type()
		for i := range t.NumField() {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name, omitEmpty := f.Name, false
			if tag, ok := f.Tag.Lookup("json"); ok {
				tagName, rest, _ := strings.Cut(tag, ",")
				if tagName == "-" {
					continue
				}
				if tagName != "" {
					name = tagName
				}
				omitEmpty = strings.Contains(rest, "omitempty")
			}
			fv := rv.Field(i)
			if omitEmpty && fv.IsZero() {
				continue
			}
			out[name] = normalize(fv.Interface())
		}
		return out
	}
	return value
}

func utf16Length(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return n
}

var patternCache = struct {
	mu sync.Mutex
	m  map[string]*regexp.Regexp
}{m: map[string]*regexp.Regexp{}}

func compilePattern(p string) (*regexp.Regexp, error) {
	patternCache.mu.Lock()
	defer patternCache.mu.Unlock()
	if re, ok := patternCache.m[p]; ok {
		return re, nil
	}
	re, err := regexp.Compile(p)
	if err != nil {
		return nil, err
	}
	patternCache.m[p] = re
	return re, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
