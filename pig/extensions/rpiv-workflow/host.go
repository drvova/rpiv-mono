package workflow

import "context"

// Host ports — the contract the workflow runtime needs from its host
// environment, expressed in workflow-domain vocabulary. Two host shapes
// implement them: the interactive launcher satisfies the OBSERVER surface
// (WorkflowLauncherContext) and is retained as a progress observer only; the
// detached executor host satisfies the FULL surface (WorkflowHostContext)
// including SpawnChild / MaxConcurrency and owns the child-session pool every
// stage runs in. Cancellation is a context.Context throughout (the port of
// the AbortSignal the TypeScript surface threads).

// NotifyLevel is the severity of a host notification.
type NotifyLevel string

const (
	NotifyInfo    NotifyLevel = "info"
	NotifyWarning NotifyLevel = "warning"
	NotifyError   NotifyLevel = "error"
)

// CommandInfo is one registered slash command as the host enumerates it.
type CommandInfo struct {
	Name   string
	Source string
}

// RegisterCommandOptions are the options WorkflowHost.RegisterCommand takes.
// The handler receives the LAUNCHER ctx (observer surface): it constructs the
// executor host that actually runs stages.
type RegisterCommandOptions struct {
	Description string
	Handler     func(args string, ctx WorkflowLauncherContext) error
}

// WorkflowHost is the registry-level host: the two members the runtime
// touches — command registration for the `/wf` entry point and the command
// enumeration the skill-registration preflight consults.
type WorkflowHost interface {
	RegisterCommand(name string, options RegisterCommandOptions)
	GetCommands() []CommandInfo
}

// HostUI is the notify-only UI contract.
type HostUI interface {
	Notify(message string, level NotifyLevel)
}

// HostSessionManager is the session-transcript read surface. GetBranch
// returns the host's transcript value DELIBERATELY untyped — ReadBranch
// (transcript.go) is the single boundary that narrows it to []BranchEntry.
// GetSessionFile reports false for non-persisting (in-memory) sessions.
type HostSessionManager interface {
	GetBranch() any
	GetSessionID() string
	GetSessionFile() (string, bool)
}

// WorkflowLauncherContext is the observer-only ctx the registry-level host
// hands the `/wf` handler. The launcher relays progress through it and
// constructs the executor host that runs stages; it excludes the executor
// additions (SpawnChild / MaxConcurrency). Signal is the cooperative
// cancellation handle the launcher forwards into the run (a Background
// context when the host provides none).
type WorkflowLauncherContext interface {
	Cwd() string
	HasUI() bool
	UI() HostUI
	SessionManager() HostSessionManager
	WaitForIdle() error
	Signal() context.Context
}

// ThinkingLevel mirrors the host's thinking levels.
type ThinkingLevel string

const (
	ThinkingOff     ThinkingLevel = "off"
	ThinkingMinimal ThinkingLevel = "minimal"
	ThinkingLow     ThinkingLevel = "low"
	ThinkingMedium  ThinkingLevel = "medium"
	ThinkingHigh    ThinkingLevel = "high"
	ThinkingXHigh   ThinkingLevel = "xhigh"
	ThinkingMax     ThinkingLevel = "max"
)

// ModelSelection is the resolved per-unit model override, applied by the
// host at child-session creation — NOT via global mutation. Model is a
// host-opaque model key the host's adapter resolves through its own
// registry. Both fields optional — empty means "host default".
type ModelSelection struct {
	Model    string        `json:"model,omitempty"`
	Thinking ThinkingLevel `json:"thinking,omitempty"`
}

// SessionFileRef names a persisted session file an open mode operates on.
type SessionFileRef struct {
	SessionFile string
}

// SpawnOptions are the inputs of WorkflowHostContext.SpawnChild.
//
// Model is the resolved per-unit override. Reattach makes the host OPEN the
// persisted session IN PLACE (appending to it) and deliver it to withSession
// WITHOUT sending Prompt — the prior turn already ran, so the body promotes
// from the existing branch or re-prompts via SendUserMessage. Fork makes the
// host FORK the persisted session into a NEW child (new id, new file)
// carrying the full prior transcript, WITHOUT mutating the source file and
// WITHOUT sending Prompt (the body sends the continuation turn, then measures
// the inherited prefix) — how sessionPolicy "continue" continues its
// predecessor under detachment. At most one of Reattach / Fork is set; both
// suppress the host's initial prompt send. UnitIndex is the fan-out unit
// index, surfaced so a lane-aware host can publish the child under a per-unit
// slot — an observability hint that never affects execution.
type SpawnOptions struct {
	Prompt    string
	Model     *ModelSelection
	Reattach  *SessionFileRef
	Fork      *SessionFileRef
	UnitIndex *int
}

// WorkflowHostContext is the per-command executor ctx. Embedders hand it to
// RunWorkflow; the runner threads it (and the child ctx delivered to each
// SpawnChild's withSession) through stages. Exhaustive list of members the
// runtime touches — adding any reach outside it is a port-widening decision.
type WorkflowHostContext interface {
	WorkflowLauncherContext

	// MaxConcurrency is the max child sessions runnable at once. 1 ⇒
	// sequential. The loop reads this; it never invents a number.
	MaxConcurrency() int

	// SpawnChild spawns an isolated child session, sends options.Prompt, runs
	// withSession on the child's ctx and returns its error. The PARENT ctx
	// STAYS VALID (no swap). The runner may have up to MaxConcurrency calls
	// in flight at once. ctx cancels THIS child mid-flight the moment it is
	// done — the dispatcher threads the run's signal here so an aborted run
	// interrupts in-flight children, not just the between-stage seam.
	// Results the body produces are captured by the closure.
	SpawnChild(ctx context.Context, options SpawnOptions, withSession func(child WorkflowSessionContext) error) error
}

// WorkflowSessionContext is the child ctx delivered to SpawnChild's
// withSession callback. The parent declares NO sender; this subtype ADDS
// SendUserMessage as the guaranteed in-session sender, plus the watchdog
// channel: ToolTimeout reports (reason, true) when a watchdog-equipped host
// aborted THIS child because a single tool call overran its wall-clock
// ceiling — the out-of-band signal that tells a watchdog timeout apart from a
// run/user abort, so it routes through the soft-halt gate instead of a
// re-dispatching abort. ResetToolTimeout clears the recorded verdict so the
// watchdog re-arms for the resumed turn. A host without a watchdog returns
// ("", false) and treats ResetToolTimeout as a no-op.
type WorkflowSessionContext interface {
	WorkflowHostContext
	SendUserMessage(content string) error
	ToolTimeout() (reason string, fired bool)
	ResetToolTimeout()
}
