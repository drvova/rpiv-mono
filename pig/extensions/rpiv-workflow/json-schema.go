package workflow

import (
	"context"
	"fmt"
	"strings"
)

// Raw-JSON-Schema ↔ Standard Schema bridge + introspection helpers. Schemas
// are inspectable AS DATA, not just executable validators:
//   - JsonSchemaToStandard(schema) wraps a plain JSON-Schema-shaped object (a
//     skill's frontmatter consumes.data / produces.data) into a StageSchema
//     backed by CheckSchema, carrying the jsonSchema converter so the captured
//     schema round-trips back out.
//   - HasJsonSchema / ExtractJsonSchema feature-detect and pull the JSON
//     Schema off ANY StageSchema; opaque schemas degrade to (nil, false) and
//     the caller skips + warns.

// Schema is a JSON Schema (draft-7 subset) as data — the one schema
// representation of the port. The constructors in typebox-adapter.go build
// them; CheckSchema validates against them.
type Schema map[string]any

// JsonSchemaObject is a JSON-Schema-shaped object — the canonical contract
// data format. The same representation as Schema.
type JsonSchemaObject = Schema

// IsJsonSchemaObject is the runtime guard that a value is a plain (non-nil)
// object — the minimum shape a JSON Schema must have. Injected contracts are
// untrusted: callers feeding a consumes.data / produces.data into the keyword
// engine guard with this first, so a malformed contract DEGRADES instead of
// failing a load or a run. The second result is the object, converted.
func IsJsonSchemaObject(value any) (JsonSchemaObject, bool) {
	s := toSchemaObject(value)
	return s, s != nil
}

// toSchemaObject converts the map shapes a schema can arrive in; nil for
// anything that is not a string-keyed map.
func toSchemaObject(value any) JsonSchemaObject {
	switch v := value.(type) {
	case Schema:
		if v == nil {
			return nil
		}
		return v
	case map[string]any:
		if v == nil {
			return nil
		}
		return Schema(v)
	}
	return nil
}

// StandardIssue is one validation failure in Standard Schema form: a message
// plus the path of the offending value (nil at the root).
type StandardIssue struct {
	Message string
	Path    []string
}

// StandardResult is what StageSchema.Validate returns: Issues nil ⇒ success
// with Value (the input, since validation-only schemas transform nothing);
// Issues non-nil (possibly empty) ⇒ failure.
type StandardResult struct {
	Value  any
	Issues []StandardIssue
}

// OK reports whether the result is a success.
func (r StandardResult) OK() bool { return r.Issues == nil }

// StageSchema is the schema attached to a stage's OutputSchema / InputSchema —
// the Go form of the Standard Schema v1 validate surface. Validate may do I/O
// and honours ctx; the runner bounds a slow schema by the stage's
// validateTimeoutMs. A returned error is the port of a throwing validator.
type StageSchema interface {
	Validate(ctx context.Context, value any) (StandardResult, error)
}

// JSONSchemaTarget names a JSON Schema dialect a converter can emit.
type JSONSchemaTarget string

// Supported targets — the pair whose dialect our structural subset serialises
// to identically.
const (
	TargetDraft2020 JSONSchemaTarget = "draft-2020-12"
	TargetDraft07   JSONSchemaTarget = "draft-07"
)

var supportedTargets = []JSONSchemaTarget{TargetDraft2020, TargetDraft07}

// JSONSchemaOptions are the converter's inputs.
type JSONSchemaOptions struct {
	Target JSONSchemaTarget
}

// JSONSchemaConverter exposes a schema as JSON Schema data — the Standard
// JSON Schema v1 converter: Input for the accepted shape, Output for the
// emitted one (identical for validation-only schemas).
type JSONSchemaConverter interface {
	Input(options JSONSchemaOptions) (JsonSchemaObject, error)
	Output(options JSONSchemaOptions) (JsonSchemaObject, error)
}

// JsonSchemaCapable is a StageSchema that ALSO exposes its JSON Schema as
// data. TypeboxSchema and JsonSchemaToStandard both return one; a
// third-party StageSchema satisfies only the validate surface.
type JsonSchemaCapable interface {
	StageSchema
	JSONSchema() JSONSchemaConverter
}

type jsonSchemaConverter struct {
	schema JsonSchemaObject
}

