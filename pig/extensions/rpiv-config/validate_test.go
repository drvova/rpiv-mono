package config

import (
	"encoding/json"
	"reflect"
	"testing"
)

type nested struct {
	A int `json:"a"`
	B int `json:"b"`
}

type EmbeddedPtr struct {
	Deep string `json:"deep"`
}

type richConfig struct {
	GuidanceFields
	*EmbeddedPtr
	Name    string            `json:"name"`
	Count   int               `json:"count"`
	Nested  nested            `json:"nested"`
	List    []string          `json:"list"`
	Opt     *int              `json:"opt"`
	Dict    map[string]string `json:"dict"`
	Skipped string            `json:"-"`
	Plain   string
	secret  string
}

func richDefaults() richConfig {
	seven := 7
	return richConfig{
		GuidanceFields: GuidanceFields{PromptSnippet: "default snippet"},
		EmbeddedPtr:    &EmbeddedPtr{Deep: "default deep"},
		Name:           "unnamed",
		Count:          1,
		Nested:         nested{A: 1, B: 2},
		List:           []string{"x"},
		Opt:            &seven,
		Dict:           map[string]string{"k": "v"},
		Skipped:        "kept",
		Plain:          "plain",
		secret:         "hidden",
	}
}

func TestValidateConfigContract(t *testing.T) {
	cases := []struct {
		name  string
		value any
		check func(t *testing.T, got richConfig)
	}{
		{"matches keys exactly, not case-insensitively", map[string]any{"Name": "x", "NAME": "y"}, func(t *testing.T, got richConfig) {
			if got.Name != "unnamed" {
				t.Fatalf("Name = %q, want the default", got.Name)
			}
		}},
		{"uses the Go field name when there is no json tag", map[string]any{"Plain": "p2", "plain": "lower"}, func(t *testing.T, got richConfig) {
			if got.Plain != "p2" {
				t.Fatalf("Plain = %q", got.Plain)
			}
		}},
		{`ignores json:"-" fields and unexported fields`, map[string]any{"Skipped": "x", "-": "y", "secret": "z"}, func(t *testing.T, got richConfig) {
			if got.Skipped != "kept" || got.secret != "hidden" {
				t.Fatalf("got %+v", got)
			}
		}},
		{"promotes the fields of an embedded struct", map[string]any{"promptSnippet": "s", "promptGuidelines": []any{"g"}}, func(t *testing.T, got richConfig) {
			want := GuidanceFields{PromptSnippet: "s", PromptGuidelines: []string{"g"}}
			if !reflect.DeepEqual(got.GuidanceFields, want) {
				t.Fatalf("GuidanceFields = %+v, want %+v", got.GuidanceFields, want)
			}
		}},
		{"does not treat the embedded type name as a key", map[string]any{"GuidanceFields": map[string]any{"promptSnippet": "s"}}, func(t *testing.T, got richConfig) {
			if got.PromptSnippet != "default snippet" {
				t.Fatalf("PromptSnippet = %q", got.PromptSnippet)
			}
		}},
		{"sets a field promoted through an embedded pointer", map[string]any{"deep": "d"}, func(t *testing.T, got richConfig) {
			if got.EmbeddedPtr == nil || got.Deep != "d" {
				t.Fatalf("got %+v", got.EmbeddedPtr)
			}
		}},
		{"replaces a nested object wholesale (no deep merge, like the TypeScript spread)", map[string]any{"nested": map[string]any{"a": float64(5)}}, func(t *testing.T, got richConfig) {
			if got.Nested != (nested{A: 5, B: 0}) {
				t.Fatalf("Nested = %+v", got.Nested)
			}
		}},
		{"ignores unknown nested keys", map[string]any{"nested": map[string]any{"a": float64(5), "zzz": true}}, func(t *testing.T, got richConfig) {
			if got.Nested != (nested{A: 5, B: 0}) {
				t.Fatalf("Nested = %+v", got.Nested)
			}
		}},
		{"replaces a map wholesale", map[string]any{"dict": map[string]any{"n": "m"}}, func(t *testing.T, got richConfig) {
			if !reflect.DeepEqual(got.Dict, map[string]string{"n": "m"}) {
				t.Fatalf("Dict = %v", got.Dict)
			}
		}},
		{"replaces a list wholesale", map[string]any{"list": []any{"a", "b"}}, func(t *testing.T, got richConfig) {
			if !reflect.DeepEqual(got.List, []string{"a", "b"}) {
				t.Fatalf("List = %v", got.List)
			}
		}},
		{"drops a key of the wrong JSON type and keeps the default", map[string]any{"count": "five", "name": float64(1), "list": "x"}, func(t *testing.T, got richConfig) {
			if got.Count != 1 || got.Name != "unnamed" || !reflect.DeepEqual(got.List, []string{"x"}) {
				t.Fatalf("got %+v", got)
			}
		}},
		{"drops a number that overflows the field type", map[string]any{"count": 1e300}, func(t *testing.T, got richConfig) {
			if got.Count != 1 {
				t.Fatalf("Count = %d", got.Count)
			}
		}},
		{"decodes null to the zero value", map[string]any{"name": nil, "opt": nil, "list": nil, "nested": nil}, func(t *testing.T, got richConfig) {
			if got.Name != "" || got.Opt != nil || got.List != nil || got.Nested != (nested{}) {
				t.Fatalf("got %+v", got)
			}
		}},
		{"applies every present key and keeps the rest", map[string]any{"name": "n", "opt": float64(3)}, func(t *testing.T, got richConfig) {
			if got.Name != "n" || got.Opt == nil || *got.Opt != 3 || got.Count != 1 || got.Nested != (nested{A: 1, B: 2}) {
				t.Fatalf("got %+v", got)
			}
		}},
		{"accepts an empty object and changes nothing", map[string]any{}, func(t *testing.T, got richConfig) {
			if !reflect.DeepEqual(got, richDefaults()) {
				t.Fatalf("got %+v", got)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dst := richDefaults()
			if !ValidateConfig(&dst, tc.value) {
				t.Fatal("want true")
			}
			tc.check(t, dst)
		})
	}
}

func TestValidateConfigDoesNotMutateSharedDefaults(t *testing.T) {
	defaults := richDefaults()
	got := ValidateConfigAs(defaults, map[string]any{
		"deep":          "changed",
		"promptSnippet": "changed",
		"nested":        map[string]any{"a": float64(9)},
		"opt":           float64(1),
	})
	if got.Deep != "changed" || got.PromptSnippet != "changed" || got.Nested.A != 9 || *got.Opt != 1 {
		t.Fatalf("overlay not applied: %+v", got)
	}
	if defaults.Deep != "default deep" {
		t.Fatalf("the pointee shared with defaults was mutated: %q", defaults.Deep)
	}
	if got.EmbeddedPtr == defaults.EmbeddedPtr {
		t.Fatal("the embedded pointer must be replaced by a copy")
	}
	if !reflect.DeepEqual(defaults, richDefaults()) {
		t.Fatalf("defaults changed: %+v", defaults)
	}
}

func TestValidateConfigRejectsBadDestinations(t *testing.T) {
	var nilCfg *sampleConfig
	n := 3
	cases := []struct {
		name string
		dst  any
	}{
		{"nil", nil},
		{"nil struct pointer", nilCfg},
		{"struct value, not a pointer", sampleConfig{}},
		{"pointer to a non-struct", &n},
		{"pointer to a map", &map[string]any{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if ValidateConfig(tc.dst, map[string]any{"name": "x"}) {
				t.Fatal("want false")
			}
		})
	}
	if got := ValidateConfigAs(5, map[string]any{"x": 1}); got != 5 {
		t.Fatalf("ValidateConfigAs on a non-struct must return its defaults, got %v", got)
	}
}

func TestValidateConfigValueForms(t *testing.T) {
	var nilMap map[string]any
	var nilPtr *sampleConfig
	raw := json.RawMessage(`{"name":"from-raw"}`)
	cases := []struct {
		name   string
		value  any
		wantOK bool
		want   sampleConfig
	}{
		{"map[string]any", map[string]any{"name": "a"}, true, sampleConfig{Name: "a"}},
		{"nil map[string]any is an empty object", nilMap, true, sampleDefaults()},
		{"map[string]json.RawMessage", map[string]json.RawMessage{"count": json.RawMessage("4")}, true, sampleConfig{Name: "unnamed", Count: 4}},
		{"map with string keys", map[string]string{"name": "b"}, true, sampleConfig{Name: "b"}},
		{"map with non-string keys", map[int]string{1: "b"}, false, sampleDefaults()},
		{"struct value", sampleConfig{Name: "c", Count: 2}, true, sampleConfig{Name: "c", Count: 2}},
		{"struct pointer", &sampleConfig{Name: "d"}, true, sampleConfig{Name: "d"}},
		{"nil struct pointer", nilPtr, false, sampleDefaults()},
		{"a struct whose JSON is not an object", json.Number("1"), false, sampleDefaults()},
		{"string", "x", false, sampleDefaults()},
		{"number", 1.5, false, sampleDefaults()},
		{"bool", true, false, sampleDefaults()},
		{"slice", []any{"a"}, false, sampleDefaults()},
		{"json.RawMessage is a slice, not an object", raw, false, sampleDefaults()},
		{"[]byte is a slice, not an object", []byte(`{"name":"x"}`), false, sampleDefaults()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dst := sampleDefaults()
			ok := ValidateConfig(&dst, tc.value)
			if ok != tc.wantOK || dst != tc.want {
				t.Fatalf("got %v, %+v; want %v, %+v", ok, dst, tc.wantOK, tc.want)
			}
		})
	}
}

