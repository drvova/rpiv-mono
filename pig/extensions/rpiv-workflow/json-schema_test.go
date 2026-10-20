package workflow

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestIsJsonSchemaObject(t *testing.T) {
	for _, v := range []any{Schema{"type": "string"}, map[string]any{}, Schema{}} {
		if _, ok := IsJsonSchemaObject(v); !ok {
			t.Fatalf("%v should be accepted", v)
		}
	}
	for _, v := range []any{nil, "string", 42, true, []any{Schema{"type": "string"}}, Schema(nil)} {
		if _, ok := IsJsonSchemaObject(v); ok {
			t.Fatalf("%v should be rejected", v)
		}
	}
}

func TestJsonSchemaConverter(t *testing.T) {
	schema := Schema{"type": "object", "properties": map[string]any{"name": Schema{"type": "string"}}}
	c := JsonSchemaConverter(schema)
	for _, target := range []JSONSchemaTarget{TargetDraft2020, TargetDraft07} {
		out, err := c.Output(JSONSchemaOptions{Target: target})
		if err != nil || !reflect.DeepEqual(out, schema) {
			t.Fatalf("%s: %v %v", target, out, err)
		}
	}
	if _, err := c.Output(JSONSchemaOptions{Target: "draft-04"}); err == nil || err.Error() != `jsonSchema: unsupported target "draft-04" (supported: draft-2020-12, draft-07)` {
		t.Fatalf("%v", err)
	}
	if in, err := c.Input(JSONSchemaOptions{Target: TargetDraft2020}); err != nil || !reflect.DeepEqual(in, schema) {
		t.Fatal("input converter")
	}
}

func TestJsonSchemaToStandard(t *testing.T) {
	schema := Schema{"type": "object", "properties": map[string]any{"name": Schema{"type": "string"}}, "required": []string{"name"}}
	std := JsonSchemaToStandard(schema)
	ctx := context.Background()
	t.Run("validates valid data", func(t *testing.T) {
		r, err := std.Validate(ctx, map[string]any{"name": "Alice"})
		if err != nil || !r.OK() || !reflect.DeepEqual(r.Value, map[string]any{"name": "Alice"}) {
			t.Fatalf("%+v %v", r, err)
		}
	})
	t.Run("reports issues for invalid data", func(t *testing.T) {
		r, _ := std.Validate(ctx, map[string]any{"name": 42})
		if r.OK() || len(r.Issues) == 0 || r.Issues[0].Message == "" {
			t.Fatalf("%+v", r)
		}
	})
	t.Run("carries the converter", func(t *testing.T) {
		if !HasJsonSchema(std) {
			t.Fatal("converter")
		}
		out, ok := ExtractJsonSchema(std, "")
		if !ok || !reflect.DeepEqual(out, schema) {
			t.Fatal("extract")
		}
	})
	t.Run("nested failures carry a path", func(t *testing.T) {
		nested := Schema{"type": "object", "properties": map[string]any{"address": Schema{"type": "object", "properties": map[string]any{"zip": Schema{"type": "number"}}, "required": []string{"zip"}}}, "required": []string{"address"}}
		r, _ := JsonSchemaToStandard(nested).Validate(ctx, map[string]any{"address": map[string]any{"zip": "not-a-number"}})
		if r.OK() {
			t.Fatal("expected failure")
		}
		found := false
		for _, i := range r.Issues {
			if strings.Join(i.Path, "/") == "address/zip" {
				found = true
			}
		}
		if !found {
			t.Fatalf("%+v", r.Issues)
		}
	})
}

type validateOnlyStub struct{}

func (validateOnlyStub) Validate(_ context.Context, v any) (StandardResult, error) {
	return StandardResult{Value: v}, nil
}

func TestHasAndExtractJsonSchema(t *testing.T) {
	if !HasJsonSchema(TypeboxSchema(T.Object(map[string]Schema{"name": T.String()}))) {
		t.Fatal("typebox-wrapped")
	}
	if !HasJsonSchema(JsonSchemaToStandard(Schema{"type": "string"})) {
		t.Fatal("raw-wrapped")
	}
	if HasJsonSchema(validateOnlyStub{}) || HasJsonSchema(nil) {
		t.Fatal("validate-only / nil")
	}
	extracted, ok := ExtractJsonSchema(TypeboxSchema(T.Object(map[string]Schema{"name": T.String()})), "")
	if !ok || extracted["type"] != "object" || extracted["properties"] == nil {
		t.Fatalf("%v", extracted)
	}
	raw := Schema{"type": "string", "minLength": 1}
	if out, ok := ExtractJsonSchema(JsonSchemaToStandard(raw), ""); !ok || !reflect.DeepEqual(out, raw) {
		t.Fatal("round trip")
	}
	if _, ok := ExtractJsonSchema(validateOnlyStub{}, ""); ok {
		t.Fatal("stub")
	}
	if _, ok := ExtractJsonSchema(nil, ""); ok {
		t.Fatal("nil")
	}
	if _, ok := ExtractJsonSchema(JsonSchemaToStandard(Schema{"type": "string"}), "draft-04"); ok {
		t.Fatal("converter error should degrade")
	}
}

