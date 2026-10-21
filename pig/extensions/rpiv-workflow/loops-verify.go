package workflow

import (
	"context"
	"errors"
	"math"
)

// Verify — the per-stage post-condition desugared to a degenerate assess
// loop. Verify builds the VerifySpec a stage carries; SynthesizeVerifyLoop is
// the desugar the runner consults (via EffectiveLoopOf).

// Verify validates the per-stage post-condition judge: after each attempt of
// the stage completes, Judge grades it and Done gates advancement — true →
// advance; false → fresh retry attempt (prompt arg from FeedForward) up to
// Max attempts (default 1 = gate-only), then a terminal "verification
// failed" halt. Requires Kind "produces" + a named outcome (checked at load).
func Verify(spec VerifySpec) (*VerifySpec, error) {
	if err := AssertShape("verify", VerifyShapeIssues(&spec)); err != nil {
		return nil, err
	}
	return &spec, nil
}

// VerifyShapeIssues is the single rule source for the verify shape. Accepts
// *VerifySpec, VerifySpec, a map literal or nil. The map-literal arm reads
// `max` as the dynamic value it is (a fraction, a string, null), so the
// report renders what the author wrote.
func VerifyShapeIssues(candidate any) []string {
	var judge, maxRaw any
	var hasDone, maxPresent, feedForwardPresent, feedForwardIsFn bool
	switch v := candidate.(type) {
	case *VerifySpec:
		if v == nil {
			return []string{"a verify object is required"}
		}
		judge, hasDone = v.Judge, v.Done != nil
		if v.Max != nil {
			maxRaw, maxPresent = *v.Max, true
		}
		feedForwardPresent, feedForwardIsFn = v.FeedForward != nil, v.FeedForward != nil
	case VerifySpec:
		judge, hasDone = v.Judge, v.Done != nil
		if v.Max != nil {
			maxRaw, maxPresent = *v.Max, true
		}
		feedForwardPresent, feedForwardIsFn = v.FeedForward != nil, v.FeedForward != nil
	case map[string]any:
		if v == nil {
			return []string{"a verify object is required"}
		}
		judge = v["judge"]
		hasDone = isDoneFn(v["done"])
		maxRaw, maxPresent = v["max"]
		var raw any
		raw, feedForwardPresent = v["feedForward"]
		feedForwardIsFn = isFeedForwardFn(raw)
	default:
		return []string{"a verify object is required"}
	}
	// The judge slot is an AnyJudge — route the panel-vs-single dispatch
	// through the shared JudgeSlotShapeIssues (one rule source per shape).
	issues := append([]string{}, JudgeSlotShapeIssues(judge)...)
	if !hasDone {
		issues = append(issues, "verify requires `done` to be a function deciding pass/fail from the verdict")
	}
	if maxPresent && !isPositiveInteger(maxRaw) {
		rendered := "null"
		if maxRaw != nil {
			rendered = jsString(maxRaw)
		}
		issues = append(issues, "verify.max: "+rendered+" — must be an integer >= 1 (run.maxIterations caps the upper bound)")
	}
	if feedForwardPresent && !feedForwardIsFn {
		issues = append(issues, "verify `feedForward` must be a function building the next attempt's prompt arg")
	}
	effectiveMax := 1.0
	if maxPresent {
		effectiveMax = jsNumber(maxRaw)
	}
	if effectiveMax > 1 && !feedForwardPresent {
		issues = append(issues, "verify.max > 1 requires `feedForward` — without it the retried prompt would be byte-identical to the original")
	}
	return issues
}

// isPositiveInteger is `Number.isInteger(v) && v >= 1` over a dynamic value.
func isPositiveInteger(v any) bool {
	f, ok := asFloat(v)
	return ok && !math.IsInf(f, 0) && f == math.Trunc(f) && f >= 1
}

// neverFeedForward is unreachable by construction: the driver's cap check
// precedes the feedForward call, so a gate-only verify (Max 1, the only
// shape allowed to omit FeedForward) caps before a second attempt could ask
// for an arg. Reaching it means that invariant broke — fail loudly.
func neverFeedForward(context.Context, FeedForwardContext) (string, error) {
	return "", errors.New("verify: feedForward invoked on a gate-only verify (max 1) — driver invariant violated")
}

// SynthesizeVerifyLoop is the desugar: a VerifySpec as a degenerate assess
// loop — the shared JudgedRepetition fields flow straight through (Max
// defaulted to 1), the cap policy is always "halt" (Done wins over the cap)
// and Result "last" restores the last attempt's producer pair. Allocates per
// call; callers cache.
func SynthesizeVerifyLoop(v *VerifySpec) *AssessLoop {
	d := VerifyLoopDefaults
	feedForward := v.FeedForward
	if feedForward == nil {
		feedForward = neverFeedForward
	}
	maxCap := v.Max
	if maxCap == nil {
		maxCap = Int(*d.Max)
	} else {
		maxCap = Int(*maxCap)
	}
	return &AssessLoop{
		LoopCommon:  LoopCommon{Max: maxCap, OnCap: d.OnCap, Result: d.Result},
		Judge:       v.Judge,
		Done:        v.Done,
		FeedForward: feedForward,
	}
}
