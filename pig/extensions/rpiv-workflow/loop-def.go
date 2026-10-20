package workflow

import "context"

// Loop vocabulary — the data shapes behind StageDef.Loop and StageDef.Verify.
// Constructors live in loops-constructors.go (Fanout / Iterate / Assess /
// Verify); the per-kind runtime strategy table lives in the loops package.

// Unit is one unit of loop work — the single unit type for all three loop
// kinds. Prompt is the body sent to the skill; Label is the human display tag;
// ID is the stable audit identity written to the row's unitId (falls back to
// Label). Deps (fanout only) are the `id ?? label` identities of the units
// this unit depends on: the wave scheduler orders dispatch over them; an
// unknown reference or a cycle halts the stage at the live entry.
type Unit struct {
	Prompt string
	Label  string
	ID     string
	Deps   []string
}

// UnitIdentity is the unit's stable audit identity: ID, falling back to Label.
func (u Unit) UnitIdentity() string {
	if u.ID != "" {
		return u.ID
	}
	return u.Label
}

// FanoutContext is handed to a FanoutFn.
type FanoutContext struct {
	Cwd string
	// Artifact is the primary inherited from upstream (nil for an entry stage).
	Artifact *Artifact
	State    RunView
}

// FanoutFn is the push-model unit source: all units computed up front, each
// run blind to the others. An empty return ⇒ no loop (single-stage
// fall-through); an error halts the stage, attributed to it (a returned
// *StagePreflightError keeps its own attribution).
type FanoutFn func(ctx context.Context, fc FanoutContext) ([]Unit, error)

// IterateContext is handed to an IterateFn.
type IterateContext struct {
	Cwd string
	// Artifact is the stage-entry primary, FROZEN across every unit.
	Artifact *Artifact
	State    RunView
	// Accumulated is the validated Outputs of this generation's completed units, in order.
	Accumulated []Output
	// Index is the 0-based index of the unit about to run (== len(Accumulated)).
	Index int
}

// IterateFn is the pull-model unit source: invoked once per unit with the
// validated Outputs of every prior unit; returns the next unit, or nil to
// terminate. RESUME CONTRACT: deterministic w.r.t. the fold-replayed state +
// the generation's accumulated outputs — the fold re-calls it and refuses on drift.
type IterateFn func(ctx context.Context, ic IterateContext) (*Unit, error)

// FeedForwardContext is handed to AssessLoop.FeedForward and VerifySpec.FeedForward.
type FeedForwardContext struct {
	Cwd string
	// Output is the producer output just judged.
	Output Output
	// Verdict is the judge verdict (incl. feedback).
	Verdict Output
	// Round is the 0-based round index of the round just judged (== the attempt index for verify).
	Round int
	State RunView
}

// DoneFn reads the model-made verdict: true → the repetition stops.
type DoneFn func(verdict Output) bool

// FeedForwardFn builds the next round/attempt's prompt arg from the just-judged pair.
type FeedForwardFn func(ctx context.Context, fc FeedForwardContext) (string, error)

// JudgedRepetition is the shared "repeat under a model judge" vocabulary —
// ONE set of field names for AssessLoop rounds and VerifySpec attempts.
// Judge is the judge SLOT (a single judge or a panel); Done decides
// termination (deterministic w.r.t. the verdict, recomputed on resume; a true
// on the final round is a normal completion); FeedForward builds the next
// prompt arg (on a prompt-dispatch stage it is the COMPLETE retry message);
// Max is the repetition cap (integer ≥ 1, clamped by run.maxIterations).
type JudgedRepetition struct {
	Judge       AnyJudge
	Done        DoneFn
	FeedForward FeedForwardFn
	Max         *int
}

// VerifySpec is the per-stage post-condition judge behind StageDef.Verify.
// After each attempt completes, the judge grades the attempt's primary
// artifact; Done true → verified; false → a fresh retry attempt (prompt arg
// from FeedForward) up to Max attempts (default 1 = gate-only), then a
// terminal "verification failed" halt. FeedForward is REQUIRED when Max > 1.
// Runtime desugars it into a degenerate assess loop (SynthesizeVerifyLoop).
type VerifySpec = JudgedRepetition

// UnitRole is the role a unit row/event carries: every main-work unit is
// "produce"; assess judge sub-steps are "judge"; verify verdict sub-steps are "verify".
type UnitRole string

const (
	RoleProduce UnitRole = "produce"
	RoleJudge   UnitRole = "judge"
	RoleVerify  UnitRole = "verify"
)

