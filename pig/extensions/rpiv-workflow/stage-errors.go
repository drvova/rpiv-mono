package workflow

// PreflightKind annotates a StagePreflightError's violation class — for
// diagnostics only; control flow at the catch site is uniform.
type PreflightKind string

const (
	// PreflightHalt is a runtime-state failure (skill not registered, missing
	// upstream artifact, schema mismatch).
	PreflightHalt PreflightKind = "halt"
	// PreflightInvariant is an authoring-time-knowable violation the validator
	// should reject at load; reaching it means validation was bypassed or the
	// rule lives only in the runner.
	PreflightInvariant PreflightKind = "invariant"
)

// StagePreflightError is returned by a preflight check on failure; it carries
// the recorded-row attribution plus the notify/err messages so the guarded
// stage entry can land a uniform JSONL row regardless of which slot tripped.
// It is a package-root leaf so both the loop-primitive layer and the runner's
// per-stage pipeline can return it without importing across the runner boundary.
type StagePreflightError struct {
	Kind          PreflightKind
	Skill         string
	NotifyMsg     string
	ErrMsg        string
	NotifyPartial bool
}

// Error implements error with the recorded errMsg.
func (e *StagePreflightError) Error() string { return e.ErrMsg }

// preflightErrorOf is the ONE construction site — NotifyPartial is DERIVED
// from Kind (halt → true, invariant → false) so a status/level mismatch is
// unrepresentable.
func preflightErrorOf(kind PreflightKind, skill, notifyMsg, errMsg string) *StagePreflightError {
	return &StagePreflightError{
		Kind:          kind,
		Skill:         skill,
		NotifyMsg:     notifyMsg,
		ErrMsg:        errMsg,
		NotifyPartial: kind == PreflightHalt,
	}
}

// HaltPreflight is pattern A — a runtime-state failure built from one
// FailureText so the toast/error pairing lives in the Fail* factory.
func HaltPreflight(skill string, failure FailureText) *StagePreflightError {
	return preflightErrorOf(PreflightHalt, skill, failure.Toast, failure.Error)
}

// InvariantPreflight is pattern B — an authoring-time-knowable violation, in
// the single-message form (notifyMsg == errMsg).
func InvariantPreflight(name, msg string) *StagePreflightError {
	return preflightErrorOf(PreflightInvariant, name, msg, msg)
}

// InvariantPreflightSplit is the two-message form of InvariantPreflight: a
// framed user message with the raw reason preserved as errMsg.
func InvariantPreflightSplit(name, notifyMsg, errMsg string) *StagePreflightError {
	return preflightErrorOf(PreflightInvariant, name, notifyMsg, errMsg)
}
