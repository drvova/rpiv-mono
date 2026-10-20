package workflow

// Schema constructors — the port of the TypeBox builders the built-in
// workflows author with (Type.Object, Type.String, …) as plain constructors
// over Schema, plus TypeboxSchema, the wrapper that promotes a constructed
// Schema to the StageSchema the validator consumes.

// T is the schema builder namespace: T.Object(...), T.String(), T.Integer(...).
var T SchemaBuilder

// SchemaBuilder builds Schema values. Every constructor accepts optional
// extra keywords that are merged over the base shape (T.Integer(Schema{
// "minimum": 0})).
type SchemaBuilder struct{}

// optionalMarker is the private keyword T.Optional attaches so T.Object can
// leave the property out of `required`; T.Object strips it.
const optionalMarker = "x-rpiv-optional"

func withOptions(base Schema, options []Schema) Schema {
	for _, opts := range options {
		for k, v := range opts {
			base[k] = v
		}
	}
	return base
}

// Object builds an object schema whose declared properties are all required
// unless wrapped in T.Optional. Properties are copied, so the input map is
// never mutated; property order in `required` follows the insertion order
// the caller supplies via PropertyOrder, else sorted names.
func (SchemaBuilder) Object(properties map[string]Schema, options ...Schema) Schema {
	props := make(map[string]any, len(properties))
	required := make([]string, 0, len(properties))
	for _, name := range sortedKeys(properties) {
		prop := properties[name]
		copied := make(Schema, len(prop))
		optional := false
		for k, v := range prop {
			if k == optionalMarker {
				optional = true
				continue
			}
			copied[k] = v
		}
		props[name] = copied
		if !optional {
			required = append(required, name)
		}
	}
	base := Schema{"type": "object", "properties": props}
	if len(required) > 0 {
		base["required"] = required
	}
	return withOptions(base, options)
}

// Optional marks a property as optional inside T.Object.
func (SchemaBuilder) Optional(prop Schema) Schema {
	copied := make(Schema, len(prop)+1)
	for k, v := range prop {
		copied[k] = v
	}
	copied[optionalMarker] = true
	return copied
}

// String builds {type: "string"}.
func (SchemaBuilder) String(options ...Schema) Schema {
	return withOptions(Schema{"type": "string"}, options)
}

// Number builds {type: "number"}.
func (SchemaBuilder) Number(options ...Schema) Schema {
	return withOptions(Schema{"type": "number"}, options)
}

// Integer builds {type: "integer"}.
func (SchemaBuilder) Integer(options ...Schema) Schema {
	return withOptions(Schema{"type": "integer"}, options)
}

// Boolean builds {type: "boolean"}.
func (SchemaBuilder) Boolean(options ...Schema) Schema {
	return withOptions(Schema{"type": "boolean"}, options)
}

// Null builds {type: "null"}.
func (SchemaBuilder) Null(options ...Schema) Schema {
	return withOptions(Schema{"type": "null"}, options)
}

// Array builds {type: "array", items}.
func (SchemaBuilder) Array(items Schema, options ...Schema) Schema {
	return withOptions(Schema{"type": "array", "items": items}, options)
}

// Literal builds {const: value}.
func (SchemaBuilder) Literal(value any, options ...Schema) Schema {
	return withOptions(Schema{"const": value}, options)
}

// Enum builds {enum: values}.
func (SchemaBuilder) Enum(values []any, options ...Schema) Schema {
	return withOptions(Schema{"enum": values}, options)
}

// Union builds {anyOf: members}.
func (SchemaBuilder) Union(members []Schema, options ...Schema) Schema {
	anyOf := make([]any, len(members))
	for i, m := range members {
		anyOf[i] = m
	}
	return withOptions(Schema{"anyOf": anyOf}, options)
}

// Record builds an object schema whose every value matches the given schema.
func (SchemaBuilder) Record(values Schema, options ...Schema) Schema {
	return withOptions(Schema{"type": "object", "additionalProperties": values}, options)
}

// Any builds the empty schema (accepts everything).
func (SchemaBuilder) Any(options ...Schema) Schema {
	return withOptions(Schema{}, options)
}

// Unknown is an alias of Any.
func (b SchemaBuilder) Unknown(options ...Schema) Schema { return b.Any(options...) }

// TypeboxSchema promotes a constructed Schema to a JsonSchemaCapable — a
// StageSchema that validates through CheckSchema AND exposes the schema as
// data. Identical in behaviour to JsonSchemaToStandard; the vendor tag
// ("typebox") records which authoring path built it.
func TypeboxSchema(schema Schema) JsonSchemaCapable {
	return &standardSchema{vendor: "typebox", schema: schema, converter: JsonSchemaConverter(schema)}
}
