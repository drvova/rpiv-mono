package workflow

import (
	"context"
	"errors"
	"strconv"
)

// Loop constructors — Fanout / Iterate / Assess build the LoopDef a stage
// carries on its single Loop field, validating at construction (the
// DefineRoute pattern): a bad Max, an invalid judge shape or a missing Done /
// FeedForward fails immediately at authoring time; load-time validation
// re-checks the same rules for hand-rolled values. Skill-agnostic — the unit
// detectors (Units / Next) are consumer-supplied.

// DefaultAssessMax is the default round cap when an Assess call omits Max.
const DefaultAssessMax = 8

// LoopKindDefaults is the per-kind defaults row: construction-default cap
// policy and result projection, the construction-default cap (nil when a
// kind has none) and whether the kind freezes a round-0 producer arg.
type LoopKindDefaults struct {
	OnCap            CapPolicy
	Result           ResultProjection
	Max              *int
	FreezesEntryArgs bool
}

// LoopDefaults is THE per-kind identity table — the single place fanout /
// iterate / assess default OnCap / Result / Max AND the round-0-arg rule are
// spelled. Consulted by every constructor and by EffectiveLoopOf /
// FreezesEntryArgsOf, so a default change is a single-row edit.
var LoopDefaults = map[LoopKind]LoopKindDefaults{
	LoopFanout:  {OnCap: CapHalt, Result: ResultEntry, FreezesEntryArgs: false},
	LoopIterate: {OnCap: CapHalt, Result: ResultLast, FreezesEntryArgs: false},
	LoopAssess:  {OnCap: CapAdvance, Result: ResultLast, Max: Int(DefaultAssessMax), FreezesEntryArgs: true},
}

// VerifyLoopDefaults are verify's gate-only defaults — a named PEER, not a
// LoopDefaults row, because verify is not a LoopKind (it desugars to a
// degenerate assess loop). Consulted by SynthesizeVerifyLoop. FreezesEntryArgs
// is carried for shape but not consulted by FreezesEntryArgsOf (a verify
// stage's effective loop is the synthesized assess loop).
var VerifyLoopDefaults = LoopKindDefaults{OnCap: CapHalt, Result: ResultLast, Max: Int(1), FreezesEntryArgs: true}

// firstDefined keeps defaulting grep-clean: the single authorised spelling of
// the "value or fallback" rule the constructors use.
func firstDefined[T comparable](v, fallback T) T {
	var zero T
	if v == zero {
		return fallback
	}
	return v
}

// LoopOptionsBase are the options shared by all three constructors.
type LoopOptionsBase struct {
	// Source is the named channel the units are split FROM (a consumes hint).
	Source string
	// Unit says how units are detected (opaque convention).
	Unit *UnitSelector
	// Max is the cardinality ceiling. Integer ≥ 1 (construction error otherwise).
	Max *int
	// OnCap overrides the cap policy (fanout/iterate → halt, assess → advance).
	OnCap CapPolicy
	// Result overrides the projection (fanout → entry, iterate/assess → last).
	Result ResultProjection
}

// FanoutOptions are the inputs of Fanout.
type FanoutOptions struct {
	LoopOptionsBase
	// Units is the push-model unit source — all units computed up front.
	Units FanoutFn
	// Concurrency caps in-flight units; 1 serialises. Integer ≥ 1.
	Concurrency *int
	// FailFast opts out of collect-all.
	FailFast *bool
	// HaltWhenAllFailed halts when a generation closes all-failed.
	HaltWhenAllFailed *bool
	// RetryHaltedUnits re-dispatches a soft-halted unit up to N more times. Integer ≥ 1.
	RetryHaltedUnits *int
	// DepArtifactFlag hands a dependent unit its dependencies' artifacts as `<flag> <path>`.
	DepArtifactFlag *string
}

// IterateOptions are the inputs of Iterate.
type IterateOptions struct {
	LoopOptionsBase
	// Next is the pull-model unit source — one unit per call.
	Next IterateFn
}

// AssessOptions are the inputs of Assess.
type AssessOptions struct {
	LoopOptionsBase
	// Judge is the judge SLOT — a single judge or an N-member Panel.
	Judge AnyJudge
	// Done reads the verdict: true → the loop stops.
	Done DoneFn
	// FeedForward builds the next producer prompt arg from the judged pair.
	FeedForward FeedForwardFn
}

// Fanout builds the push loop: all units precomputed, each unit its own
// session. On a produces stage units COLLECT (outcome.name required); on an
// acts stage units are side-effects. An empty Units return ⇒ single-stage
// fall-through.
func Fanout(opts FanoutOptions) (*FanoutLoop, error) {
	d := LoopDefaults[LoopFanout]
	maxCap, err := checkedPositiveInt(opts.Max, "fanout(): max")
	if err != nil {
		return nil, err
	}
	loop := &FanoutLoop{
		LoopCommon: LoopCommon{
			Source: opts.Source,
			Unit:   opts.Unit,
			Max:    maxCap,
			OnCap:  firstDefined(opts.OnCap, d.OnCap),
			Result: firstDefined(opts.Result, d.Result),
		},
		Units:             opts.Units,
		FailFast:          opts.FailFast,
		HaltWhenAllFailed: opts.HaltWhenAllFailed,
	}
	if loop.Concurrency, err = checkedPositiveInt(opts.Concurrency, "fanout(): concurrency"); err != nil {
		return nil, err
	}
	if opts.DepArtifactFlag != nil {
		if err := checkedDepArtifactFlag(*opts.DepArtifactFlag); err != nil {
			return nil, err
		}
		loop.DepArtifactFlag = *opts.DepArtifactFlag
	}
	if loop.RetryHaltedUnits, err = checkedPositiveInt(opts.RetryHaltedUnits, "fanout(): retryHaltedUnits"); err != nil {
		return nil, err
	}
	return loop, nil
}

