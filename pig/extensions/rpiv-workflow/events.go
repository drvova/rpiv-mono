package workflow

// Lifecycle callbacks — the typed observation surface for in-flight runs.
// Two subscription paths converge through the same LifecycleListeners shape:
// per-call (RunWorkflowOptions.Lifecycle) and global (RegisterLifecycle,
// which returns a disposer; multiple registrations fan out per event).
// Every event fires AFTER the corresponding JSONL row lands on disk, so a
// listener reading past rows sees consistent state. Listener errors are
// caught + surfaced through the host's notify at "warning" and never halt
// the run. Pre-flight rejections fire ZERO lifecycle events.

// LifecycleContext is the run-scoped context shared by every callback.
type LifecycleContext struct {
	Cwd         string
	RunID       string
	Workflow    string
	TotalStages int
	Trigger     RunTrigger
	State       RunView
}

// StageRefKind discriminates StageRef.
type StageRefKind string

const (
	StageRefSkill  StageRefKind = "skill"
	StageRefScript StageRefKind = "script"
)

// StageRef is the projection of a stage as observed by a listener — the
// runner's resolved view minus the DSL internals. Kind "skill" dispatches a
// skill body (Skill set); "script" runs a function (no skill body).
type StageRef struct {
	Kind        StageRefKind
	Name        string
	StageNumber int
	Skill       string
}

// LoopStartKind is the loop kind an onLoopStart / onLoopCap event reports —
// a LoopKind, or "verify" for a verify-bearing stage (a desugared
// attempt→verify loop; verified stages follow loop semantics).
type LoopStartKind string

// LoopKindVerify is the LoopStartKind of a verify-bearing stage.
const LoopKindVerify LoopStartKind = "verify"

// LoopStartInfo is the onLoopStart payload. Units is present only when the
// loop precomputes its unit list (fanout).
type LoopStartInfo struct {
	Kind  LoopStartKind
	Units []Unit
}

// UnitEvent is the per-unit event payload. Skill is the unit's DISPATCHED
// skill body — the parent stage's skill for produce units, the judge's own
// skill (or the synthetic `<parent>-judge` label for prompt judges) for judge units.
type UnitEvent struct {
	Role UnitRole
	// Index is the 0-based generation cursor (== the round index for assess loops).
	Index int
	// UnitID is the stable audit identity; empty for assess units.
	UnitID string
	Label  string
	Skill  string
}

// LoopCapInfo is the onLoopCap payload — fired when a loop's effective cap trips.
type LoopCapInfo struct {
	Kind LoopStartKind
	// Count is units run (fanout/iterate) or rounds run (assess) when the cap tripped.
	Count int
	// Max is the effective cap: min(loop.max, run.maxIterations).
	Max    int
	Policy CapPolicy
}

// LifecycleListeners is an opt-in listener bundle. Single subscriber per slot
// per bundle — fan-out is a userland wrapper. The runner awaits every
// callback before advancing (back-pressure for free).
type LifecycleListeners struct {
	// OnWorkflowStart fires after the JSONL header lands; before the start stage's preflight.
	OnWorkflowStart func(ctx LifecycleContext) error
	// OnStageStart fires after preflight + skill check; before the session opens.
	OnStageStart func(stage StageRef, ctx LifecycleContext) error
	// OnStageEnd fires after the stage's success row lands; output is the validated envelope.
	OnStageEnd func(stage StageRef, output Output, ctx LifecycleContext) error
	// OnStageRetry fires after an outputSchema rejection, before the re-prompt. attempt is 1-based.
	OnStageRetry func(stage StageRef, attempt int, ctx LifecycleContext) error
	// OnStageError fires after the "failed"/"aborted" row lands. Terminal for the run.
	OnStageError func(stage StageRef, errMsg string, ctx LifecycleContext) error
	// OnRoute fires after an EdgeFn picks and its routing row lands; to may be Stop.
	OnRoute func(from StageRef, to string, ctx LifecycleContext) error
	// OnLoopStart fires after OnStageStart, before unit 1's session.
	OnLoopStart func(stage StageRef, info LoopStartInfo, ctx LifecycleContext) error
	// OnUnitStart fires per unit BEFORE the unit's session opens, for produce AND judge units.
	OnUnitStart func(stage StageRef, unit UnitEvent, ctx LifecycleContext) error
	// OnUnitEnd fires per unit after its row lands. Loop units never fire OnStageEnd.
	OnUnitEnd func(stage StageRef, unit UnitEvent, output Output, ctx LifecycleContext) error
	// OnUnitHalt fires after a collect-all fanout unit's NON-TERMINAL
	// collected:true row lands — the unit halted but the run survives.
	OnUnitHalt func(stage StageRef, unit UnitEvent, reason string, ctx LifecycleContext) error
	// OnLoopCap fires after an onCap "advance" trip (after the loop-cap row append attempt).
	OnLoopCap func(stage StageRef, info LoopCapInfo, ctx LifecycleContext) error
	// OnWorkflowEnd is the last call — result is the same envelope RunWorkflow returns.
	OnWorkflowEnd func(result RunWorkflowResult, ctx LifecycleContext) error
}

