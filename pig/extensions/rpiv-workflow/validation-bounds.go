package workflow

// Output-validation policy bounds — the retry-count and per-attempt timeout
// clamps shared by the runtime retry loop and the load-time stage-rule
// validator. A dependency-free leaf so the validator reads the bounds
// without importing the runtime retry engine.
const (
	MinValidationRetries     = 1
	MaxValidationRetries     = 3
	DefaultValidationRetries = 1

	DefaultValidationRetryTimeoutMs = 5 * 60 * 1000
	// MaxValidationRetryTimeoutMs is the hard ceiling for a single validation
	// attempt; mirrors the bash tool's 30-minute ceiling, separately owned.
	MaxValidationRetryTimeoutMs = 30 * 60 * 1000
	MinValidationRetryTimeoutMs = 1_000
)

// ClampRange clamps raw to [min, max], defaulting to def when raw is nil. The
// ONE runtime-clamp idiom shared by the retry loop and the input preflight:
// the load-time validator rejects out-of-range values, but programmatic
// callers can bypass it, and clamping here means a misconfigured stage
// degrades to the spec-default behaviour instead of firing a 100ms timeout.
func ClampRange(raw *int, lo, def, hi int) int {
	v := def
	if raw != nil {
		v = *raw
	}
	return max(lo, min(v, hi))
}
