package workflow

import "context"

// Stage vocabulary + factories — what a stage IS (StageDef and its three
// dispatch arms), what a workflow is, and the blessed constructors
// (DefineWorkflow, Produces / Acts / Terminal and their Script / Prompt
// builders). The loop shapes live in loop-def.go, the routing DSL in
// routing-dsl.go. Factories are pure passthroughs that apply sane defaults.

// StageKind discriminates artifact-producing stages from side-effect stages:
//   - "produces"    — protocol skills that write an artifact; the runner halts
//     the chain if the collector finds none.
//   - "side-effect" — action skills (commit, implement) where the side effect
//     IS the work; the chain inherits the prior primary artifact.
//
// The kind literal is descriptive while its factory is a verb (Acts()).
// Terminal() is not a third kind: it builds "side-effect" + InheritsArtifacts: false.
type StageKind string

const (
	KindProduces   StageKind = "produces"
	KindSideEffect StageKind = "side-effect"
)

// StageKinds is the single source of truth the validator's enum check consumes.
var StageKinds = []StageKind{KindProduces, KindSideEffect}

// ProgressValue is a backward-jump guard verdict — what a stage's optional
// Progress hook returns per decision-edge re-entry. "improved" WAIVES the
// re-entry; the other three COUNT exactly as a hook-less re-entry does.
type ProgressValue string

const (
	ProgressImproved  ProgressValue = "improved"
	ProgressUnchanged ProgressValue = "unchanged"
	ProgressRegressed ProgressValue = "regressed"
	ProgressUnknown   ProgressValue = "unknown"
)

// ProgressValues enumerates the four verdicts in waives/counts order; the
// guard normalises any off-list return to "unknown" via membership here.
var ProgressValues = []ProgressValue{ProgressImproved, ProgressUnchanged, ProgressRegressed, ProgressUnknown}

// SessionPolicy says how a stage's child session is opened:
//   - "fresh"    — a brand-new detached child session.
//   - "continue" — the stage FORKS its predecessor's persisted child session,
//     so the child carries the prior conversation as context; no predecessor
//     session degrades to a fresh dispatch.
type SessionPolicy string

const (
	SessionFresh    SessionPolicy = "fresh"
	SessionContinue SessionPolicy = "continue"
)

// SessionPolicies is the runtime enumeration the validator consumes.
var SessionPolicies = []SessionPolicy{SessionFresh, SessionContinue}

// OnInvalid is what happens when a stage's OutputSchema rejects the output:
// "retry" re-invokes the stage up to MaxRetries with the issues threaded back
// to the agent; "halt" records a terminal failure on the first rejection.
type OnInvalid string

const (
	OnInvalidRetry OnInvalid = "retry"
	OnInvalidHalt  OnInvalid = "halt"
)

// OnInvalidValues is the runtime enumeration the validator consumes.
var OnInvalidValues = []OnInvalid{OnInvalidRetry, OnInvalidHalt}

// ScriptContext is handed to a script stage's Run function and to a PromptFn:
// frozen identity (Cwd) + the chain data the function needs (Input — the
// upstream Output envelope, nil when the stage is the entry point or the
// upstream cleared the rolling primary) + a read-only state view.
type ScriptContext struct {
	Cwd   string
	Input *Output
	State RunView
}

// ScriptOutput is what a produces script returns: the Output envelope's
// value-channel fields. The runner stamps Meta.
type ScriptOutput struct {
	Kind      string
	Artifacts []Artifact
	Data      any
}

// ScriptFn is a script stage body — one of ProducesScriptFn / ActsScriptFn.
type ScriptFn interface {
	isScriptFn()
}

// ProducesScriptFn is a script produces body: it returns the envelope's
// value-channel fields directly; the runner stamps meta.
type ProducesScriptFn func(ctx context.Context, sc ScriptContext) (ScriptOutput, error)

// ActsScriptFn is a script acts / terminal body: it returns nothing; the
// runner builds a side-effect envelope so audit + chain wiring stay uniform.
type ActsScriptFn func(ctx context.Context, sc ScriptContext) error