// LifecycleEvent names one listener slot — the event label MsgLifecycleThrew renders.
type LifecycleEvent string

// The listener slots, named as the TypeScript bundle keys.
const (
	EventWorkflowStart LifecycleEvent = "onWorkflowStart"
	EventStageStart    LifecycleEvent = "onStageStart"
	EventStageEnd      LifecycleEvent = "onStageEnd"
	EventStageRetry    LifecycleEvent = "onStageRetry"
	EventStageError    LifecycleEvent = "onStageError"
	EventRoute         LifecycleEvent = "onRoute"
	EventLoopStart     LifecycleEvent = "onLoopStart"
	EventUnitStart     LifecycleEvent = "onUnitStart"
	EventUnitEnd       LifecycleEvent = "onUnitEnd"
	EventUnitHalt      LifecycleEvent = "onUnitHalt"
	EventLoopCap       LifecycleEvent = "onLoopCap"
	EventWorkflowEnd   LifecycleEvent = "onWorkflowEnd"
)

// DispatchHost is the subset of the host the dispatcher needs for
// throw-safe logging.
type DispatchHost interface {
	UI() HostUI
}

// LifecycleDispatcher is the fan-out + error-safe invoker for one event
// across every registered listener bundle. Constructed once per run and
// threaded through RunContext so every firing site uses the same instance.
// Bundles run in registration order; the per-call bundle (when present)
// fires after every globally-registered bundle. Snapshot semantics: the
// registry is read per fire, so a registration made mid-event applies to
// subsequent events but not the in-flight one.
type LifecycleDispatcher struct {
	perCall *LifecycleListeners
}

// NewLifecycleDispatcher builds a dispatcher over the global registry plus
// an optional per-call bundle.
func NewLifecycleDispatcher(perCall *LifecycleListeners) *LifecycleDispatcher {
	return &LifecycleDispatcher{perCall: perCall}
}

// Fire invokes event on every bundle via invoke(bundle) — invoke selects the
// slot and calls it with the event's arguments, returning nil when the bundle
// has no listener for the slot. An error is surfaced through host notify at
// "warning"; the run never halts.
func (d *LifecycleDispatcher) Fire(host DispatchHost, event LifecycleEvent, invoke func(bundle *LifecycleListeners) error) {
	for _, bundle := range collectBundles(d.perCall) {
		if err := safeInvoke(invoke, bundle); err != nil {
			host.UI().Notify(MsgLifecycleThrew(string(event), FormatError(err)), NotifyWarning)
		}
	}
}

// safeInvoke runs a listener, turning a panic into the error the dispatcher reports.
func safeInvoke(invoke func(*LifecycleListeners) error, bundle *LifecycleListeners) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &listenerPanic{value: r}
		}
	}()
	return invoke(bundle)
}

type listenerPanic struct{ value any }