// CapPolicy is what happens when a loop hits its effective cap: "halt" is a
// terminal failure; "advance" is a soft-stop (warn, loop-cap telemetry row,
// onLoopCap, keep the projected result, advance).
type CapPolicy string

const (
	CapHalt    CapPolicy = "halt"
	CapAdvance CapPolicy = "advance"
)

// ResultProjection is what the loop leaves in {state.output, state.primaryArtifact}:
// "entry" restores the pair captured at loop entry; "last" is the last
// completed produce unit's pair (zero produce units degrades to entry).
type ResultProjection string

const (
	ResultEntry ResultProjection = "entry"
	ResultLast  ResultProjection = "last"
)

// UnitSelector says how a stage's work is split into units, AS DATA. The
// framework never interprets By / Pattern — they are introspection hints.
type UnitSelector struct {
	By      string         `json:"by"`
	Pattern string         `json:"pattern,omitempty"`
	Meta    map[string]any `json:"meta,omitempty"`
}

// LoopKind names a loop kind.
type LoopKind string

const (
	LoopFanout  LoopKind = "fanout"
	LoopIterate LoopKind = "iterate"
	LoopAssess  LoopKind = "assess"
)

// LoopKinds is the runtime enumeration the validator (kind whitelist) and the
// per-kind strategy table consume.
var LoopKinds = []LoopKind{LoopFanout, LoopIterate, LoopAssess}

// LoopCommon is the introspectable data common to all three loop kinds.
type LoopCommon struct {
	// Source is the named channel the units are split FROM (a consumes signal).
	Source string
	// Unit says how units are detected (opaque convention).
	Unit *UnitSelector
	// Max is the cardinality ceiling; clamped by run.maxIterations at runtime.
	Max    *int
	OnCap  CapPolicy
	Result ResultProjection
}

// Common returns the shared facet.
func (c *LoopCommon) Common() *LoopCommon { return c }

// LoopDef is the single loop field's value — a DATA object with
// function-valued fields, introspectable by construction (LoopSpecOf).
// Implemented by *FanoutLoop, *IterateLoop and *AssessLoop.
type LoopDef interface {
	Kind() LoopKind
	Common() *LoopCommon
}

// FanoutLoop is the parallel-shaped push loop — units run in bounded parallel.
type FanoutLoop struct {
	LoopCommon
	Units FanoutFn
	// Concurrency caps in-flight units to min(Concurrency, host
	// maxConcurrency); 1 SERIALISES the loop — the safe model for units that
	// mutate shared state with unknown write-sets. nil ⇒ the host cap governs.
	Concurrency *int
	// FailFast opts out of collect-all: any unit failure halts the run and
	// cancels in-flight siblings. nil/false ⇒ collect-all.
	FailFast *bool
	// HaltWhenAllFailed halts terminally when a closing generation's every
	// declared slot is a failed sentinel instead of advancing into a fan-in
	// over an empty channel. FailFast wins when both are set.
	HaltWhenAllFailed *bool
	// RetryHaltedUnits re-dispatches a soft-halted collect-all unit up to N
	// MORE times; only the final attempt's output folds. Integer ≥ 1.
	RetryHaltedUnits *int
	// DepArtifactFlag, when set, injects each completed dependency's published
	// artifact path into the dependent unit's prompt as `<flag> <path>`.
	DepArtifactFlag string
}

// Kind implements LoopDef.
func (*FanoutLoop) Kind() LoopKind { return LoopFanout }

// IterateLoop is the sequential accumulating pull loop.
type IterateLoop struct {
	LoopCommon
	Next IterateFn
}

// Kind implements LoopDef.
func (*IterateLoop) Kind() LoopKind { return LoopIterate }

// AssessLoop is the model-judged until-done loop: producer→judge rounds.
// Shares the JudgedRepetition vocabulary with VerifySpec; here Max (the round
// cap) and FeedForward are REQUIRED — the Assess constructor defaults Max to
// DefaultAssessMax and rejects a missing FeedForward.
type AssessLoop struct {
	LoopCommon
	Judge       AnyJudge
	Done        DoneFn
	FeedForward FeedForwardFn
}

// Kind implements LoopDef.
func (*AssessLoop) Kind() LoopKind { return LoopAssess }

// RoundCap is the assess round cap (always set by the constructor; 0 only on
// a hand-rolled literal that omitted it).
func (a *AssessLoop) RoundCap() int {
	if a.Max == nil {
		return 0
	}
	return *a.Max
}