func TestValidateConfigHonorsFieldUnmarshalers(t *testing.T) {
	type withLevel struct {
		Level level `json:"level"`
	}
	dst := withLevel{Level: 1}
	if !ValidateConfig(&dst, map[string]any{"level": "high"}) || dst.Level != 3 {
		t.Fatalf("got %+v", dst)
	}
	dst = withLevel{Level: 1}
	if !ValidateConfig(&dst, map[string]any{"level": "bogus"}) || dst.Level != 1 {
		t.Fatalf("a failing UnmarshalJSON must keep the default, got %+v", dst)
	}
}

// level is a field type with its own JSON decoding.
type level int

func (l *level) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch s {
	case "low":
		*l = 1
	case "high":
		*l = 3
	default:
		return &json.UnmarshalTypeError{Value: s, Type: reflect.TypeFor[level]()}
	}
	return nil
}

// ---------------------------------------------------------------------------
// jsonFields: encoding/json's naming and dominance rules
// ---------------------------------------------------------------------------

type inner struct {
	X string `json:"x"`
	Y string `json:"y"`
}

type shadowing struct {
	inner
	X string `json:"x"` // depth 0 hides inner.X at depth 1
}

type leftX struct {
	X string
	Y string `json:"y"`
}

type rightX struct {
	X string
}