func (ProducesScriptFn) isScriptFn() {}
func (ActsScriptFn) isScriptFn()     {}

// PromptFn is the dynamic form of a raw-prompt dispatch body: it returns the
// COMPLETE user message sent into the stage's session (no `/skill:` prefix,
// no implicit upstream-artifact arg).
type PromptFn func(ctx context.Context, sc ScriptContext) (string, error)

// Prompt is a raw-prompt dispatch body — a static string or a PromptFn (the
// port of `string | PromptFn`). The zero value means "no prompt dispatch".
type Prompt struct {
	text string
	fn   PromptFn
	set  bool
}

// StaticPrompt builds a static prompt.
func StaticPrompt(text string) Prompt { return Prompt{text: text, set: true} }

// DynamicPrompt builds a prompt from a function.
func DynamicPrompt(fn PromptFn) Prompt { return Prompt{fn: fn, set: true} }

// IsSet reports whether the stage carries a prompt dispatch.
func (p Prompt) IsSet() bool { return p.set }

// IsDynamic reports whether the prompt is function-valued.
func (p Prompt) IsDynamic() bool { return p.fn != nil }

// Text is the static text (empty for a dynamic prompt).
func (p Prompt) Text() string { return p.text }

// Resolve renders the prompt: the static text, or the function's result.
func (p Prompt) Resolve(ctx context.Context, sc ScriptContext) (string, error) {
	if p.fn != nil {
		return p.fn(ctx, sc)
	}
	return p.text, nil
}

// StageRead is a single `reads:` entry: a channel name read latest-wins (the
// historical default) or, with All set, EVERY accumulated entry of the channel.
// Fanin builds the all-entries form — the consumer-side mirror of Fanout.
type StageRead struct {
	Name string
	All  bool
}

// Read builds the latest-wins read of a channel (the bare-string form).
func Read(name string) StageRead { return StageRead{Name: name} }

// Fanin reads EVERY accumulated entry of a channel (run order), not just the
// latest — the canonical consumer side of fanout-and-synthesize. Returns an
// error on an empty name, mirroring the value-returning loop/routing builders.
func Fanin(name string) (StageRead, error) {
	if err := RequireNonEmptyString(name, "fanin(name)", "name must be a non-empty channel name", false); err != nil {
		return StageRead{}, err
	}
	return StageRead{Name: name, All: true}, nil
}

// ReadName is the channel name of a read.
func ReadName(read StageRead) string { return read.Name }

// ReadsAll reports whether a read consumes ALL accumulated entries.
func ReadsAll(read StageRead) bool { return read.All }

// ProgressFn is the optional backward-jump progress hook, consulted ONLY
// when a decision edge re-enters the stage: "improved" WAIVES the re-entry;
// anything else counts. A failing hook degrades to "unknown"; an off-list
// return normalises to "unknown".
type ProgressFn func(ctx context.Context, state RunView) (ProgressValue, error)

