package workflow

// NumericPredicate is what the gate(...) helpers return — a side-effect-free
// test over a single field value the runtime evaluates against
// Number(output.data[field]). A missing or non-numeric field coerces to NaN,
// and NaN compares false against every threshold, so no branch fires
// spuriously; gate's explicit `otherwise` handles the unmatched case.
type NumericPredicate func(value float64) bool

// Predicate is the deprecated pre-rename alias of NumericPredicate (the bare
// name collided with route predicates). Ships for one release.
type Predicate = NumericPredicate

// Gt is strictly greater than.
func Gt(n float64) NumericPredicate { return func(v float64) bool { return v > n } }

// Gte is greater than or equal.
func Gte(n float64) NumericPredicate { return func(v float64) bool { return v >= n } }

// Lt is strictly less than.
func Lt(n float64) NumericPredicate { return func(v float64) bool { return v < n } }

// Lte is less than or equal.
func Lte(n float64) NumericPredicate { return func(v float64) bool { return v <= n } }

// Eq is numeric equality (after Number(...) coercion of output.data[field]).
func Eq(n float64) NumericPredicate { return func(v float64) bool { return v == n } }