// checkedDepArtifactFlag rejects a blank/whitespace flag at construction — it
// would inject a bare ` <path>` with no marker.
func checkedDepArtifactFlag(flag string) error {
	return RequireNonEmptyString(flag, "fanout()", "depArtifactFlag must be a non-empty string (got "+jsonStringify(flag)+")", true)
}

// Iterate builds the pull loop: sequential, accumulating — each Next call
// sees the prior units' validated Outputs. First-call nil ⇒ zero-unit no-op.
func Iterate(opts IterateOptions) (*IterateLoop, error) {
	d := LoopDefaults[LoopIterate]
	maxCap, err := checkedPositiveInt(opts.Max, "iterate(): max")
	if err != nil {
		return nil, err
	}
	return &IterateLoop{
		LoopCommon: LoopCommon{
			Source: opts.Source,
			Unit:   opts.Unit,
			Max:    maxCap,
			OnCap:  firstDefined(opts.OnCap, d.OnCap),
			Result: firstDefined(opts.Result, d.Result),
		},
		Next: opts.Next,
	}, nil
}

// Assess builds the model-judged until-done loop: each round runs a producer
// session then a judge session; Done decides termination; FeedForward carries
// the verdict into the next round. The cap soft-stops by default.
func Assess(opts AssessOptions) (*AssessLoop, error) {
	if err := AssertShape("assess", AssessShapeIssues(opts)); err != nil {
		return nil, err
	}
	d := LoopDefaults[LoopAssess]
	maxCap, err := checkedPositiveInt(opts.Max, "assess(): max")
	if err != nil {
		return nil, err
	}
	if maxCap == nil {
		maxCap = Int(*d.Max)
	}
	return &AssessLoop{
		LoopCommon: LoopCommon{
			Source: opts.Source,
			Unit:   opts.Unit,
			Max:    maxCap,
			OnCap:  firstDefined(opts.OnCap, d.OnCap),
			Result: firstDefined(opts.Result, d.Result),
		},
		Judge:       opts.Judge,
		Done:        opts.Done,
		FeedForward: opts.FeedForward,
	}, nil
}

// JudgeSlotShapeIssues is THE panel-vs-single judge-slot shape dispatch: a
// panel routes to PanelShapeIssues, anything else (a single judge, a map
// literal, nil) to JudgeShapeIssues — so a missing judge reads "a judge
// object is required" rather than failing on a nil access.
func JudgeSlotShapeIssues(slot any) []string {
	switch p := slot.(type) {
	case *PanelJudge:
		if p != nil {
			return PanelShapeIssues(p)
		}
	case PanelJudge:
		return PanelShapeIssues(&p)
	case map[string]any:
		if kind, _ := p["kind"].(string); kind == "panel" {
			return PanelShapeIssues(p)
		}
	}
	return JudgeShapeIssues(slot)
}

// AssessShapeIssues is the single rule source for the assess shape (the
// construction-time twin of the load gate's per-code reports). Accepts an
// AssessOptions, an *AssessLoop or a map literal.
func AssessShapeIssues(candidate any) []string {
	var judge any
	var hasDone, hasFeedForward bool
	switch a := candidate.(type) {
	case AssessOptions:
		judge, hasDone, hasFeedForward = a.Judge, a.Done != nil, a.FeedForward != nil
	case *AssessOptions:
		if a == nil {
			return []string{"an assess object is required"}
		}
		judge, hasDone, hasFeedForward = a.Judge, a.Done != nil, a.FeedForward != nil
	case *AssessLoop:
		if a == nil {
			return []string{"an assess object is required"}
		}
		judge, hasDone, hasFeedForward = a.Judge, a.Done != nil, a.FeedForward != nil
	case map[string]any:
		if a == nil {
			return []string{"an assess object is required"}
		}
		judge = a["judge"]
		hasDone = isDoneFn(a["done"])
		hasFeedForward = isFeedForwardFn(a["feedForward"])
	default:
		return []string{"an assess object is required"}
	}
	issues := append([]string{}, JudgeSlotShapeIssues(judge)...)
	if !hasDone {
		issues = append(issues, "`done` must be a function deciding termination from the verdict")
	}
	if !hasFeedForward {
		issues = append(issues, "`feedForward` must be a function building the next producer arg")
	}
	return issues
}

// isDoneFn / isFeedForwardFn are the `typeof x === "function"` tests of the
// map-literal arms: the named function type and its underlying signature
// both read as the function they are; a nil function does not.
func isDoneFn(v any) bool {
	switch f := v.(type) {
	case DoneFn:
		return f != nil
	case func(Output) bool:
		return f != nil
	}
	return false
}

func isFeedForwardFn(v any) bool {
	switch f := v.(type) {
	case FeedForwardFn:
		return f != nil
	case func(context.Context, FeedForwardContext) (string, error):
		return f != nil
	}
	return false
}

// checkedPositiveInt is the construction-time positive-int gate for Max /
// Concurrency / RetryHaltedUnits — the single spelling of "integer >= 1".
// The label is the full origin+field prefix (`fanout(): max`); the message is
// `<label> must be an integer >= 1 (got <value>)` — a plain error, no
// `origin:` prefix, matching the TypeScript throw.
func checkedPositiveInt(value *int, label string) (*int, error) {
	if value == nil {
		return nil, nil
	}
	if *value < 1 {
		return nil, errors.New(label + " must be an integer >= 1 (got " + strconv.Itoa(*value) + ")")
	}
	return Int(*value), nil
}