func (p *listenerPanic) Error() string { return FormatError(p.value) }

// lifecycleRegistryKey anchors the global registry on a process-wide slot so
// a duplicate load shares one registry.
var lifecycleRegistryKey = SymbolFor("@juicesharp/rpiv-workflow:lifecycle")

type lifecycleRegistry struct {
	bundles []*LifecycleListeners
}

// RegisterLifecycle registers a cross-package listener bundle and returns a
// disposer that removes it. Multiple registrations coexist — every fired
// event walks the full registry in registration order, then the per-call bundle.
func RegisterLifecycle(listeners *LifecycleListeners) func() {
	globalSlots.mu.Lock()
	reg := registryLocked()
	reg.bundles = append(reg.bundles, listeners)
	globalSlots.mu.Unlock()
	return func() {
		globalSlots.mu.Lock()
		defer globalSlots.mu.Unlock()
		reg := registryLocked()
		for i, b := range reg.bundles {
			if b == listeners {
				reg.bundles = append(reg.bundles[:i], reg.bundles[i+1:]...)
				return
			}
		}
	}
}

// registryLocked resolves the registry while the slot-table lock is held.
func registryLocked() *lifecycleRegistry {
	if value, ok := globalSlots.m[lifecycleRegistryKey]; ok {
		return value.(*lifecycleRegistry)
	}
	reg := &lifecycleRegistry{}
	globalSlots.m[lifecycleRegistryKey] = reg
	return reg
}

// ResetLifecycleRegistry is the test reset — clears every registered bundle.
func ResetLifecycleRegistry() {
	globalSlots.mu.Lock()
	defer globalSlots.mu.Unlock()
	registryLocked().bundles = nil
}

// collectBundles snapshots the globally-registered bundles (registration
// order) followed by the per-call bundle.
func collectBundles(perCall *LifecycleListeners) []*LifecycleListeners {
	globalSlots.mu.Lock()
	global := append([]*LifecycleListeners(nil), registryLocked().bundles...)
	globalSlots.mu.Unlock()
	if perCall != nil {
		return append(global, perCall)
	}
	return global
}

// LifecycleContextArgs are the inputs of BuildLifecycleContext.
type LifecycleContextArgs = LifecycleContext

// BuildLifecycleContext builds a LifecycleContext from the runner's per-run identity.
func BuildLifecycleContext(args LifecycleContextArgs) LifecycleContext { return args }

// LifecycleCtxFor is THE one RunContext → LifecycleContext projection shared
// by the runner, the loop driver and the resume entries; captured per fire
// so listeners always see the latest state.
func LifecycleCtxFor(run *RunContext) LifecycleContext {
	return BuildLifecycleContext(LifecycleContext{
		Cwd:         run.Cwd,
		RunID:       run.RunID,
		Workflow:    run.Workflow.Name,
		TotalStages: run.TotalStages,
		Trigger:     run.Trigger,
		State:       run.State,
	})
}

// LifecycleCtxFromSession builds a LifecycleContext from any
// SessionContext-shaped value (cwd + runId + the frozen run identity + live state).
func LifecycleCtxFromSession(s *SessionContext) LifecycleContext {
	return BuildLifecycleContext(LifecycleContext{
		Cwd:         s.Cwd,
		RunID:       s.RunID,
		Workflow:    s.RunIdentity.Workflow,
		TotalStages: s.RunIdentity.TotalStages,
		Trigger:     s.RunIdentity.Trigger,
		State:       s.State,
	})
}

// SkillStageRef builds the "skill" arm of StageRef.
func SkillStageRef(name string, stageNumber int, skill string) StageRef {
	return StageRef{Kind: StageRefSkill, Name: name, StageNumber: stageNumber, Skill: skill}
}

// ScriptStageRef builds the "script" arm of StageRef — skillless stages.
func ScriptStageRef(name string, stageNumber int) StageRef {
	return StageRef{Kind: StageRefScript, Name: name, StageNumber: stageNumber}
}
