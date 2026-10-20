package workflow

import (
	"fmt"
	"sort"
	"strings"
)

// SchemaCompatResult is the result of a conservative structural
// compatibility check between a producer's output schema and a consumer's
// input schema. Reason is set only when OK is false.
type SchemaCompatResult struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

// typeSet is the set of JSON-Schema types a schema allows (string or
// []string), or nil if untyped.
func typeSet(schema JsonSchemaObject) map[string]bool {
	switch t := schema["type"].(type) {
	case string:
		return map[string]bool{t: true}
	case []string:
		set := map[string]bool{}
		for _, x := range t {
			set[x] = true
		}
		return set
	case []any:
		set := map[string]bool{}
		for _, x := range t {
			s, ok := x.(string)
			if !ok {
				return nil
			}
			set[s] = true
		}
		return set
	}
	return nil
}

func disjoint(a, b map[string]bool) bool {
	for x := range a {
		if b[x] {
			return false
		}
	}
	return true
}

// widenNumeric treats integer as a subtype of number so the two never read
// as disjoint.
func widenNumeric(s map[string]bool) map[string]bool {
	if !s["integer"] {
		return s
	}
	out := map[string]bool{"number": true}
	for x := range s {
		out[x] = true
	}
	return out
}

// orderedTypes renders a type set in declaration order for a reason string.
func orderedTypes(schema JsonSchemaObject, set map[string]bool) string {
	switch t := schema["type"].(type) {
	case string:
		return t
	case []string:
		return strings.Join(t, "|")
	case []any:
		parts := make([]string, 0, len(t))
		for _, x := range t {
			parts = append(parts, x.(string))
		}
		return strings.Join(parts, "|")
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, "|")
}

func propsOf(schema JsonSchemaObject) map[string]JsonSchemaObject {
	out := map[string]JsonSchemaObject{}
	switch props := schema["properties"].(type) {
	case map[string]any:
		for k, v := range props {
			if sub := toSchemaObject(v); sub != nil {
				out[k] = sub
			}
		}
	case map[string]Schema:
		for k, v := range props {
			out[k] = v
		}
	case Schema:
		for k, v := range props {
			if sub := toSchemaObject(v); sub != nil {
				out[k] = sub
			}
		}
	}
	return out
}

func requiredOf(schema JsonSchemaObject) []string {
	switch r := schema["required"].(type) {
	case []string:
		return r
	case []any:
		out := make([]string, 0, len(r))
		for _, x := range r {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// Rule 1 — a producer that CLOSES its object (additionalProperties: false)
// and omits a field the consumer requires provably never emits it.
func closedProducerOmitsRequired(producer JsonSchemaObject, producerProps map[string]JsonSchemaObject, required []string) *SchemaCompatResult {
	if closed, ok := producer["additionalProperties"].(bool); !ok || closed {
		return nil
	}
	for _, key := range required {
		if _, ok := producerProps[key]; !ok {
			return &SchemaCompatResult{OK: false, Reason: fmt.Sprintf("consumer requires %q but producer (closed object) never emits it", key)}
		}
	}
	return nil
}

// Rule 2 — producer and consumer pin disjoint type sets for the shared field.
func fieldTypeConflict(key string, p, c JsonSchemaObject) *SchemaCompatResult {
	pt, ct := typeSet(p), typeSet(c)
	if pt != nil && ct != nil && disjoint(widenNumeric(pt), widenNumeric(ct)) {
		return &SchemaCompatResult{OK: false, Reason: fmt.Sprintf("field %q: producer emits %s but consumer expects %s", key, orderedTypes(p, pt), orderedTypes(c, ct))}
	}
	return nil
}

func enumKeys(schema JsonSchemaObject) (map[string]bool, bool) {
	raw, ok := schema["enum"]
	if !ok {
		return nil, false
	}
	var values []any
	switch e := raw.(type) {
	case []any:
		values = e
	case []string:
		for _, s := range e {
			values = append(values, s)
		}
	default:
		return nil, false
	}
	set := map[string]bool{}
	for _, v := range values {
		set[jsonStringify(v)] = true
	}
	return set, true
}

// Rule 3 — both restrict to an enum and the two enums are disjoint.
func fieldEnumConflict(key string, p, c JsonSchemaObject) *SchemaCompatResult {
	pe, pok := enumKeys(p)
	ce, cok := enumKeys(c)
	if !pok || !cok {
		return nil
	}
	if disjoint(pe, ce) {
		return &SchemaCompatResult{OK: false, Reason: fmt.Sprintf("field %q: producer enum is disjoint from consumer enum", key)}
	}
	return nil
}

// Rule 4 — both pin a const and the two constants differ (compared with
// DeepEqual, key-order-independent, to match the registry collision check).
func fieldConstConflict(key string, p, c JsonSchemaObject) *SchemaCompatResult {
	pc, pok := p["const"]
	cc, cok := c["const"]
	if pok && cok && !DeepEqual(pc, cc) {
		return &SchemaCompatResult{OK: false, Reason: fmt.Sprintf("field %q: producer const %s ≠ consumer const %s", key, jsonStringify(pc), jsonStringify(cc))}
	}
	return nil
}

// IsSchemaCompatible is the conservative structural compatibility of a
// producer's output schema against a consumer's input schema. Deliberately
// NOT a full subtype engine — it only flags DEFINITE mismatches. For each
// field present in BOTH properties it reports a conflict when: (1) their
// allowed type sets are disjoint (integer counts as number); (2) both restrict
// to an enum and the two enums are disjoint; (3) both pin a const and the
// constants differ. Plus: a closed producer object that omits a field the
// consumer requires. Everything else returns ok. Non-object roots compare
// their root type when both declare one.
func IsSchemaCompatible(producer, consumer JsonSchemaObject) SchemaCompatResult {
	pType, _ := producer["type"].(string)
	cType, _ := consumer["type"].(string)
	if pType != "object" || cType != "object" {
		if pType != "" && cType != "" && pType != cType {
			return SchemaCompatResult{OK: false, Reason: fmt.Sprintf("root type mismatch: producer is %q, consumer is %q", pType, cType)}
		}
		return SchemaCompatResult{OK: true}
	}
	producerProps := propsOf(producer)
	consumerProps := propsOf(consumer)
	required := requiredOf(consumer)

	if omission := closedProducerOmitsRequired(producer, producerProps, required); omission != nil {
		return *omission
	}
	keys := make([]string, 0, len(consumerProps))
	for k := range consumerProps {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		p, pok := producerProps[key]
		c := consumerProps[key]
		if !pok {
			continue
		}
		conflict := fieldTypeConflict(key, p, c)
		if conflict == nil {
			conflict = fieldEnumConflict(key, p, c)
		}
		if conflict == nil {
			conflict = fieldConstConflict(key, p, c)
		}
		if conflict != nil {
			return *conflict
		}
	}
	return SchemaCompatResult{OK: true}
}
