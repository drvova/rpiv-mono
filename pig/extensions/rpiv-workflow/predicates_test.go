package workflow

import (
	"math"
	"testing"
)

func TestNumericPredicates(t *testing.T) {
	cases := []struct {
		name  string
		p     NumericPredicate
		truth map[float64]bool
	}{
		{"gt strictly greater", Gt(5), map[float64]bool{6: true, 5: false, 4: false}},
		{"gte boundary included", Gte(5), map[float64]bool{6: true, 5: true, 4: false}},
		{"lt strictly less", Lt(5), map[float64]bool{4: true, 5: false, 6: false}},
		{"lte boundary included", Lte(5), map[float64]bool{4: true, 5: true, 6: false}},
		{"eq numeric equality", Eq(5), map[float64]bool{5: true, 6: false, 4: false}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for v, want := range c.truth {
				if c.p(v) != want {
					t.Fatalf("p(%v) = %v want %v", v, c.p(v), want)
				}
			}
			if c.p(math.NaN()) {
				t.Fatal("NaN must compare false")
			}
		})
	}
	var alias Predicate = Gt(5)
	if !alias(6) {
		t.Fatal("deprecated alias")
	}
}
