package workflow

// The root package IS the barrel: every authoring, embedding and inspection
// name the TypeScript api.ts / registration.ts entries re-export from the
// vocabulary modules is declared here directly. This file carries the small
// helpers Go literals need to express optional fields and value-returning
// constructors inline.

// Bool returns a pointer to v — for optional boolean fields (InheritsArtifacts, FailFast, …).
func Bool(v bool) *bool { return &v }

// Int returns a pointer to v — for optional integer fields (Max, MaxRetries, Concurrency, …).
func Int(v int) *int { return &v }

// String returns a pointer to v — for optional string fields (MatchOptions.Fallback, DepArtifactFlag, …).
func String(v string) *string { return &v }

// Must unwraps a value-returning constructor (Gate, Match, Fanout, …) inside
// a workflow literal, panicking on the construction error the TypeScript
// factory would have thrown at authoring time. Use it only where the
// definition is static program text; load-time validation re-checks every
// rule for values that bypass the constructors.
func Must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
