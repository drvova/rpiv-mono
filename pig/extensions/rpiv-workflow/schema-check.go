package workflow

import (
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// CheckSchema — the validator behind every StageSchema this package builds.
// It reproduces TypeBox's Value.Check / Value.Errors (typebox@1.1.39, the
// copy the TypeScript implementation validated with) over the JSON Schema
// keywords TypeBox enforces: type (a name or an array of names), properties,
// required, additionalProperties (bool or schema), patternProperties,
// propertyNames, dependencies / dependentRequired / dependentSchemas,
// minProperties / maxProperties, items (schema, bool or tuple), prefixItems,
// contains (+ minContains / maxContains), minItems / maxItems, uniqueItems,
// minLength / maxLength (grapheme clusters — see schema-guards.go), format,
// pattern, minimum / maximum / exclusiveMinimum / exclusiveMaximum,
// multipleOf, const, enum, if / then / else, not, allOf / anyOf / oneOf and
// boolean schemas. Keyword evaluation order, messages (TypeBox's en_US
// locale), instance paths and schema paths all follow TypeBox, so a failure
// reads identically whichever implementation produced it. Unknown keywords
// and unknown `type` names constrain nothing. $ref and the unevaluated*
// keywords are not resolved (doc.go, Divergences).

// SchemaError is one validation failure in the shape TypeBox's Value.Errors
// yields: the failing keyword, the JSON pointer of the offending value (empty
// at the root), the JSON pointer of the schema whose keyword failed ("#" at
// the root) and the locale message.
type SchemaError struct {
	Keyword      string
	InstancePath string
	SchemaPath   string
	Message      string
}

// SchemaDefectError reports a SCHEMA that cannot be evaluated — today a
// `pattern` / `patternProperties` key that is not a valid regular expression.
// TypeBox throws a SyntaxError from inside Value.Check for the same defect,
// so CheckSchema returns it as an error instead of reporting the value as
// invalid: a broken schema surfaces as a thrown validator.
type SchemaDefectError struct {
	Message string
}

// Error implements error.
func (e *SchemaDefectError) Error() string { return e.Message }

// CheckSchema validates value against schema and returns every failure (nil
// when valid), or a *SchemaDefectError when the schema itself cannot be
// evaluated. Values are the shapes encoding/json produces (map[string]any,
// []any, float64, string, bool, nil) plus Go's native numerics, Schema maps,
// typed slices and structs, so authored data and decoded data validate
// identically. A nil schema accepts everything.
func CheckSchema(schema JsonSchemaObject, value any) ([]SchemaError, error) {
	c := &checker{}
	c.check(schema, value, "", "#")
	if c.defect != nil {
		return nil, c.defect
	}
	return c.errs, nil
}

// checker accumulates errors the way TypeBox's error context does; a sub
// checker is TypeBox's AccumulatedErrorContext (errors kept aside until the
// composite keyword decides whether to merge them).
type checker struct {
	errs   []SchemaError
	defect error
}

func (c *checker) add(keyword, instancePath, schemaPath, message string) {
	c.errs = append(c.errs, SchemaError{Keyword: keyword, InstancePath: instancePath, SchemaPath: schemaPath, Message: message})
}

// fault records a schema defect — the port of TypeBox's thrown SyntaxError.
func (c *checker) fault(err error) {
	if c.defect == nil {
		c.defect = err
	}
}

// capture validates value against raw in isolation and returns the errors
// for the caller to merge or discard; a defect propagates.
func (c *checker) capture(raw, value any, path, schemaPath string) []SchemaError {
	sub := &checker{}
	sub.check(raw, value, path, schemaPath)
	if sub.defect != nil {
		c.fault(sub.defect)
	}
	return sub.errs
}

// probe is capture reduced to validity.
func (c *checker) probe(raw, value any, path, schemaPath string) bool {
	return len(c.capture(raw, value, path, schemaPath)) == 0
}

// check evaluates one schema (a Schema map, a boolean schema or nil) against
// a value, in TypeBox's keyword order: type; the keywords gated on the
// value's kind (object, array, string, number); then const, enum, if, not,
// allOf, anyOf, oneOf. A failed type does not stop evaluation — the kind
// gates skip what does not apply.
func (c *checker) check(raw, value any, path, schemaPath string) {
	if b, ok := raw.(bool); ok {
		if !b {
			c.add("boolean", path, schemaPath, "schema is false")
		}
		return
	}
	schema := toSchemaObject(raw)
	if schema == nil {
		return
	}
	v := normalize(value)
	fail := func(keyword, message string) { c.add(keyword, path, schemaPath, message) }

	if t, ok := schema["type"]; ok && !matchesType(t, v) {
		fail("type", typeMessage(t))
	}
	switch x := v.(type) {
	case map[string]any:
		c.checkObject(schema, x, path, schemaPath, fail)
	case []any:
		c.checkArray(schema, x, path, schemaPath, fail)
	case string:
		c.checkString(schema, x, fail)
	case float64:
		if !math.IsNaN(x) && !math.IsInf(x, 0) {
			checkNumber(schema, x, fail)
		}
	}
	if cst, ok := schema["const"]; ok && !DeepEqual(normalize(cst), v) {
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
	if cond, ok := schema["if"]; ok {
		c.checkIf(schema, cond, v, path, schemaPath, fail)
	}
	if n, ok := schema["not"]; ok && c.probe(n, v, path, schemaPath+"/not") {
		fail("not", "must not be valid")
	}
	if members, ok := schemaList(schema["allOf"]); ok {
		for i, m := range members {
			c.check(m, v, path, schemaPath+"/allOf/"+strconv.Itoa(i))
		}
	}
	if members, ok := schemaList(schema["anyOf"]); ok {
		var failed []SchemaError
		matched := false
		for i, m := range members {
			errs := c.capture(m, v, path, schemaPath+"/anyOf/"+strconv.Itoa(i))
			if len(errs) == 0 {
				matched = true
			}
			failed = append(failed, errs...)
		}
		if !matched {
			c.errs = append(c.errs, failed...)
			fail("anyOf", "must match a schema in anyOf")
		}
	}
	if members, ok := schemaList(schema["oneOf"]); ok {
		var failed []SchemaError
		passing := 0
		for i, m := range members {
			errs := c.capture(m, v, path, schemaPath+"/oneOf/"+strconv.Itoa(i))
			if len(errs) == 0 {
				passing++
			}
			failed = append(failed, errs...)
		}
		if passing != 1 {
			if passing == 0 {
				c.errs = append(c.errs, failed...)
			}
			fail("oneOf", "must match exactly one schema in oneOf")
		}
	}
}

// checkIf: when `if` holds, `then` must hold (its own errors are discarded,
// only the `if` failure is reported); otherwise `else` must hold, and its
// errors are reported ahead of the `if` failure — TypeBox's asymmetry.
func (c *checker) checkIf(schema JsonSchemaObject, cond, v any, path, schemaPath string, fail func(string, string)) {
	if c.probe(cond, v, path, schemaPath+"/if") {
		if then, ok := schema["then"]; ok && !c.probe(then, v, path, schemaPath+"/then") {
			fail("if", `must match "then" schema`)
		}
		return
	}
	if els, ok := schema["else"]; ok {
		before := len(c.errs)
		c.check(els, v, path, schemaPath+"/else")
		if len(c.errs) > before {
			fail("if", `must match "else" schema`)
		}
	}
}

// checkObject evaluates the object keywords in TypeBox's order: required,
// additionalProperties, dependencies, dependentRequired, dependentSchemas,
// patternProperties, properties, propertyNames, minProperties,
// maxProperties. Go maps are unordered, so property and key walks run in
// sorted name order where TypeBox walks insertion order.
func (c *checker) checkObject(schema JsonSchemaObject, x map[string]any, path, schemaPath string, fail func(string, string)) {
	keys := sortedKeys(x)
	if required := requiredOf(schema); len(required) > 0 {
		var missing []string
		for _, key := range required {
			if _, ok := x[key]; !ok {
				missing = append(missing, key)
			}
		}
		if len(missing) > 0 {
			fail("required", "must have required properties "+strings.Join(missing, ", "))
		}
	}
	if ap, ok := schema["additionalProperties"]; ok {
		if allowed, isBool := ap.(bool); !isBool || !allowed {
			declared := c.declaredKey(schema)
			extra := false
			for _, key := range keys {
				if declared(key) {
					continue
				}
				if isBool || !c.probe(ap, x[key], path+"/"+key, schemaPath+"/additionalProperties") {
					extra = true
				}
			}
			if extra {
				fail("additionalProperties", "must not have additional properties")
			}
		}
	}
	if len(x) > 0 {
		c.checkDependencies(schema, "dependencies", x, path, schemaPath, fail)
		c.checkDependencies(schema, "dependentRequired", x, path, schemaPath, fail)
		c.checkDependencies(schema, "dependentSchemas", x, path, schemaPath, fail)
	}
	if pp := toSchemaObject(schema["patternProperties"]); pp != nil {
		for _, pattern := range sortedKeys(map[string]any(pp)) {
			re, err := compilePattern(pattern)
			if err != nil {
				c.fault(err)
				return
			}
			for _, key := range keys {
				if re.MatchString(key) {
					c.check(pp[pattern], x[key], path+"/"+key, schemaPath+"/patternProperties/"+pattern)
				}
			}
		}
	}
	if props := toSchemaObject(schema["properties"]); props != nil {
		for _, name := range sortedKeys(map[string]any(props)) {
			if val, ok := x[name]; ok {
				c.check(props[name], val, path+"/"+name, schemaPath+"/properties/"+name)
			}
		}
	}
	if pn, ok := schema["propertyNames"]; ok {
		var invalid []string
		for _, key := range keys {
			if !c.probe(pn, key, path+"/"+key, schemaPath+"/propertyNames") {
				invalid = append(invalid, key)
			}
		}
		if len(invalid) > 0 {
			fail("propertyNames", "property names "+strings.Join(invalid, ", ")+" are invalid")
		}
	}
	if m, ok := numberKeyword(schema, "minProperties"); ok && float64(len(x)) < m {
		fail("minProperties", "must not have fewer than "+formatJSNumber(m)+" properties")
	}
	if m, ok := numberKeyword(schema, "maxProperties"); ok && float64(len(x)) > m {
		fail("maxProperties", "must not have more than "+formatJSNumber(m)+" properties")
	}
}

// checkDependencies covers TypeBox's three spellings of "when key is
// present, require …": dependencies (array or schema entries),
// dependentRequired (array entries) and dependentSchemas (schema entries).
// An array entry reports one error per missing dependency, each listing the
// whole dependency list; a schema entry is evaluated against the object.
func (c *checker) checkDependencies(schema JsonSchemaObject, keyword string, x map[string]any, path, schemaPath string, fail func(string, string)) {
	deps := toSchemaObject(schema[keyword])
	if deps == nil {
		return
	}
	for _, key := range sortedKeys(map[string]any(deps)) {
		if _, present := x[key]; !present {
			continue
		}
		entry := deps[key]
		if names, ok := stringList(entry); ok {
			if keyword == "dependentSchemas" {
				continue
			}
			for _, dep := range names {
				if _, ok := x[dep]; !ok {
					fail(keyword, "must have properties "+strings.Join(names, ", ")+" when property "+key+" is present")
				}
			}
			continue
		}
		if keyword != "dependentRequired" {
			c.check(entry, x, path, schemaPath+"/"+keyword+"/"+key)
		}
	}
}

// declaredKey reports whether a key is accounted for by `properties` or by a
// `patternProperties` pattern — the keys additionalProperties leaves alone.
func (c *checker) declaredKey(schema JsonSchemaObject) func(string) bool {
	props := toSchemaObject(schema["properties"])
	var patterns []*regexp.Regexp
	if pp := toSchemaObject(schema["patternProperties"]); pp != nil {
		for _, pattern := range sortedKeys(map[string]any(pp)) {
			re, err := compilePattern(pattern)
			if err != nil {
				c.fault(err)
				continue
			}
			patterns = append(patterns, re)
		}
	}
	return func(key string) bool {
		if props != nil {
			if _, ok := props[key]; ok {
				return true
			}
		}
		for _, re := range patterns {
			if re.MatchString(key) {
				return true
			}
		}
		return false
	}
}

// checkArray evaluates the array keywords in TypeBox's order: contains,
// items, maxContains, maxItems, minContains, minItems, prefixItems,
// uniqueItems. A tuple-form `items` validates by index; a schema-form
// `items` validates the elements after any prefixItems.
func (c *checker) checkArray(schema JsonSchemaObject, x []any, path, schemaPath string, fail func(string, string)) {
	const containsMessage = "must contain at least 1 valid item"
	contains, hasContains := schema["contains"]
	containing := 0
	if hasContains {
		for i, item := range x {
			if c.probe(contains, item, path+"/"+strconv.Itoa(i), schemaPath+"/contains") {
				containing++
			}
		}
		if containing == 0 {
			fail("contains", containsMessage)
		}
	}
	if items, ok := schema["items"]; ok {
		if tuple, isTuple := schemaList(items); isTuple {
			for i, sub := range tuple {
				if i >= len(x) {
					break
				}
				c.check(sub, x[i], path+"/"+strconv.Itoa(i), schemaPath+"/items/"+strconv.Itoa(i))
			}
		} else {
			offset := 0
			if prefix, ok := schemaList(schema["prefixItems"]); ok {
				offset = len(prefix)
			}
			for i := offset; i < len(x); i++ {
				c.check(items, x[i], path+"/"+strconv.Itoa(i), schemaPath+"/items")
			}
		}
	}
	if m, ok := numberKeyword(schema, "maxContains"); ok && hasContains && float64(containing) > m {
		fail("contains", containsMessage)
	}
	if m, ok := numberKeyword(schema, "maxItems"); ok && float64(len(x)) > m {
		fail("maxItems", "must not have more than "+formatJSNumber(m)+" items")
	}
	if m, ok := numberKeyword(schema, "minContains"); ok && hasContains && float64(containing) < m {
		fail("contains", containsMessage)
	}
	if m, ok := numberKeyword(schema, "minItems"); ok && float64(len(x)) < m {
		fail("minItems", "must not have fewer than "+formatJSNumber(m)+" items")
	}
	if prefix, ok := schemaList(schema["prefixItems"]); ok && len(x) > 0 {
		for i, sub := range prefix {
			if i >= len(x) {
				break
			}
			c.check(sub, x[i], path+"/"+strconv.Itoa(i), schemaPath+"/prefixItems/"+strconv.Itoa(i))
		}
	}
	if unique, ok := schema["uniqueItems"].(bool); ok && unique && hasDuplicate(x) {
		fail("uniqueItems", "must not have duplicate items")
	}
}

func hasDuplicate(x []any) bool {
	for i := range x {
		for j := range i {
			if DeepEqual(x[i], x[j]) {
				return true
			}
		}
	}
	return false
}

// checkString evaluates maxLength, minLength, format, pattern — in that
// order. Lengths are TypeBox grapheme counts; an unknown format constrains
// nothing; a pattern that does not compile is a schema defect.
func (c *checker) checkString(schema JsonSchemaObject, x string, fail func(string, string)) {
	if m, ok := numberKeyword(schema, "maxLength"); ok && !isMaxLength(x, m) {
		fail("maxLength", "must not have more than "+formatJSNumber(m)+" characters")
	}
	if m, ok := numberKeyword(schema, "minLength"); ok && !isMinLength(x, m) {
		fail("minLength", "must not have fewer than "+formatJSNumber(m)+" characters")
	}
	if f, ok := schema["format"].(string); ok {
		if test, known := formatChecks[f]; known && !test(x) {
			fail("format", "must match format \""+f+"\"")
		}
	}
	if p, ok := schema["pattern"].(string); ok {
		re, err := compilePattern(p)
		if err != nil {
			c.fault(err)
			return
		}
		if !re.MatchString(x) {
			fail("pattern", "must match pattern \""+p+"\"")
		}
	}
}

// checkNumber evaluates exclusiveMaximum, exclusiveMinimum, maximum,
// minimum, multipleOf — in that order.
func checkNumber(schema JsonSchemaObject, x float64, fail func(string, string)) {
	if m, ok := numberKeyword(schema, "exclusiveMaximum"); ok && x >= m {
		fail("exclusiveMaximum", "must be < "+formatJSNumber(m))
	}
	if m, ok := numberKeyword(schema, "exclusiveMinimum"); ok && x <= m {
		fail("exclusiveMinimum", "must be > "+formatJSNumber(m))
	}
	if m, ok := numberKeyword(schema, "maximum"); ok && x > m {
		fail("maximum", "must be <= "+formatJSNumber(m))
	}
	if m, ok := numberKeyword(schema, "minimum"); ok && x < m {
		fail("minimum", "must be >= "+formatJSNumber(m))
	}
	if m, ok := numberKeyword(schema, "multipleOf"); ok && !isMultipleOf(x, m) {
		fail("multipleOf", "must be multiple of "+formatJSNumber(m))
	}
}

func numberKeyword(schema JsonSchemaObject, keyword string) (float64, bool) {
	raw, ok := schema[keyword]
	if !ok {
		return 0, false
	}
	return asFloat(raw)
}

// schemaList reads an array-valued keyword (allOf / anyOf / oneOf, tuple
// items, prefixItems) as its members — schemas or boolean schemas; false
// when the keyword is not an array.
func schemaList(raw any) ([]any, bool) {
	switch list := raw.(type) {
	case []any:
		return list, true
	case []Schema:
		out := make([]any, len(list))
		for i, m := range list {
			out[i] = m
		}
		return out, true
	case []map[string]any:
		out := make([]any, len(list))
		for i, m := range list {
			out[i] = Schema(m)
		}
		return out, true
	}
	return nil, false
}

// stringList reads an array of property names; false for anything else.
func stringList(raw any) ([]string, bool) {
	switch list := raw.(type) {
	case []string:
		return list, true
	case []any:
		out := make([]string, 0, len(list))
		for _, s := range list {
			name, ok := s.(string)
			if !ok {
				return nil, false
			}
			out = append(out, name)
		}
		return out, true
	}
	return nil, false
}

// typeMessage renders TypeBox's type failure: "must be string" for a name,
// "must be either string or null" for an array of names.
func typeMessage(t any) string {
	names, ok := stringList(t)
	if !ok {
		return "must be " + jsString(t)
	}
	return "must be either " + strings.Join(names, " or ")
}

func matchesType(t any, v any) bool {
	if name, ok := t.(string); ok {
		return isType(name, v)
	}
	names, ok := stringList(t)
	if !ok {
		return true
	}
	for _, name := range names {
		if isType(name, v) {
			return true
		}
	}
	return false
}

// xschemaTypeNames are TypeBox's non-JSON type names; no JSON value is one.
var xschemaTypeNames = map[string]bool{"asyncIterator": true, "bigint": true, "constructor": true, "function": true, "iterator": true, "symbol": true, "undefined": true, "void": true}

// isType is TypeBox's per-name guard: "number" means a finite number,
// "integer" a finite whole number, an unknown name constrains nothing.
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
		f, ok := v.(float64)
		return ok && !math.IsNaN(f) && !math.IsInf(f, 0)
	case "integer":
		f, ok := v.(float64)
		return ok && !math.IsNaN(f) && !math.IsInf(f, 0) && f == math.Trunc(f)
	case "array":
		_, ok := v.([]any)
		return ok
	case "object":
		_, ok := v.(map[string]any)
		return ok
	}
	return !xschemaTypeNames[name]
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

var patternCache = struct {
	mu sync.Mutex
	m  map[string]*regexp.Regexp
}{m: map[string]*regexp.Regexp{}}

// compilePattern compiles a schema pattern (TypeBox compiles with the `u`
// flag; RE2 is Unicode-aware by construction). A pattern that does not
// compile is a *SchemaDefectError whose text mirrors V8's SyntaxError.
func compilePattern(p string) (*regexp.Regexp, error) {
	patternCache.mu.Lock()
	defer patternCache.mu.Unlock()
	if re, ok := patternCache.m[p]; ok {
		return re, nil
	}
	re, err := regexp.Compile(p)
	if err != nil {
		return nil, &SchemaDefectError{Message: "Invalid regular expression: /" + p + "/u: " + err.Error()}
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