type ambiguous struct {
	leftX
	rightX // "X" is promoted twice at depth 1, both untagged: ambiguous, dropped
}

type taggedX struct {
	X string `json:"X"`
}

type mixed struct {
	taggedX
	rightX // "X" at depth 1 from both; the tagged one wins
}

type namedEmbed struct {
	inner                 // promotes x and y
	rightX `json:"named"` // a tagged embedded struct is a plain field, not promoted
}

type base struct {
	X string
}

type viaA struct{ base }
type viaB struct{ base }

type twice struct {
	viaA
	viaB // base is reached twice at depth 2: its fields are ambiguous
}

type oddTags struct {
	Field string `json:"ok.name!"` // valid: punctuation is allowed
	Other string `json:"a\"b"`     // invalid: a quote is not, so the Go name is used
}

func TestJSONFields(t *testing.T) {
	cases := []struct {
		name string
		typ  reflect.Type
		want map[string][]int
	}{
		{"flat with tags", reflect.TypeFor[sampleConfig](), map[string][]int{"name": {0}, "count": {1}}},
		{"shallower field hides a promoted one", reflect.TypeFor[shadowing](), map[string][]int{"x": {1}, "y": {0, 1}}},
		{"same-depth untagged tie is ambiguous", reflect.TypeFor[ambiguous](), map[string][]int{"y": {0, 1}}},
		{"tagged field wins over an untagged one at the same depth", reflect.TypeFor[mixed](), map[string][]int{"X": {0, 0}}},
		{"named embedded struct is a field", reflect.TypeFor[namedEmbed](), map[string][]int{"x": {0, 0}, "y": {0, 1}, "named": {1}}},
		{"same type reached twice is ambiguous", reflect.TypeFor[twice](), map[string][]int{}},
		{"tag validity", reflect.TypeFor[oddTags](), map[string][]int{"ok.name!": {0}, "Other": {1}}},
		{"embedded pointer and value, dash and unexported", reflect.TypeFor[richConfig](), map[string][]int{
			"promptSnippet": {0, 0}, "promptGuidelines": {0, 1}, "description": {0, 2},
			"deep": {1, 0},
			"name": {2}, "count": {3}, "nested": {4}, "list": {5}, "opt": {6}, "dict": {7}, "Plain": {9},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := jsonFields(tc.typ)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("jsonFields(%v) = %v, want %v", tc.typ, got, tc.want)
			}
		})
	}
}

