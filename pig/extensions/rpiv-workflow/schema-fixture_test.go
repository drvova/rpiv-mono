package workflow

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// typeboxFixture replays testdata/typebox-fixture.json: every case was run
// through the real typebox@1.1.39 Value.Check / Value.Errors, so this is the
// differential contract between the two validators — validity, and each
// error's keyword, instance path, schema path and message, in order.
type typeboxFixture struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Value  any             `json:"value"`
	OK     bool            `json:"ok"`
	Errors []struct {
		Keyword      string `json:"keyword"`
		InstancePath string `json:"instancePath"`
		SchemaPath   string `json:"schemaPath"`
		Message      string `json:"message"`
	} `json:"errors"`
	Threw string `json:"threw,omitempty"`
}

func TestCheckSchemaAgainstTypeBoxFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/typebox-fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []typeboxFixture
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 150 {
		t.Fatalf("fixture holds only %d cases", len(cases))
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			var schema any
			if err := json.Unmarshal(c.Schema, &schema); err != nil {
				t.Fatal(err)
			}
			checker := &checker{}
			checker.check(schema, c.Value, "", "#")
			if c.Threw != "" {
				if checker.defect == nil {
					t.Fatalf("typebox threw %q; got errors %v", c.Threw, checker.errs)
				}
				return
			}
			if checker.defect != nil {
				t.Fatalf("schema defect %v", checker.defect)
			}
			if got := len(checker.errs) == 0; got != c.OK {
				t.Fatalf("valid=%v want %v (errors %+v)", got, c.OK, checker.errs)
			}
			want := make([]SchemaError, len(c.Errors))
			for i, e := range c.Errors {
				want[i] = SchemaError{Keyword: e.Keyword, InstancePath: e.InstancePath, SchemaPath: e.SchemaPath, Message: e.Message}
			}
			got := checker.errs
			if got == nil {
				got = []SchemaError{}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("errors\n got  %+v\n want %+v", got, want)
			}
		})
	}
}