// StageDef is a stage in the workflow graph — the three dispatch arms of the
// TypeScript union in one record, discriminated by presence: Run set ⇒ a
// script stage; Prompt set ⇒ a raw-prompt stage; neither ⇒ a skill stage (the
// default), dispatching `/skill:<Skill>` where Skill defaults to the stage's
// record key. The stage's identity is the surrounding Workflow.Stages key.
// Illegal combinations (Run + Skill, Prompt + Reads, …) are rejected by the
// load-time validator, which re-checks every rule for hand-rolled values.
//
// Optional numeric / boolean knobs are pointers so "unset" stays distinct
// from a zero value; Skill, OnInvalid and Prompt are unset when zero.
type StageDef struct {
	Kind          StageKind
	SessionPolicy SessionPolicy
	// OutputSchema validates output.data after the stage's Outcome produces
	// it; on rejection the runner honours OnInvalid.
	OutputSchema StageSchema
	// InputSchema validates the inherited upstream output.data before the
	// stage runs; a rejection halts the chain immediately.
	InputSchema       StageSchema
	OnInvalid         OnInvalid
	MaxRetries        *int
	ValidateTimeoutMs *int
	// InheritsArtifacts says whether the stage inherits the chain's primary
	// artifact. nil/true inherits; false (authored via Terminal) hands the
	// stage the original input, bypasses the upstream-artifact preflight and
	// clears the rolling primary on success.
	InheritsArtifacts *bool
	Progress          ProgressFn

	// Skill is the skill body to dispatch when the stage id and the skill
	// differ (aliased stages). Empty ⇒ the record key.
	Skill   string
	Outcome *Outcome
	// Loop is the opt-in unit loop (Fanout / Iterate / Assess constructors).
	// Mutually exclusive with SessionPolicy "continue" and Verify.
	Loop LoopDef
	// Verify is the opt-in post-condition judge, desugared at runtime into a
	// degenerate assess loop. Requires Kind "produces".
	Verify *VerifySpec
	// Reads names the channels the stage consumes from state.named to build
	// its prompt (`/skill:<name> --<n1> <h1> …`).
	Reads []StageRead

	// Run is the script dispatch body (ProducesScriptFn or ActsScriptFn).
	Run ScriptFn
	// Prompt is the raw-prompt dispatch body.
	Prompt Prompt
}

// Workflow is a complete workflow: Name is what users type as `/wf <name>`;
// Start is the entry stage; Stages is the lexicon; Edges is the wiring. Every
// key in Edges must exist in Stages; every string target must exist in
// Stages or be Stop. Validated at load time.
type Workflow struct {
	Name        string
	Description string
	Start       string
	Stages      map[string]StageDef
	Edges       map[string]EdgeTarget
}

// DefineWorkflow is an identity passthrough; reserved for future
// normalisation / metadata hooks.
func DefineWorkflow(spec Workflow) Workflow { return spec }

// withDefaults is THE stage-defaults authority: `{kind, sessionPolicy: fresh,
// ...overrides}` — every present override field wins (a present Kind too, as
// the TypeScript spread lets it), and an unset SessionPolicy reads "fresh".
func withDefaults(kind StageKind, overrides StageDef) StageDef {
	def := overrides
	if def.Kind == "" {
		def.Kind = kind
	}
	if def.SessionPolicy == "" {
		def.SessionPolicy = SessionFresh
	}
	return def
}

func firstOverride(overrides []StageDef) StageDef {
	if len(overrides) == 0 {
		return StageDef{}
	}
	return overrides[0]
}

// ProducesScriptOptions are the options ProducesScript takes — the
// validation + inheritance knobs that apply to a pure function. Kind and
// SessionPolicy are not configurable: script stages are "produces" + "fresh".
type ProducesScriptOptions struct {
	Run               ProducesScriptFn
	OutputSchema      StageSchema
	InputSchema       StageSchema
	OnInvalid         OnInvalid
	MaxRetries        *int
	ValidateTimeoutMs *int
	InheritsArtifacts *bool
	Reads             []StageRead
	Progress          ProgressFn
}

// ActsScriptOptions are the options ActsScript / TerminalScript take.
// Side-effect stages emit no data envelope, so the retry knobs don't apply.
type ActsScriptOptions struct {
	Run               ActsScriptFn
	InputSchema       StageSchema
	InheritsArtifacts *bool
	Reads             []StageRead
	Progress          ProgressFn
}

// ProducesPromptOptions are the options ProducesPrompt takes — the typed
// builder for a raw-prompt produces stage. Outcome is required; Loop is
// assess-only; SessionPolicy "continue" makes this a follow-up turn.
type ProducesPromptOptions struct {
	Prompt            Prompt
	Outcome           *Outcome
	OutputSchema      StageSchema
	InputSchema       StageSchema
	OnInvalid         OnInvalid
	MaxRetries        *int
	ValidateTimeoutMs *int
	Loop              *AssessLoop
	Verify            *VerifySpec
	Progress          ProgressFn
	SessionPolicy     SessionPolicy
}

// ActsPromptOptions are the options ActsPrompt takes — a pure chat turn (no
// outcome). For a collecting side-effect prompt stage use the bare Acts form.
type ActsPromptOptions struct {
	Prompt        Prompt
	InputSchema   StageSchema
	Progress      ProgressFn
	SessionPolicy SessionPolicy
}

