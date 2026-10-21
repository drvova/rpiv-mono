package workflow

// Schema constructors — the port of the TypeBox builders the built-in
// workflows author with (Type.Object, Type.String, …) as plain constructors
// over Schema, emitting the same JSON Schema data TypeBox emits, plus
// TypeboxSchema, the wrapper that promotes a constructed Schema to the
// StageSchema the validator consumes.

// T is the schema builder namespace: T.Object(...), T.String(), T.Integer(...).
var T SchemaBuilder

// SchemaBuilder builds Schema values. Every constructor accepts optional
// extra keywords that are merged over the base shape (T.Integer(Schema{
// "minimum": 0})).
type SchemaBuilder struct{}

// optionalMarker is the private keyword T.Optional attaches so T.Object can
// leave the property out of `required`. T.Object strips it, and so does
// every constructor that embeds a sub-schema, so the marker never reaches
// schema data (TypeBox keeps its marker on a symbol key for the same reason).
const optionalMarker = "x-rpiv-optional"

// stripOptional returns s without the optional marker (s itself when unmarked).
func stripOptional(s Schema) Schema {
	if _, marked := s[optionalMarker]; !marked {
		return s
	}
	copied := make(Schema, len(s))
	for k, v := range s {
		if k != optionalMarker {
			copied[k] = v
		}
	}
	return copied
}

func withOptions(base Schema, options []Schema) Schema {
	for _, opts := range options {
		for k, v := range opts {
			if sub, ok := v.(Schema); ok {
				v = stripOptional(sub)
			}
			base[k] = v
		}
	}
	return base
}

// Object builds an object schema whose declared properties are all required
// unless wrapped in T.Optional — the shape TypeBox's Type.Object emits, with
// `required` omitted when no property is required. Properties are copied, so
// the input map is never mutated. Go maps carry no insertion order, so
// `required` lists names in sorted order; pass Schema{"required": …} in
// options to pin another order where a consumer compares the schema as data.
func (SchemaBuilder) Object(properties map[string]Schema, options ...Schema) Schema {
	props := make(map[string]any, len(properties))
	required := make([]string, 0, len(properties))
	for _, name := range sortedKeys(properties) {
		prop := properties[name]
		_, optional := prop[optionalMarker]
		props[name] = stripOptional(prop)
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
	return withOptions(Schema{"type": "array", "items": stripOptional(items)}, options)
}

// Literal builds the schema TypeBox's Type.Literal emits: the value's JSON
// type plus `const` (a value of another kind carries only `const`).
func (SchemaBuilder) Literal(value any, options ...Schema) Schema {
	base := Schema{"const": value}
	switch value.(type) {
	case string:
		base["type"] = "string"
	case bool:
		base["type"] = "boolean"
	default:
		if _, ok := asFloat(value); ok {
			base["type"] = "number"
		}
	}
	return withOptions(base, options)
}

// Enum builds {enum: values}.
func (SchemaBuilder) Enum(values []any, options ...Schema) Schema {
	return withOptions(Schema{"enum": values}, options)
}

// Union builds {anyOf: members}.
func (SchemaBuilder) Union(members []Schema, options ...Schema) Schema {
	anyOf := make([]any, len(members))
	for i, m := range members {
		anyOf[i] = stripOptional(m)
	}
	return withOptions(Schema{"anyOf": anyOf}, options)
}

// recordKeyPattern is the pattern TypeBox's Type.Record emits for a string
// key schema.
const recordKeyPattern = "^.*$"

// Record builds an object schema whose every value matches the given schema
// — {type: "object", patternProperties: {"^.*$": values}}, the shape
// TypeBox's Type.Record(Type.String(), values) emits.
func (SchemaBuilder) Record(values Schema, options ...Schema) Schema {
	return withOptions(Schema{"type": "object", "patternProperties": map[string]any{recordKeyPattern: stripOptional(values)}}, options)
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
