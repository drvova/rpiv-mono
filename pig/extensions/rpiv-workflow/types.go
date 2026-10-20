package workflow

import "context"

// Runtime types. Three nouns flow through the workflow runtime:
//   - RunContext — per-run carry (cwd, runId, workflow, state, visited,
//     registeredSkills, resolveModel, budgets). Read by every layer; mutated
//     only by the runner.
//   - RunState — mutable bookkeeping (output, counters, telemetry,
//     termination). Mutated through the chain-state authorities by the
//     runner, the loop driver, the audit layer and the resume fold; external
//     consumers get the RunView projection instead.
//   - WorkflowHostContext — the host port (host.go), threaded from
//     withSession callbacks down through stage/phase helpers.
//
// Per-stage / per-unit sessions extend the shared SessionContext base.

// TerminationStatus is the discriminator of RunTermination.
type TerminationStatus string

const (
	// TerminationRunning — not terminated yet (also: the runner unwound without
	// reaching any terminal write — treated as failure).
	TerminationRunning TerminationStatus = "running"
	// TerminationCompleted — the chain reached stop.
	TerminationCompleted TerminationStatus = "completed"
	// TerminationFailed — a stage/preflight/routing halt; Error carries the cause.
	TerminationFailed TerminationStatus = "failed"
	// TerminationAborted — cooperative cancellation, or the model aborted the stage.
	TerminationAborted TerminationStatus = "aborted"
	// TerminationCancelled — the user dismissed the live session mid-stage.
	// Recorded on disk as the legacy FROZEN StageStatus "skipped"; the
	// canonical name and the frozen row value differ by design.
	TerminationCancelled TerminationStatus = "cancelled"
)

// RunTermination is the run-termination outcome behind RunState.Termination
// and RunWorkflowResult.Termination. Error is set for failed / aborted /
// cancelled, empty otherwise.
type RunTermination struct {
	Status TerminationStatus `json:"status"`
	Error  string            `json:"error,omitempty"`
}

// FailureMemo is one entry in RunState.FailureMemos — a bounded record of a
// stage/unit failure this run has already incurred, surfaced to the next
// agent's prompt as an additive suffix so the run does not repeat a dead end.
// Stage is the machine identity (a loop unit's parent, or the audit stage
// name); UnitID is set only for a loop-unit failure; Ts is ISO-8601.
type FailureMemo struct {
	Stage  string `json:"stage"`
	UnitID string `json:"unitId,omitempty"`
	ErrMsg string `json:"errMsg"`
	Ts     string `json:"ts"`
}

// DroppedRoutingRow records a routing decision whose JSONL append failed.
type DroppedRoutingRow struct {
	FromStageIndex int    `json:"fromStageIndex"`
	FromStage      string `json:"fromStage"`
	Decision       string `json:"decision"`
}

// RunTelemetry is the post-hoc-only block of RunState (never consulted by
// chain advancement).
type RunTelemetry struct {
	// BackwardJumps is the run-wide cumulative count of decision-edge
	// re-entries, waived or counted. Never reset; the halt decision reads the
	// per-destination ledgers on RunContext, not this total.
	BackwardJumps int
	// DroppedRoutingRows are routing rows whose append failed mid-run — the
	// chain advanced past them (routing rows are write-only telemetry).
	DroppedRoutingRows []DroppedRoutingRow
	// DroppedFailureRows are stages whose terminal failure/aborted row failed
	// to append. Unlike routing rows these ARE reconstruction inputs; a run
	// holding entries must not be resumed from disk.
	DroppedFailureRows []string
}

// GatedDispatch is the validation-retry gate memory: the digest captured the
// last time a schema-validated produces stage was dispatched, keyed with the
// stage name + StagesCompleted at capture. Transient — never persisted.
type GatedDispatch struct {
	Stage           string
	Digest          string
	StagesCompleted int
}

// RunState is the mutable per-run bookkeeping threaded through the chain by
// pointer. It satisfies RunView.
type RunState struct {
	// OriginalInputValue is frozen — the user's `/wf` argument.
	OriginalInputValue string

	// PrimaryArtifact is the chain-input artifact — the rolling slot the next
	// stage's prompt inherits. Updated ONLY by produces stages whose
	// collector returned at least one artifact (the first becomes the new
	// primary). Reads go through the chain-state accessor.
	PrimaryArtifact *Artifact
	// OutputValue is the most recent completed stage's envelope.
	OutputValue *Output
	// NamedValue is the publish registry — produces stages APPEND their
	// envelope onto the slot keyed by outcome.name ?? record key. Slots are
	// histories; the default read resolves to the most-recent entry. Never
	// cleared by Terminal — an additive channel orthogonal to PrimaryArtifact.
	NamedValue map[string][]Output
	// StagesCompleted counts stages whose JSONL row landed on disk.
	StagesCompleted int
	// LastAllocatedStageNumber advances on every recordStage call.
	LastAllocatedStageNumber int
	// LastSession is the SessionRef of the most recently completed SINGLE
	// stage — what a downstream "continue" stage forks from. Loop units never
	// touch it. nil at run start or when the predecessor persisted no session.
	LastSession *SessionRef
	// LastGatedDispatch is the validation-retry gate memory (see GatedDispatch).
	LastGatedDispatch *GatedDispatch

	Telemetry RunTelemetry

	// FailureMemos is the bounded log of failures this run incurred, rendered
	// as an additive prompt suffix on every subsequent session. Empty on a
	// clean run ⇒ the suffix is "" ⇒ byte-identical prompt.
	FailureMemos []FailureMemo

	// Termination is how the run ended — "running" until the single
	// end-of-run write via the audit layer's terminate, the ONLY sanctioned mutator.
	Termination RunTermination
}

