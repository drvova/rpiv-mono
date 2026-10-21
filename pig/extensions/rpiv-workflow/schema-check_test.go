package workflow

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func keywords(errs []SchemaError) string {
	parts := make([]string, len(errs))
	for i, e := range errs {
		parts[i] = e.Keyword + "@" + e.InstancePath
	}
	return strings.Join(parts, " ")
}

func mustCheck(t *testing.T, schema Schema, value any) []SchemaError {
	t.Helper()
	errs, err := CheckSchema(schema, value)
	if err != nil {
		t.Fatalf("schema defect: %v", err)
	}
	return errs
}

func TestCheckSchema(t *testing.T) {
	cases := []struct {
		name   string
		schema Schema
		value  any
		want   string
	}{
		{"string ok", T.String(), "x", ""},
		{"string type mismatch", T.String(), 1, "type@"},
		{"number accepts ints and floats", T.Number(), 1, ""},
		{"integer rejects fractions", T.Integer(), 1.5, "type@"},
		{"integer accepts float64 whole numbers", T.Integer(), 2.0, ""},
		{"boolean", T.Boolean(), true, ""},
		{"null", T.Null(), nil, ""},
		{"nullable via type array", Schema{"type": []string{"string", "null"}}, nil, ""},
		{"nullable via type array rejects number", Schema{"type": []any{"string", "null"}}, 1, "type@"},
		{"unknown type name constrains nothing", Schema{"type": "strng"}, "x", ""},
		{"required property missing", T.Object(map[string]Schema{"name": T.String()}), map[string]any{}, "required@"},
		{"optional property may be missing", T.Object(map[string]Schema{"name": T.Optional(T.String())}), map[string]any{}, ""},
		{"nested property path", T.Object(map[string]Schema{"a": T.Object(map[string]Schema{"b": T.Number()})}), map[string]any{"a": map[string]any{"b": "x"}}, "type@/a/b"},
		{"additionalProperties false", Schema{"type": "object", "properties": map[string]any{}, "additionalProperties": false}, map[string]any{"x": 1}, "additionalProperties@"},
		{"additionalProperties schema reports the object", Schema{"type": "object", "additionalProperties": T.Number()}, map[string]any{"x": "s"}, "additionalProperties@"},
		{"record (patternProperties) reports the value", T.Record(T.Number()), map[string]any{"x": "s"}, "type@/x"},
		{"patternProperties exempts additionalProperties", Schema{"type": "object", "patternProperties": map[string]any{"^x": T.Number()}, "additionalProperties": false}, map[string]any{"x1": 1}, ""},
		{"propertyNames", Schema{"type": "object", "propertyNames": Schema{"pattern": "^a"}}, map[string]any{"b": 1}, "propertyNames@"},
		{"dependentRequired", Schema{"type": "object", "dependentRequired": map[string]any{"a": []any{"b"}}}, map[string]any{"a": 1}, "dependentRequired@"},
		{"array items", T.Array(T.String()), []any{"a", 1}, "type@/1"},
		{"tuple items", Schema{"type": "array", "items": []any{T.String(), T.Number()}}, []any{"a", "b"}, "type@/1"},
		{"prefixItems", Schema{"type": "array", "prefixItems": []any{T.String(), T.Number()}}, []any{"a", "b"}, "type@/1"},
		{"contains", Schema{"type": "array", "contains": T.String()}, []any{1, 2}, "contains@"},
		{"typed slice normalises", T.Array(T.Integer()), []int{1, 2}, ""},
		{"minItems", T.Array(T.String(), Schema{"minItems": 1}), []any{}, "minItems@"},
		{"maxItems", T.Array(T.String(), Schema{"maxItems": 1}), []any{"a", "b"}, "maxItems@"},
		{"uniqueItems", T.Array(T.Number(), Schema{"uniqueItems": true}), []any{1, 1}, "uniqueItems@"},
		{"enum hit", T.Enum([]any{"a", "b"}), "a", ""},
		{"enum miss", T.Enum([]any{"a", "b"}), "c", "enum@"},
		{"const hit", T.Literal("fast"), "fast", ""},
		{"const miss", T.Literal("fast"), "slow", "const@"},
		{"literal of another type fails type and const", T.Literal("fast"), 1, "type@ const@"},
		{"const object compares structurally", Schema{"const": map[string]any{"a": 1}}, map[string]any{"a": 1.0}, ""},
		{"anyOf hit", T.Union([]Schema{T.String(), T.Number()}), 1, ""},
		{"anyOf miss lists each member's failure", T.Union([]Schema{T.String(), T.Number()}), true, "type@ type@ anyOf@"},
		{"oneOf exactly one", Schema{"oneOf": []any{T.Number(), T.Integer()}}, 1.5, ""},
		{"oneOf ambiguous", Schema{"oneOf": []any{T.Number(), T.Integer()}}, 1, "oneOf@"},
		{"allOf", Schema{"allOf": []any{T.String(), Schema{"minLength": 2}}}, "a", "minLength@"},
		{"not", Schema{"not": T.String()}, "a", "not@"},
		{"if then", Schema{"if": T.String(), "then": Schema{"minLength": 2}}, "a", "if@"},
		{"if else reports the else failure first", Schema{"if": T.String(), "else": Schema{"minimum": 5}}, 1, "minimum@ if@"},
		{"minimum", T.Integer(Schema{"minimum": 0}), -1, "minimum@"},
		{"maximum", T.Integer(Schema{"maximum": 3}), 4, "maximum@"},
		{"exclusiveMinimum", T.Number(Schema{"exclusiveMinimum": 0}), 0, "exclusiveMinimum@"},
		{"exclusiveMaximum", T.Number(Schema{"exclusiveMaximum": 1}), 1, "exclusiveMaximum@"},
		{"multipleOf", T.Number(Schema{"multipleOf": 0.5}), 1.25, "multipleOf@"},
		{"multipleOf tolerates 0.3 of 0.1", T.Number(Schema{"multipleOf": 0.1}), 0.3, ""},
		{"minLength counts an emoji once", T.String(Schema{"minLength": 2}), "😀", "minLength@"},
		{"maxLength counts an emoji once", T.String(Schema{"maxLength": 1}), "😀", ""},
		{"maxLength counts a ZWJ family once", T.String(Schema{"maxLength": 1}), "👨‍👩‍👧", ""},
		{"maxLength", T.String(Schema{"maxLength": 1}), "ab", "maxLength@"},
		{"pattern hit", T.String(Schema{"pattern": "^a+$"}), "aaa", ""},
		{"pattern miss", T.String(Schema{"pattern": "^a+$"}), "b", "pattern@"},
		{"format email", T.String(Schema{"format": "email"}), "not-an-email", "format@"},
		{"unknown format constrains nothing", T.String(Schema{"format": "custom-thing"}), "x", ""},
		{"minProperties", Schema{"type": "object", "minProperties": 1}, map[string]any{}, "minProperties@"},
		{"maxProperties", Schema{"type": "object", "maxProperties": 1}, map[string]any{"a": 1, "b": 2}, "maxProperties@"},
		{"empty schema accepts anything", T.Any(), []any{1}, ""},
		{"nil schema accepts anything", nil, 1, ""},
		{"struct values validate by json name", T.Object(map[string]Schema{"pass": T.Boolean(), "votes": T.Object(map[string]Schema{"pass": T.Integer()})}), PanelVerdict{Pass: true}, ""},
		{"pointer to struct", T.Object(map[string]Schema{"stage": T.String()}), &OutputMeta{Stage: "s"}, ""},
		{"multiple failures all reported", T.Object(map[string]Schema{"a": T.String(), "b": T.Number()}), map[string]any{"a": 1, "b": "x"}, "type@/a type@/b"},
		{"properties walk in sorted name order", T.Object(map[string]Schema{"b": T.String(), "a": T.Number()}), map[string]any{"b": 1, "a": "x"}, "type@/a type@/b"},
		{"unknown keywords ignored", Schema{"type": "string", "$comment": "x", "x-custom": 1}, "anything", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := keywords(mustCheck(t, c.schema, c.value)); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestCheckSchemaDefects(t *testing.T) {
	cases := []struct {
		name   string
		schema Schema
	}{
		{"pattern that does not compile", T.String(Schema{"pattern": "("})},
		{"lookahead is not RE2", T.String(Schema{"pattern": "^(?=.*a).*$"})},
		{"patternProperties that does not compile", Schema{"type": "object", "patternProperties": map[string]any{"(": T.Number()}}},
		{"patternProperties behind additionalProperties", Schema{"type": "object", "patternProperties": map[string]any{"(": T.Number()}, "additionalProperties": false}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			value := any("banana")
			if c.schema["type"] == "object" {
				value = map[string]any{"x": 1}
			}
			errs, err := CheckSchema(c.schema, value)
			var defect *SchemaDefectError
			if !errors.As(err, &defect) || errs != nil || !strings.HasPrefix(defect.Message, "Invalid regular expression: /") {
				t.Fatalf("errs=%v err=%v", errs, err)
			}
		})
	}
	t.Run("a defect is the validator throwing, not an issue", func(t *testing.T) {
		_, err := TypeboxSchema(T.String(Schema{"pattern": "("})).Validate(t.Context(), "x")
		var defect *SchemaDefectError
		if !errors.As(err, &defect) {
			t.Fatalf("%v", err)
		}
	})
}

func TestCheckSchemaMessagesAndDecodedJSON(t *testing.T) {
	var decoded any
	if err := json.Unmarshal([]byte(`{"name": 42, "tags": ["a", 2]}`), &decoded); err != nil {
		t.Fatal(err)
	}
	schema := T.Object(map[string]Schema{"name": T.String(), "tags": T.Array(T.String()), "id": T.Integer()})
	errs := mustCheck(t, schema, decoded)
	if keywords(errs) != "required@ type@/name type@/tags/1" {
		t.Fatalf("%s", keywords(errs))
	}
	if errs[0].Message != "must have required properties id" || errs[1].Message != "must be string" || errs[1].SchemaPath != "#/properties/name" || errs[2].SchemaPath != "#/properties/tags/items" {
		t.Fatalf("%+v", errs)
	}
	t.Run("builder shapes", func(t *testing.T) {
		o := T.Object(map[string]Schema{"b": T.Optional(T.String()), "a": T.Integer(Schema{"minimum": 0})}, Schema{"additionalProperties": false})
		raw, _ := json.Marshal(o)
		if string(raw) != `{"additionalProperties":false,"properties":{"a":{"minimum":0,"type":"integer"},"b":{"type":"string"}},"required":["a"],"type":"object"}` {
			t.Fatalf("%s", raw)
		}
		if _, leaked := o["properties"].(map[string]any)["b"].(Schema)[optionalMarker]; leaked {
			t.Fatal("optional marker leaked")
		}
		shapes := map[string]string{
			`{"properties":{"a":{"type":"string"}},"type":"object"}`:           jsonStringify(T.Object(map[string]Schema{"a": T.Optional(T.String())})),
			`{"items":{"type":"string"},"type":"array"}`:                       jsonStringify(T.Array(T.Optional(T.String()))),
			`{"anyOf":[{"type":"string"},{"type":"number"}]}`:                  jsonStringify(T.Union([]Schema{T.Optional(T.String()), T.Number()})),
			`{"patternProperties":{"^.*$":{"type":"number"}},"type":"object"}`: jsonStringify(T.Record(T.Optional(T.Number()))),
			`{"const":"fast","type":"string"}`:                                 jsonStringify(T.Literal("fast")),
			`{"const":1,"type":"number"}`:                                      jsonStringify(T.Literal(1)),
			`{"const":true,"type":"boolean"}`:                                  jsonStringify(T.Literal(true)),
			`{"const":null}`:                                                   jsonStringify(T.Literal(nil)),
			`{"enum":["a","b"]}`:                                               jsonStringify(T.Enum([]any{"a", "b"})),
			`{}`:                                                               jsonStringify(T.Any()),
			`{"minProperties":1,"patternProperties":{"^.*$":{"type":"number"}},"type":"object"}`: jsonStringify(T.Record(T.Number(), Schema{"minProperties": 1})),
		}
		for want, got := range shapes {
			if want != got {
				t.Errorf("got  %s\nwant %s", got, want)
			}
		}
		if jsonStringify(PanelVerdictSchema) != `{"properties":{"agreement":{"type":"number"},"pass":{"type":"boolean"},"tie":{"type":"boolean"},"votes":{"properties":{"fail":{"type":"integer"},"pass":{"type":"integer"}},"required":["pass","fail"],"type":"object"}},"required":["pass","votes","agreement","tie"],"type":"object"}` {
			t.Fatalf("%s", jsonStringify(PanelVerdictSchema))
		}
	})
}