// Produces builds an artifact-producing stage: invokes a skill that writes an
// artifact. Defaults to fresh-session; the skill body defaults to the record
// key. Pass at most one overrides record.
func Produces(overrides ...StageDef) StageDef {
	return withDefaults(KindProduces, firstOverride(overrides))
}

// ProducesScript is the skillless variant of Produces: a pure function runs
// in place of a skill body and returns the envelope directly.
func ProducesScript(opts ProducesScriptOptions) StageDef {
	return withDefaults(KindProduces, StageDef{
		Run:               opts.Run,
		OutputSchema:      opts.OutputSchema,
		InputSchema:       opts.InputSchema,
		OnInvalid:         opts.OnInvalid,
		MaxRetries:        opts.MaxRetries,
		ValidateTimeoutMs: opts.ValidateTimeoutMs,
		InheritsArtifacts: opts.InheritsArtifacts,
		Reads:             opts.Reads,
		Progress:          opts.Progress,
	})
}

// ProducesPrompt is the raw-prompt variant of Produces: author-owned text is
// dispatched (no `/skill:` prefix) and the reply collected via Outcome.
func ProducesPrompt(opts ProducesPromptOptions) StageDef {
	var loop LoopDef
	if opts.Loop != nil {
		loop = opts.Loop
	}
	return withDefaults(KindProduces, StageDef{
		Prompt:            opts.Prompt,
		Outcome:           opts.Outcome,
		OutputSchema:      opts.OutputSchema,
		InputSchema:       opts.InputSchema,
		OnInvalid:         opts.OnInvalid,
		MaxRetries:        opts.MaxRetries,
		ValidateTimeoutMs: opts.ValidateTimeoutMs,
		Loop:              loop,
		Verify:            opts.Verify,
		Progress:          opts.Progress,
		SessionPolicy:     opts.SessionPolicy,
	})
}

// Acts builds a side-effect stage: a skill whose side effect IS the work
// (commit, implement). No artifact-emission check; defaults to fresh-session.
func Acts(overrides ...StageDef) StageDef {
	return withDefaults(KindSideEffect, firstOverride(overrides))
}

// ActsScript is the skillless variant of Acts; the runner synthesises a
// side-effect envelope so the chain stays uniform.
func ActsScript(opts ActsScriptOptions) StageDef {
	return withDefaults(KindSideEffect, StageDef{
		Run:               opts.Run,
		InputSchema:       opts.InputSchema,
		InheritsArtifacts: opts.InheritsArtifacts,
		Reads:             opts.Reads,
		Progress:          opts.Progress,
	})
}

// ActsPrompt is the raw-prompt variant of Acts: a pure chat turn.
func ActsPrompt(opts ActsPromptOptions) StageDef {
	return withDefaults(KindSideEffect, StageDef{
		Prompt:        opts.Prompt,
		InputSchema:   opts.InputSchema,
		Progress:      opts.Progress,
		SessionPolicy: opts.SessionPolicy,
	})
}

// Terminal builds a terminal side-effect stage: an Acts-shaped stage that
// does NOT inherit the upstream primary artifact — its prompt receives the
// run's original input, the upstream-artifact preflight is bypassed and the
// rolling primary is cleared on success. Desugars to Acts with
// InheritsArtifacts false applied AFTER the overrides, so a caller cannot
// reverse the opt-out. ("terminal" also names a graph sink — a stage with no
// outgoing edge or an explicit Stop — and the run-outcome prose "terminal
// failure"; the three senses are unrelated.)
func Terminal(overrides ...StageDef) StageDef {
	def := firstOverride(overrides)
	def.InheritsArtifacts = Bool(false)
	return withDefaults(KindSideEffect, def)
}

// TerminalScript is the skillless variant of Terminal.
func TerminalScript(opts ActsScriptOptions) StageDef {
	opts.InheritsArtifacts = Bool(false)
	return ActsScript(opts)
}