func (c jsonSchemaConverter) convert(options JSONSchemaOptions) (JsonSchemaObject, error) {
	for _, t := range supportedTargets {
		if options.Target == t {
			return c.schema, nil
		}
	}
	names := make([]string, len(supportedTargets))
	for i, t := range supportedTargets {
		names[i] = string(t)
	}
	return nil, fmt.Errorf("jsonSchema: unsupported target %q (supported: %s)", options.Target, strings.Join(names, ", "))
}

func (c jsonSchemaConverter) Input(options JSONSchemaOptions) (JsonSchemaObject, error) {
	return c.convert(options)
}

func (c jsonSchemaConverter) Output(options JSONSchemaOptions) (JsonSchemaObject, error) {
	return c.convert(options)
}

// JsonSchemaConverter builds the converter for a captured schema. The
// captured value is already a clean JSON Schema, so Input and Output both
// return it. draft-2020-12 and draft-07 are supported; other targets error.
func JsonSchemaConverter(schema JsonSchemaObject) JSONSchemaConverter {
	return jsonSchemaConverter{schema: schema}
}

// ValidateToJsonSchemaIssues is the shared CheckSchema → StandardIssue
// mapping both wrappers call, so validation failures are reported identically
// regardless of which wrapper was used. Each issue's path is the instance
// path split on "/" (nil at the root). A *SchemaDefectError is the port of
// Value.Check throwing on an unusable schema: the validator threw, the value
// did not fail.
func ValidateToJsonSchemaIssues(schema JsonSchemaObject, value any) (StandardResult, error) {
	errs, err := CheckSchema(schema, value)
	if err != nil {
		return StandardResult{}, err
	}
	if len(errs) == 0 {
		return StandardResult{Value: value}, nil
	}
	issues := make([]StandardIssue, 0, len(errs))
	for _, e := range errs {
		message := e.Message
		if message == "" {
			at := e.InstancePath
			if at == "" {
				at = "root"
			}
			message = fmt.Sprintf("%s validation failed at %s", e.Keyword, at)
		}
		var path []string
		if e.InstancePath != "" {
			for _, part := range strings.Split(e.InstancePath, "/") {
				if part != "" {
					path = append(path, part)
				}
			}
		}
		issues = append(issues, StandardIssue{Message: message, Path: path})
	}
	return StandardResult{Issues: issues}, nil
}

// standardSchema is the JsonSchemaCapable both wrappers return.
type standardSchema struct {
	vendor    string
	schema    JsonSchemaObject
	converter JSONSchemaConverter
}

func (s *standardSchema) Validate(_ context.Context, value any) (StandardResult, error) {
	return ValidateToJsonSchemaIssues(s.schema, value)
}

func (s *standardSchema) JSONSchema() JSONSchemaConverter { return s.converter }

// Vendor names the wrapper that built the schema ("rpiv-json-schema" or "typebox").
func (s *standardSchema) Vendor() string { return s.vendor }

// JsonSchemaToStandard wraps a raw JSON-Schema-shaped object into a
// StageSchema that BOTH validates (via CheckSchema) and exposes the schema as
// data (the converter).
func JsonSchemaToStandard(schema JsonSchemaObject) JsonSchemaCapable {
	return &standardSchema{vendor: "rpiv-json-schema", schema: schema, converter: JsonSchemaConverter(schema)}
}

// HasJsonSchema is true when a StageSchema exposes its JSON Schema as data
// (carries a non-nil converter). A nil schema or a validate-only schema does not.
func HasJsonSchema(schema StageSchema) bool {
	if schema == nil {
		return false
	}
	capable, ok := schema.(JsonSchemaCapable)
	return ok && capable.JSONSchema() != nil
}

// ExtractJsonSchema pulls the JSON Schema back off a StageSchema as data, or
// (nil, false) when the schema is opaque or the converter rejects the target.
// An empty target defaults to draft-2020-12.
func ExtractJsonSchema(schema StageSchema, target JSONSchemaTarget) (JsonSchemaObject, bool) {
	if !HasJsonSchema(schema) {
		return nil, false
	}
	if target == "" {
		target = TargetDraft2020
	}
	out, err := schema.(JsonSchemaCapable).JSONSchema().Output(JSONSchemaOptions{Target: target})
	if err != nil {
		return nil, false
	}
	return out, true
}