func TestValidateToJsonSchemaIssues(t *testing.T) {
	schema := Schema{"type": "object", "properties": map[string]any{"name": Schema{"type": "string"}}, "required": []string{"name"}}
	if !ValidateToJsonSchemaIssues(schema, map[string]any{"name": "Alice"}).OK() {
		t.Fatal("valid")
	}
	r := ValidateToJsonSchemaIssues(schema, map[string]any{"name": 42})
	if r.OK() || r.Issues[0].Message == "" {
		t.Fatalf("%+v", r)
	}
	nested := Schema{"type": "object", "properties": map[string]any{"address": Schema{"type": "object", "properties": map[string]any{"zip": Schema{"type": "number"}}, "required": []string{"zip"}}}, "required": []string{"address"}}
	r = ValidateToJsonSchemaIssues(nested, map[string]any{"address": map[string]any{"zip": "x"}})
	if r.OK() || len(r.Issues[0].Path) == 0 {
		t.Fatalf("%+v", r)
	}
	t.Run("typebox round-trip validates the same values", func(t *testing.T) {
		extracted, _ := ExtractJsonSchema(TypeboxSchema(T.Object(map[string]Schema{"count": T.Number()})), "")
		rewrapped := JsonSchemaToStandard(extracted)
		if ok, _ := rewrapped.Validate(context.Background(), map[string]any{"count": 42}); !ok.OK() {
			t.Fatal("valid")
		}
		if bad, _ := rewrapped.Validate(context.Background(), map[string]any{"count": "x"}); bad.OK() {
			t.Fatal("invalid")
		}
	})
}

func TestIsSchemaCompatible(t *testing.T) {
	obj := func(props map[string]any, extra ...Schema) Schema {
		s := Schema{"type": "object", "properties": props}
		for _, e := range extra {
			for k, v := range e {
				s[k] = v
			}
		}
		return s
	}
	cases := []struct {
		name     string
		producer Schema
		consumer Schema
		ok       bool
		reason   string
	}{
		{"identical object schemas", obj(map[string]any{"name": Schema{"type": "string"}}), obj(map[string]any{"name": Schema{"type": "string"}}), true, ""},
		{"disjoint type sets", obj(map[string]any{"a": Schema{"type": "string"}}), obj(map[string]any{"a": Schema{"type": "number"}}), false, `field "a": producer emits string but consumer expects number`},
		{"integer vs number is fine", obj(map[string]any{"count": Schema{"type": "integer"}}), obj(map[string]any{"count": Schema{"type": "number"}}), true, ""},
		{"number vs integer is fine", obj(map[string]any{"count": Schema{"type": "number"}}), obj(map[string]any{"count": Schema{"type": "integer"}}), true, ""},
		{"overlapping type arrays", obj(map[string]any{"v": Schema{"type": []string{"string", "null"}}}), obj(map[string]any{"v": Schema{"type": "string"}}), true, ""},
		{"disjoint type arrays", obj(map[string]any{"v": Schema{"type": []any{"string", "null"}}}), obj(map[string]any{"v": Schema{"type": "number"}}), false, `field "v": producer emits string|null but consumer expects number`},
		{"disjoint enums", obj(map[string]any{"status": Schema{"type": "string", "enum": []any{"ok", "pending"}}}), obj(map[string]any{"status": Schema{"type": "string", "enum": []any{"error", "failed"}}}), false, `field "status": producer enum is disjoint from consumer enum`},
		{"overlapping enums", obj(map[string]any{"status": Schema{"type": "string", "enum": []string{"ok", "pending"}}}), obj(map[string]any{"status": Schema{"type": "string", "enum": []string{"ok", "done"}}}), true, ""},
		{"conflicting const", obj(map[string]any{"mode": Schema{"const": "fast"}}), obj(map[string]any{"mode": Schema{"const": "slow"}}), false, `field "mode": producer const "fast" ≠ consumer const "slow"`},
		{"matching const", obj(map[string]any{"mode": Schema{"const": "fast"}}), obj(map[string]any{"mode": Schema{"const": "fast"}}), true, ""},
		{"closed producer missing a required field", obj(map[string]any{"a": Schema{"type": "string"}}, Schema{"additionalProperties": false}), obj(map[string]any{"a": Schema{"type": "string"}, "b": Schema{"type": "number"}}, Schema{"required": []string{"b"}}), false, `consumer requires "b" but producer (closed object) never emits it`},
		{"open producer missing a required field", obj(map[string]any{"a": Schema{"type": "string"}}), obj(map[string]any{"a": Schema{"type": "string"}, "b": Schema{"type": "number"}}, Schema{"required": []string{"b"}}), true, ""},
		{"root type mismatch string vs number", Schema{"type": "string"}, Schema{"type": "number"}, false, `root type mismatch: producer is "string", consumer is "number"`},
		{"root type mismatch array vs object", Schema{"type": "array"}, Schema{"type": "object"}, false, `root type mismatch: producer is "array", consumer is "object"`},
		{"matching non-object roots", Schema{"type": "string"}, Schema{"type": "string"}, true, ""},
		{"one side lacks a root type", Schema{}, Schema{"type": "string"}, true, ""},
		{"other side lacks a root type", Schema{"type": "string"}, Schema{}, true, ""},
		{"neither has properties", Schema{"type": "object"}, Schema{"type": "object"}, true, ""},
		{"fields present in only one schema", obj(map[string]any{"a": Schema{"type": "string"}, "extra": Schema{"type": "boolean"}}), obj(map[string]any{"a": Schema{"type": "string"}, "other": Schema{"type": "number"}}), true, ""},
		{"nested-object differences are not compared", obj(map[string]any{"nested": obj(map[string]any{"x": Schema{"type": "string"}})}), obj(map[string]any{"nested": obj(map[string]any{"x": Schema{"type": "number"}})}), true, ""},
		{"integer in a type array is a number", obj(map[string]any{"v": Schema{"type": []string{"integer", "null"}}}), obj(map[string]any{"v": Schema{"type": "number"}}), true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := IsSchemaCompatible(c.producer, c.consumer)
			if got.OK != c.ok || got.Reason != c.reason {
				t.Fatalf("got %+v want ok=%v reason=%q", got, c.ok, c.reason)
			}
		})
	}
}
