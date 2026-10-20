package workflow

// RunTrigger identifies what triggered a workflow run. Recorded in the JSONL
// header (WorkflowHeader.Trigger), threaded into every lifecycle event
// (LifecycleContext.Trigger) and surfaced on past-run enumeration.
//
// `/wf` sets {Kind: "command", Name: "wf"}. Programmatic embedders default to
// {Kind: "programmatic"}. External trigger sources (webhook, cron, sibling
// spawn) set {Kind: "external", Source, Ref?} so post-hoc readers can filter
// by origin. Meta is an open escape hatch for trigger-specific payload.
//
// Concurrency note: external triggers MUST gate their own spawning if a run
// is already in flight — the runtime does not enforce a process-wide mutex,
// and the names.json claim protocol is atomic per write but has no
// cross-process lock.
type RunTrigger struct {
	Kind   string         `json:"kind"`
	Name   string         `json:"name,omitempty"`
	Source string         `json:"source,omitempty"`
	Ref    string         `json:"ref,omitempty"`
	Meta   map[string]any `json:"meta,omitempty"`
}

// Trigger kinds.
const (
	TriggerKindCommand      = "command"
	TriggerKindProgrammatic = "programmatic"
	TriggerKindExternal     = "external"
)

// DefaultTrigger is the trigger a run gets when RunWorkflowOptions.Trigger is
// omitted. Treat it as frozen: it is shared by every run that omits a trigger,
// so a mutation (for example via Meta) would leak into unrelated runs.
var DefaultTrigger = RunTrigger{Kind: TriggerKindProgrammatic}