// OriginalInput implements RunView.
func (s *RunState) OriginalInput() string { return s.OriginalInputValue }

// Output implements RunView.
func (s *RunState) Output() *Output { return s.OutputValue }

// Named implements RunView.
func (s *RunState) Named() map[string][]Output { return s.NamedValue }

// ModelResolveID identifies the unit a model override is resolved for.
type ModelResolveID struct {
	Workflow string
	Stage    string
	Skill    string
}

// ResolveModelFn resolves a per-stage model override; nil ⇒ host default.
type ResolveModelFn func(id ModelResolveID) *ModelSelection

// WorktreeDigestFn computes the worktree digest the validation-retry gate
// compares; (_, false) ⇒ the gate degrades to always-proceed.
type WorktreeDigestFn func(cwd string) (string, bool)

// ReadSessionBranchFn re-opens a persisted child-session file and returns
// its branch; (nil, false) when the file cannot be read.
type ReadSessionBranchFn func(file string) ([]BranchEntry, bool)

// RunWorkflowOptions are the inputs of RunWorkflow.
type RunWorkflowOptions struct {
	// Workflow to execute — the caller resolves by name from the loaded registry.
	Workflow Workflow
	// Input is passed to the start stage as its argument.
	Input string
	// Host is the registry-level host — enumerated once for the
	// skill-registration snapshot; nil opts out of that preflight.
	Host WorkflowHost
	// MaxBackwardJumps is the per-destination decision-edge re-entry cap
	// (waive-aware). nil ⇒ MAX_BACKWARD_JUMPS.
	MaxBackwardJumps *int
	// MaxLaps is the per-destination ABSOLUTE ceiling on re-entries — every
	// re-entry counts (a progress verdict waives the cap, never this). nil ⇒ MAX_LAPS.
	MaxLaps *int
	// MaxIterations is the run-wide safety cap on loop units. nil ⇒ MAX_ITERATIONS.
	MaxIterations *int
	// Trigger is what triggered this run; nil ⇒ DefaultTrigger.
	Trigger *RunTrigger
	// Lifecycle is the per-call listener bundle, fired AFTER every globally
	// registered bundle. Listener errors are logged, never halt the run.
	Lifecycle *LifecycleListeners
	// Signal is cooperative cancellation: once done, the runner stops at the
	// next between-stage seam, records an "aborted" row and returns success
	// false; it does not interrupt a stage already streaming.
	Signal context.Context
	// ResolveModel is the embedder's per-stage model-override resolver.
	ResolveModel ResolveModelFn
	// WorktreeDigest overrides the built-in digest recipe for the validation-retry gate.
	WorktreeDigest WorktreeDigestFn
	// Name is a human-readable alias for the run, stored in the header and the
	// names index; rejected if already in use.
	Name string
}

// RunWorkflowResult is the run envelope RunWorkflow returns.
type RunWorkflowResult struct {
	// RunID is the run's identity on disk; empty ONLY for pre-flight
	// rejections where no JSONL file was created.
	RunID           string `json:"runId,omitempty"`
	StagesCompleted int    `json:"stagesCompleted"`
	Success         bool   `json:"success"`
	// LastArtifact is the primary artifact at termination, serialised to its
	// handle's canonical string form; empty if no produces stage produced one.
	LastArtifact string `json:"lastArtifact,omitempty"`
	Error        string `json:"error,omitempty"`
	// Termination is the full-fidelity outcome; nil ONLY for pre-flight rejections.
	Termination *RunTermination `json:"termination,omitempty"`
	// DroppedRoutingRows are decisions made in memory whose audit row failed to persist.
	DroppedRoutingRows []DroppedRoutingRow `json:"droppedRoutingRows,omitempty"`
	// DroppedFailureRows are stages whose terminal row failed to persist —
	// consumers holding entries should not resume the run from disk.
	DroppedFailureRows []string `json:"droppedFailureRows,omitempty"`
}