// The mapping must agree with encoding/json on which keys it fills: a key jsonFields reports decodes into
// exactly that field, and a key it omits (an ambiguous one) is ignored by encoding/json too.
func TestJSONFieldsAgreesWithEncodingJSON(t *testing.T) {
	t.Run("ambiguous key is ignored by both", func(t *testing.T) {
		var v ambiguous
		if err := json.Unmarshal([]byte(`{"X":"set","y":"y"}`), &v); err != nil {
			t.Fatal(err)
		}
		if v.leftX.X != "" || v.rightX.X != "" || v.Y != "y" {
			t.Fatalf("got %+v", v)
		}
		dst := ambiguous{leftX: leftX{X: "dx", Y: "dy"}, rightX: rightX{X: "dx2"}}
		ValidateConfig(&dst, map[string]any{"X": "set", "y": "y"})
		if dst.leftX.X != "dx" || dst.rightX.X != "dx2" || dst.Y != "y" {
			t.Fatalf("got %+v", dst)
		}
	})
	t.Run("tagged field receives the key for both", func(t *testing.T) {
		var v mixed
		if err := json.Unmarshal([]byte(`{"X":"set"}`), &v); err != nil {
			t.Fatal(err)
		}
		dst := mixed{taggedX: taggedX{X: "t"}, rightX: rightX{X: "u"}}
		ValidateConfig(&dst, map[string]any{"X": "set"})
		if v.taggedX.X != "set" || v.rightX.X != "" || dst.taggedX.X != "set" || dst.rightX.X != "u" {
			t.Fatalf("json %+v, validate %+v", v, dst)
		}
	})
	t.Run("shadowed key goes to the outer field for both", func(t *testing.T) {
		var v shadowing
		if err := json.Unmarshal([]byte(`{"x":"outer"}`), &v); err != nil {
			t.Fatal(err)
		}
		dst := shadowing{inner: inner{X: "ix"}, X: "ox"}
		ValidateConfig(&dst, map[string]any{"x": "outer"})
		if v.X != "outer" || v.inner.X != "" || dst.X != "outer" || dst.inner.X != "ix" {
			t.Fatalf("json %+v, validate %+v", v, dst)
		}
	})
	t.Run("type reached twice is ignored by both", func(t *testing.T) {
		var v twice
		if err := json.Unmarshal([]byte(`{"X":"set"}`), &v); err != nil {
			t.Fatal(err)
		}
		dst := twice{viaA: viaA{base{X: "a"}}, viaB: viaB{base{X: "b"}}}
		ValidateConfig(&dst, map[string]any{"X": "set"})
		if v.viaA.X != "" || v.viaB.X != "" || dst.viaA.X != "a" || dst.viaB.X != "b" {
			t.Fatalf("json %+v, validate %+v", v, dst)
		}
	})
}

type hiddenPtr struct {
	Deep string `json:"deep"`
}

type viaUnexportedPtr struct {
	*hiddenPtr
	Name string `json:"name"`
}

func TestValidateConfigDropsKeysBehindUnexportedEmbeddedPointer(t *testing.T) {
	shared := &hiddenPtr{Deep: "d"}
	dst := viaUnexportedPtr{hiddenPtr: shared, Name: "n"}
	if !ValidateConfig(&dst, map[string]any{"deep": "x", "name": "m"}) {
		t.Fatal("want true")
	}
	if dst.Name != "m" || dst.Deep != "d" || shared.Deep != "d" || dst.hiddenPtr != shared {
		t.Fatalf("got %+v (shared %+v)", dst, shared)
	}
	empty := viaUnexportedPtr{Name: "n"}
	if !ValidateConfig(&empty, map[string]any{"deep": "x"}) || empty.hiddenPtr != nil {
		t.Fatalf("got %+v", empty)
	}
}
