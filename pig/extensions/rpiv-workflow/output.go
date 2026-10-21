package workflow

import (
	"encoding/json"
)

// OutputMeta is the metadata stamped on every Output envelope. Field order is
// the persisted JSON order; Skill is omitted (not null) for script stages.
type OutputMeta struct {
	// Stage is the workflow stage record key — matches WorkflowStage.Stage.
	Stage string `json:"stage"`
	// Skill is the Pi skill body when the producing stage was skill-based;
	// absent for script stages. Matches WorkflowStage.Skill.
	Skill string `json:"skill,omitempty"`
	// StageNumber is 1-based; matches WorkflowStage.StageNumber.
	StageNumber int `json:"stageNumber"`
	// Ts is ISO-8601.
	Ts string `json:"ts"`
	// RunID is duplicated from the header for ergonomic JSONL reads.
	RunID string `json:"runId"`
}

// Output is one stage's contribution to the chain. Artifacts is always
// present (possibly empty for side-effect stages); Data is whatever the parser
// shaped (or the artifact list itself when no parser is wired). Kind
// discriminates the data shape so downstream consumers narrow on it; the
// literal "artifacts" is the default parser-less shape.
type Output struct {
	Kind      string     `json:"kind"`
	Artifacts []Artifact `json:"artifacts"`
	Data      any        `json:"data"`
	Meta      OutputMeta `json:"meta"`
}

// MarshalJSON writes Artifacts as an array even when nil, so a Go zero value
// serialises to the `"artifacts": []` every TypeScript row carries — the
// reader indexes artifacts[0] and filters the list, and never meets null.
func (o Output) MarshalJSON() ([]byte, error) {
	type plain Output
	if o.Artifacts == nil {
		o.Artifacts = []Artifact{}
	}
	return json.Marshal(plain(o))
}

// Built-in output kinds — the two framework-native shapes. Outcome-specific
// kinds live with their producing outcome.
const (
	ArtifactsOutputKind  = "artifacts"
	SideEffectOutputKind = "side-effect"
)

// Verdict is a judge's graded output — structurally just an Output (the
// verdict is a normal envelope published to the judge's named channel), named
// so judged signatures can say what they mean.
type Verdict = Output

// RunView is the read surface over a live run that user-authored functions
// (edge predicates, script stages, loop unit sources, judges, collectors)
// receive as ctx.State. The runner hands its live *RunState here; the
// interface keeps a user function from writing runner bookkeeping. Named
// returns the live registry — treat it as read-only.
type RunView interface {
	// OriginalInput is the user's original `/wf` argument — frozen for the run.
	OriginalInput() string
	// Output is the most recent completed stage's full envelope (nil before any).
	Output() *Output
	// Named is the publish registry — each produces success APPENDS its
	// envelope onto the slot named by stage.outcome.name ?? stage record key.
	// Slots are histories; read the latest via the last element.
	Named() map[string][]Output
}

// FinalizeOutput is the single source of output metadata authorship. The
// runner calls it after a stage's collector returned artifacts and the parser
// (or parser-less default) returned {kind, data}. Nothing is cloned: the
// artifacts slice and data value pass through by reference, except that a
// nil artifacts slice becomes the empty list (an envelope's artifacts is
// always an array).
func FinalizeOutput(kind string, artifacts []Artifact, data any, meta OutputMeta) Output {
	if artifacts == nil {
		artifacts = []Artifact{}
	}
	return Output{Kind: kind, Artifacts: artifacts, Data: data, Meta: meta}
}

// OutputMetaArgs are the inputs of OutputMeta's one construction home.
type OutputMetaArgs struct {
	Stage       string
	Skill       string
	StageNumber int
	Ts          string
	RunID       string
}

// NewOutputMeta is the single construction home for an OutputMeta literal, so
// the live path's minted meta and the resume fold's rebuilt sentinel cannot
// diverge structurally. Ts is a PARAMETER: live call sites pass NowIso(); the
// resume fold replays the persisted row's ts. An empty Skill stays absent from
// the serialised row (the script-row "no skill field" contract).
func NewOutputMeta(args OutputMetaArgs) OutputMeta {
	return OutputMeta{
		Stage:       args.Stage,
		Skill:       args.Skill,
		StageNumber: args.StageNumber,
		Ts:          args.Ts,
		RunID:       args.RunID,
	}
}

// FailedOutputKind is the data kind of a failed-unit sentinel — collect-all
// fanout places one in a unit's declared slot when the unit halts.
const FailedOutputKind = "failed"

// FailedOutputData is the data shape of a failed-unit sentinel. Dimension is
// the failed unit's label; its key is omitted when no dimension is given so a
// dimensionless sentinel stays byte-identical to the prior on-disk shape.
type FailedOutputData struct {
	Reason    string `json:"reason"`
	Dimension string `json:"dimension,omitempty"`
}

// FailedOutput builds a failed unit's contribution to a collect-all fanout: a
// real Output with NO artifacts, so the rolling primary is left alone, fanin
// readers contribute no args for it, the cursor advances without making it
// the "last" produce, and the resume fold replays it like any produce row. A
// dimension-bearing sentinel blocks its dimension at every gate fold; a
// dimensionless one is skipped by the fold.
func FailedOutput(meta OutputMeta, reason, dimension string) Output {
	return FinalizeOutput(FailedOutputKind, []Artifact{}, FailedOutputData{Reason: reason, Dimension: dimension}, meta)
}

// IsFailedOutput discriminates the collect-all sentinel from a normal Output.
func IsFailedOutput(o Output) bool { return o.Kind == FailedOutputKind }