// RunContext is the per-run context the chain carries from stage to stage.
type RunContext struct {
	Cwd      string
	RunID    string
	Workflow Workflow
	// TotalStages is the count of stages reachable from Workflow.Start,
	// computed once at run start (surfaced via LifecycleContext).
	TotalStages int
	State       *RunState
	// Visited holds the stage names already executed; a decision edge
	// resolving to one is a backward jump.
	Visited map[string]struct{}
	// Revisits is the decision-edge re-entry count per destination — the
	// backward-jump guard's waive-aware cap ledger.
	Revisits map[string]int
	// ProgressTrail is the observation-only ring (bounded at 3) of a
	// destination's most recent progress verdicts, oldest → newest. Appended
	// ONLY when the re-entered stage declares a Progress hook; never consulted
	// for the halt decision.
	ProgressTrail map[string][]ProgressValue
	// Laps is the ABSOLUTE re-entry count per destination — the lap ledger
	// behind MaxLaps. Revisits ≤ Laps always.
	Laps map[string]int
	// RegisteredSkills is the set of bare skill names registered with the host
	// at workflow start; nil when no host was passed (the preflight is skipped).
	RegisteredSkills map[string]struct{}
	// SkillContracts is the registered (declared-source) contract registry
	// snapshot; nil degrades to no contract validation.
	SkillContracts    SkillContractMap
	ResolveModel      ResolveModelFn
	ReadSessionBranch ReadSessionBranchFn
	WorktreeDigest    WorktreeDigestFn
	MaxBackwardJumps  int
	MaxLaps           int
	// MaxIterations clamps the effective cap of EVERY loop kind
	// (min(loop.max, MaxIterations)) — the backstop for a never-terminating source.
	MaxIterations int
	Trigger       RunTrigger
	Lifecycle     *LifecycleDispatcher
	// Signal is the optional cooperative-cancellation context (nil = none),
	// checked at the between-stage seam.
	Signal context.Context
}

// RunIdentity is the read-only run identity passed to lifecycle callbacks,
// captured at session construction.
type RunIdentity struct {
	Workflow    string
	TotalStages int
	Trigger     RunTrigger
}

// SessionContext is the per-stage / per-unit common base. StageName is the
// workflow stage's record key (the value in WorkflowStage.stage); Skill is
// the skill body the runner dispatches — equal in the common case, divergent
// for aliased stages.
type SessionContext struct {
	Cwd   string
	RunID string
	State *RunState
	// Prompt is the `/skill:<name> <args>` message (or the raw prompt text).
	Prompt    string
	StageName string
	Skill     string
	Lifecycle *LifecycleDispatcher
	// RunIdentity is built once per run, reused.
	RunIdentity RunIdentity
	// AllocatedStageNumber is the activation's JSONL stage number, assigned
	// ONCE when output production begins; nil until then (pre-output halts
	// allocate at record time instead).
	AllocatedStageNumber *int
	// ReadSessionBranch is the host-injected persisted-session branch reader
	// the death-scene artifact writer consumes; nil degrades silently.
	ReadSessionBranch ReadSessionBranchFn
}

// UnitRef is the unit identity threaded onto a loop unit's session — the
// source of the structured JSONL row fields and the UnitEvent payload.
// Parent is the loop stage's record key; Index the 0-based generation cursor
// (== the round index for assess loops); ID the stable audit identity
// (unit.id ?? unit.label for fanout/iterate; empty for assess); Label the display tag.
type UnitRef struct {
	Parent string
	Role   UnitRole
	Index  int
	ID     string
	Label  string
}

// OnSuccessFn receives the stage's VALIDATED Output envelope. Its result is
// opaque to the session layer (the chain walk's continuations plug in
// directly); the session layer only awaits settlement.
type OnSuccessFn func(ctx WorkflowHostContext, output Output) (any, error)

// StageSessionContext is the per-stage session context.
type StageSessionContext struct {
	SessionContext
	Stage StageDef
	// SkillContracts lets output validation fall back to the dispatched
	// skill's produces.data when the stage carries no OutputSchema.
	SkillContracts SkillContractMap
	// StageIndex is the 0-based stage index within this run.
	StageIndex int
	// Snapshot is the pre-stage snapshot value (nil if the outcome has no snapshot).
	Snapshot any
	// Model is the resolved per-unit override, applied by the host at child creation.
	Model          *ModelSelection
	WorktreeDigest WorktreeDigestFn
	// Signal is the per-child cooperative-abort context.
	Signal context.Context
	// CollectAll — a collect-all fanout unit: a SEMANTIC unit failure
	// soft-halts THIS unit instead of terminating the run; an infra-death
	// stop hard-fails even here so resume re-dispatches the dead unit.
	CollectAll bool
	// LaneUnitIndex is the per-unit lane key, set ONLY for fan-out units.
	LaneUnitIndex *int
	// BranchOffset is set only for continue stages — the branch slice offset.
	BranchOffset *int
	// BashTimeoutStrikes is the per-activation strike-ceiling override; nil ⇒ the module default.
	BashTimeoutStrikes *int
	// AttemptOrdinal is the 1-based dispatch ordinal of THIS attempt within a
	// fanout unit's retryHaltedUnits window; nil for sequential units and single stages.
	AttemptOrdinal *int
	// Unit is present iff this session IS one loop unit.
	Unit      *UnitRef
	OnFailure func(ctx WorkflowHostContext)
	OnSuccess OnSuccessFn
}
