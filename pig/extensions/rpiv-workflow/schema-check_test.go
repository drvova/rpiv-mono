package workflow

import (
	"encoding/json"
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
		{"required property missing", T.Object(map[string]Schema{"name": T.String()}), map[string]any{}, "required@"},
		{"optional property may be missing", T.Object(map[string]Schema{"name": T.Optional(T.String())}), map[string]any{}, ""},
		{"nested property path", T.Object(map[string]Schema{"a": T.Object(map[string]Schema{"b": T.Number()})}), map[string]any{"a": map[string]any{"b": "x"}}, "type@/a/b"},
		{"additionalProperties false", Schema{"type": "object", "properties": map[string]any{}, "additionalProperties": false}, map[string]any{"x": 1}, "additionalProperties@"},
		{"additionalProperties schema", T.Record(T.Number()), map[string]any{"x": "s"}, "type@/x"},
		{"array items", T.Array(T.String()), []any{"a", 1}, "type@/1"},
		{"typed slice normalises", T.Array(T.Integer()), []int{1, 2}, ""},
		{"minItems", T.Array(T.String(), Schema{"minItems": 1}), []any{}, "minItems@"},
		{"maxItems", T.Array(T.String(), Schema{"maxItems": 1}), []any{"a", "b"}, "maxItems@"},
		{"uniqueItems", T.Array(T.Number(), Schema{"uniqueItems": true}), []any{1, 1}, "uniqueItems@"},
		{"enum hit", T.Enum([]any{"a", "b"}), "a", ""},
		{"enum miss", T.Enum([]any{"a", "b"}), "c", "enum@"},
		{"const hit", T.Literal("fast"), "fast", ""},
		{"const miss", T.Literal("fast"), "slow", "const@"},
		{"const object compares structurally", Schema{"const": map[string]any{"a": 1}}, map[string]any{"a": 1.0}, ""},
		{"anyOf hit", T.Union([]Schema{T.String(), T.Number()}), 1, ""},
		{"anyOf miss", T.Union([]Schema{T.String(), T.Number()}), true, "anyOf@"},
		{"oneOf exactly one", Schema{"oneOf": []any{T.Number(), T.Integer()}}, 1.5, ""},
		{"oneOf ambiguous", Schema{"oneOf": []any{T.Number(), T.Integer()}}, 1, "oneOf@"},
		{"allOf", Schema{"allOf": []any{T.String(), Schema{"minLength": 2}}}, "a", "minLength@"},
		{"not", Schema{"not": T.String()}, "a", "not@"},
		{"minimum", T.Integer(Schema{"minimum": 0}), -1, "minimum@"},
		{"maximum", T.Integer(Schema{"maximum": 3}), 4, "maximum@"},
		{"exclusiveMinimum", T.Number(Schema{"exclusiveMinimum": 0}), 0, "exclusiveMinimum@"},
		{"exclusiveMaximum", T.Number(Schema{"exclusiveMaximum": 1}), 1, "exclusiveMaximum@"},
		{"multipleOf", T.Number(Schema{"multipleOf": 0.5}), 1.25, "multipleOf@"},
		{"minLength counts UTF-16 units", T.String(Schema{"minLength": 2}), "😀", ""},
		{"maxLength", T.String(Schema{"maxLength": 1}), "ab", "maxLength@"},
		{"pattern hit", T.String(Schema{"pattern": "^a+$"}), "aaa", ""},
		{"pattern miss", T.String(Schema{"pattern": "^a+$"}), "b", "pattern@"},
		{"minProperties", Schema{"type": "object", "minProperties": 1}, map[string]any{}, "minProperties@"},
		{"maxProperties", Schema{"type": "object", "maxProperties": 1}, map[string]any{"a": 1, "b": 2}, "maxProperties@"},
		{"empty schema accepts anything", T.Any(), []any{1}, ""},
		{"nil schema accepts anything", nil, 1, ""},
		{"struct values validate by json name", T.Object(map[string]Schema{"pass": T.Boolean(), "votes": T.Object(map[string]Schema{"pass": T.Integer()})}), PanelVerdict{Pass: true}, ""},
		{"pointer to struct", T.Object(map[string]Schema{"stage": T.String()}), &OutputMeta{Stage: "s"}, ""},
		{"multiple failures all reported", T.Object(map[string]Schema{"a": T.String(), "b": T.Number()}), map[string]any{"a": 1, "b": "x"}, "type@/a type@/b"},
		{"unknown keywords ignored", Schema{"type": "string", "format": "email", "$comment": "x"}, "not-an-email", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := keywords(CheckSchema(c.schema, c.value)); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestCheckSchemaMessagesAndDecodedJSON(t *testing.T) {
	var decoded any
	if err := json.Unmarshal([]byte(`{"name": 42, "tags": ["a", 2]}`), &decoded); err != nil {
		t.Fatal(err)
	}
	schema := T.Object(map[string]Schema{"name": T.String(), "tags": T.Array(T.String()), "id": T.Integer()})
	errs := CheckSchema(schema, decoded)
	if keywords(errs) != "required@ type@/name type@/tags/1" {
		t.Fatalf("%s", keywords(errs))
	}
	if errs[0].Message != "must have required property 'id'" || errs[1].Message != "must be string" || errs[1].SchemaPath != "#/properties/name/type" {
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
	})
}
