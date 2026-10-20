package workflow

import (
	"math"
	"testing"
)

func TestJSNumber(t *testing.T) {
	cases := []struct {
		in   any
		want float64
	}{
		{nil, math.NaN()}, {true, 1}, {false, 0}, {"", 0}, {"  12 ", 12}, {"1e3", 1000}, {"0x1A", 26}, {"abc", math.NaN()},
		{"Infinity", math.Inf(1)}, {3, 3}, {int64(4), 4}, {2.5, 2.5}, {[]any{}, 0}, {[]any{"7"}, 7}, {[]any{1, 2}, math.NaN()}, {map[string]any{}, math.NaN()},
	}
	for _, c := range cases {
		got := jsNumber(c.in)
		if math.IsNaN(c.want) {
			if !math.IsNaN(got) {
				t.Errorf("jsNumber(%v) = %v want NaN", c.in, got)
			}
		} else if got != c.want {
			t.Errorf("jsNumber(%v) = %v want %v", c.in, got, c.want)
		}
	}
}

func TestFormatJSNumber(t *testing.T) {
	cases := map[float64]string{0: "0", 3: "3", -1: "-1", 1.5: "1.5", 1e21: "1e+21", 1e-7: "1e-7", 123456789: "123456789", 0.1: "0.1", math.NaN(): "NaN", math.Inf(-1): "-Infinity"}
	for in, want := range cases {
		if got := formatJSNumber(in); got != want {
			t.Errorf("formatJSNumber(%v) = %q want %q", in, got, want)
		}
	}
	if jsonStringify("a\"<b>") != `"a\"<b>"` || jsonStringify(nil) != "null" || jsonStringify(1.0) != "1" {
		t.Fatal("jsonStringify")
	}
	if jsTypeof(1) != "number" || jsTypeof("s") != "string" || jsTypeof(true) != "boolean" || jsTypeof(nil) != "undefined" || jsTypeof(map[string]any{}) != "object" {
		t.Fatal("jsTypeof")
	}
}

func TestFieldOf(t *testing.T) {
	if v, ok := fieldOf(map[string]any{"a": 1}, "a"); !ok || v != 1 {
		t.Fatal("map")
	}
	if _, ok := fieldOf(map[string]any{"a": 1}, "b"); ok {
		t.Fatal("missing")
	}
	if v, ok := fieldOf(map[string]any{"a": nil}, "a"); !ok || v != nil {
		t.Fatal("explicit nil is present")
	}
	if v, ok := fieldOf(PanelVerdict{Pass: true}, "pass"); !ok || v != true {
		t.Fatal("struct by json tag")
	}
	if v, ok := fieldOf(&FailedOutputData{Reason: "r"}, "reason"); !ok || v != "r" {
		t.Fatal("pointer to struct")
	}
	if v, ok := fieldOf(map[string]float64{"n": 2}, "n"); !ok || v != 2.0 {
		t.Fatal("typed map")
	}
	if _, ok := fieldOf(nil, "a"); ok {
		t.Fatal("nil data")
	}
	if _, ok := fieldOf("str", "a"); ok {
		t.Fatal("scalar data")
	}
	if !jsStrictEqual(1, 1.0) || jsStrictEqual("1", 1) || !jsStrictEqual("a", "a") || jsStrictEqual(true, 1) || !jsStrictEqual(nil, nil) {
		t.Fatal("strict equal")
	}
}
