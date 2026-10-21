package testkit

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/MichaelKinsy/PiG/extensions/sdk"
	host "github.com/juicesharp/rpiv-mono/pig/extensions/rpiv-host"
)

// Compile-time proof that *Fake implements every member of host.Host, and so
// every sibling's narrower subset of it.
var _ host.Host = (*Fake)(nil)

// Notification is one Notify call: the message and its level ("info",
// "warning" or "error").
type Notification struct {
	Message string
	Level   string
}

// StatusUpdate is one SetStatus call. An empty Text is a clear.
type StatusUpdate struct {
	Key  string
	Text string
}

// WidgetCall is one SetWidget call: the key, the content handed to the host
// ([]string lines, a kit.View, or nil for a clear) and the options passed.
type WidgetCall struct {
	Key     string
	Content any
	Options []sdk.WidgetOptions
}

// SentMessage is one SendMessage or SendCustomMessage call. Content is the
// string SendMessage was given, or the string or block list of the
// sdk.CustomMessage; Details is set by SendCustomMessage only.
type SentMessage struct {
	CustomType string
	Content    any
	Display    bool
	Details    any
	Opts       sdk.SendMessageOptions
}

// UserMessage is one SendUserMessage call.
type UserMessage struct {
	Content   any
	DeliverAs string
}

// AppendedEntry is one AppendEntry call.
type AppendedEntry struct {
	CustomType string
	Data       any
}

// Dialog is one Select, Confirm, Input or Editor call. Kind is "select",
// "confirm", "input" or "editor"; Text is the message, placeholder or
// prefill; Options are Select's choices.
type Dialog struct {
	Kind    string
	Title   string
	Text    string
	Options []string
}

// ExecCall is one Exec or ExecWithOptions call.
type ExecCall struct {
	Command string
	Args    []string
	Options sdk.ExecOptions
	// WithOptions reports an ExecWithOptions call.
	WithOptions bool
}

// CompleteCall is one Complete call.
type CompleteCall struct {
	Model   map[string]any
	Request map[string]any
	Auth    map[string]any
}

// StreamCall is one Stream or StreamSimple call.
type StreamCall struct {
	Model   map[string]any
	Request map[string]any
	Options map[string]any
	Simple  bool
}

// ExecuteToolCall is one ExecuteTool call.
type ExecuteToolCall struct {
	Name    string
	Args    any
	Options *sdk.ExecuteToolOptions
}

// ModelAuthCall is one GetModelAuth call.
type ModelAuthCall struct {
	Provider string
	ModelID  string
}

// Fake is a scripted, recording host.Host. Set the exported state fields and
// func hooks before handing it to the code under test; read the capture
// buckets after. Every method takes the mutex, so the code under test may
// call it from goroutines; the fields themselves are meant to be set and read
// from the test goroutine while no handler runs.
type Fake struct {
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc

	// Request and process facts.

	// CwdDir is Cwd. Default "/tmp/test-cwd".
	CwdDir string
	// ModeName is Mode: "tui", "rpc", "json" or "print". Default "tui".
	ModeName string
	// UI is HasUI. Default true. Without a UI, Custom returns nil without
	// recording and OnTerminalInput keeps no handler, as the SDK does.
	UI bool
	// TermWidth and TermHeight are Width and Height. Defaults 80 and 24.
	TermWidth  int
	TermHeight int
	// CallID is ToolCallID. Default "tc".
	CallID string
	// ConfigHomeDir is ConfigHome.
	ConfigHomeDir string
	// AgentDirPath is AgentDir; empty fails with sdk.ErrHostInfoUnavailable,
	// as sdk.AgentDir does outside a host.
	AgentDirPath string
	// ConfigDir is ConfigDirName. Default ".pig".
	ConfigDir string
	// HostVersion is Version. Default "0.5.0".
	HostVersion string

	// Flags, settings and prompts.

	// Flags answers GetFlag; an absent name is nil (Pi's undefined).
	Flags map[string]any
	// Settings answers GetSettings; nil is an empty object.
	Settings sdk.Settings
	// SystemPrompt answers GetSystemPrompt.
	SystemPrompt string
	// PromptOptions answers GetSystemPromptOptions.
	PromptOptions sdk.SystemPromptOptions

	// Tools and commands.

	// ActiveTools answers GetActiveTools; SetActiveTools and RegisterTool
	// change it.
	ActiveTools []string
	// AllTools answers GetAllTools.
	AllTools []sdk.ToolInfo
	// Commands answers GetCommands, followed by one "skill:<name>" command
	// with source "skill" per entry of Skills.
	Commands []sdk.CommandInfo
	// Skills are the skill names GetCommands lists, as the TypeScript mock's
	// skills option did.
	Skills []string

	// Session.

	// Entries answers GetEntries, BuildContextEntries and SessionGetBranch,
	// and GetBranch when Branch is nil (the root-to-leaf walk from LeafID,
	// decoded as sdk.BranchEntry).
	Entries []json.RawMessage
	// Branch answers GetBranch when set.
	Branch []sdk.BranchEntry
	// SessionID answers GetSessionID. Default "test-session".
	SessionID string
	// SessionFile answers GetSessionFile; empty is nil (an in-memory session).
	SessionFile string
	// SessionName answers GetSessionName; empty is nil (an unnamed session).
	SessionName string
	// LeafID answers GetLeafID and starts GetBranch's walk; empty uses the
	// last entry's id.
	LeafID string
	// SessionCtx answers BuildSessionContext when set; otherwise the context
	// is built from Entries, ThinkingLevel, ModelID and Provider.
	SessionCtx *sdk.SessionContext
	// Idle answers IsIdle. Default true.
	Idle bool
	// Pending answers HasPendingMessages.
	Pending bool
	// Trusted answers IsProjectTrusted. Default true.
	Trusted bool

	// Editor.

	// EditorText answers GetEditorText; SetEditorText changes it.
	EditorText string
	// ToolsExpanded answers GetToolsExpanded.
	ToolsExpanded bool

	// Models.

	// ModelID and Provider answer Model, ModelProvider and ModelQualified.
	ModelID  string
	Provider string
	// ModelInfo answers GetModelInfo; nil is no model.
	ModelInfo *sdk.ModelInfo
	// ThinkingLevel answers GetThinkingLevel. Default "medium".
	ThinkingLevel string
	// ContextUsage answers GetContextUsage; nil is unknown.
	ContextUsage *sdk.ContextUsage
	// Models answers AllModels and AvailableModels and is what FindModel
	// searches by "provider" and "id".
	Models []map[string]any
	// ProviderAuth answers ProviderAuthStatus; an absent provider is not
	// configured.
	ProviderAuth map[string]sdk.ProviderAuthStatus
	// ProviderNames answers ProviderDisplayName; an absent provider is named
	// by its id.
	ProviderNames map[string]string
	// APIKeys answers APIKeyForProvider; an absent provider has no key.
	APIKeys map[string]string
	// ModelAuth answers GetModelAuth; nil is {"ok":true,"apiKey":"test-key","headers":{}}.
	ModelAuth json.RawMessage

	// Themer answers Theme. Default TagStyler{}.
	Themer host.Styler

	// Errors scripts a failure by Host method name ("GetEntries", "Exec",
	// "SendMessage", ...): the method records its call and returns the error.
	Errors map[string]error

	// Answer hooks. A nil hook gives the default documented on each.

	// SelectFn answers Select; nil cancels ("", false, nil).
	SelectFn func(title string, options []string) (string, bool, error)
	// ConfirmFn answers Confirm; nil declines (false, nil).
	ConfirmFn func(title, message string) (bool, error)
	// InputFn answers Input; nil cancels.
	InputFn func(title, placeholder string) (string, bool, error)
	// EditorFn answers Editor; nil cancels.
	EditorFn func(title, prefill string) (string, bool, error)
	// CustomFn answers Custom; nil closes the component with (nil, nil).
	CustomFn func(component any, options any) (any, error)
	// ExecFn answers Exec and ExecWithOptions; nil returns a zero ExecResult.
	ExecFn func(command string, args []string, opts sdk.ExecOptions) (*sdk.ExecResult, error)
	// CompleteFn answers Complete; nil returns an empty JSON object.
	CompleteFn func(model, request, auth map[string]any) (json.RawMessage, error)
	// StreamFn answers Stream (simple false) and StreamSimple (simple true);
	// nil returns a ScriptedStream with no events and a nil result.
	StreamFn func(model, request, options map[string]any, simple bool) host.ModelStream
	// ExecuteToolFn answers ExecuteTool; nil returns an outcome whose
	// ToolCall names the tool and whose result is empty.
	ExecuteToolFn func(name string, args any, options *sdk.ExecuteToolOptions) (sdk.AgentToolCallOutcome, error)
	// RegisterToolFn observes RegisterTool after the definition is recorded.
	RegisterToolFn func(def sdk.ToolDefinition)
	// SetModelFn answers SetModel; nil records the model, splits a
	// "provider/model" into Provider and ModelID and succeeds.
	SetModelFn func(model string) (bool, error)
	// ReloadFn answers Reload after Reloaded is incremented; nil succeeds.
	ReloadFn func() error
	// WaitForIdleFn answers WaitForIdle after WaitedForIdle is incremented;
	// nil returns at once.
	WaitForIdleFn func() error
	// APIKeyAndHeadersFn answers APIKeyAndHeaders; nil returns
	// {"ok":true,"apiKey":"test-key","headers":{}}.
	APIKeyAndHeadersFn func(model map[string]any) (map[string]any, error)
	// AppendEntryFn observes AppendEntry after the entry is recorded and may
	// fail it.
	AppendEntryFn func(customType string, data any) error
	// SendMessageFn observes SendMessage and SendCustomMessage after the
	// message is recorded and may fail it.
	SendMessageFn func(msg SentMessage) error
	// SendUserMessageFn observes SendUserMessage after the message is
	// recorded and may fail it.
	SendUserMessageFn func(content any, deliverAs string) error

	// Capture buckets, in call order.

	Notifications []Notification
	// Statuses is the current status by key; an empty text clears the key.
	Statuses  map[string]string
	StatusLog []StatusUpdate
	Titles    []string
	// WorkingMessages records SetWorkingMessage; ClearWorkingMessage records
	// an empty string.
	WorkingMessages []string
	Dialogs         []Dialog
	// Widgets is the current widget by key; a nil content removes the key.
	Widgets   map[string]WidgetCall
	WidgetLog []WidgetCall
	// Footers and Headers record every SetFooter and SetHeader; a nil entry
	// is a clear. Footer and Header are the current lines.
	Footers [][]string
	Headers [][]string
	Footer  []string
	Header  []string
	// SentMessages records SendMessage and SendCustomMessage.
	SentMessages    []SentMessage
	UserMessages    []UserMessage
	AppendedEntries []AppendedEntry
	// ActiveToolCalls records every SetActiveTools argument.
	ActiveToolCalls [][]string
	RegisteredTools []sdk.ToolDefinition
	ExecutedTools   []ExecuteToolCall
	// Updates records OnUpdate partial results.
	Updates []any
	// Emitted records EmitEvent calls with the payload as emitted.
	Emitted []EmittedEvent
	// HandlerErrors records the errors event-bus handlers returned; EmitEvent
	// does not return them, as the host does not return them to the emitter.
	HandlerErrors []HandlerError
	// Customs records the components handed to Custom; CustomOptions the
	// options, in the same order.
	Customs       []any
	CustomOptions []any
	// EditorComponents records SetEditorComponent factories (nil restores).
	EditorComponents []sdk.EditorFactory
	EditorTextsSet   []string
	Pasted           []string
	SessionNamesSet  []string
	// TerminalInputHandlers and WidthHandlers are the live subscriptions;
	// FeedTerminalInput and Resize drive them.
	TerminalInputHandlers []sdk.TerminalInputHandler
	WidthHandlers         []sdk.WidthChangeHandler
	Compactions           []map[string]any
	Aborted               bool
	AbortCount            int
	Reloaded              int
	WaitedForIdle         int
	ModelsSet             []string
	ThinkingLevelsSet     []string
	Execs                 []ExecCall
	Completions           []CompleteCall
	Streams               []StreamCall
	ModelAuthCalls        []ModelAuthCall

	// Event bus.
	subscriptions []*subscription
	nextID        uint64
	terminalIDs   []uint64
	widthIDs      []uint64
}

// NewFake returns a Fake with the TypeScript createMockCtx defaults: cwd
// "/tmp/test-cwd", mode "tui", a UI, an 80x24 terminal, tool call id "tc",
// session "test-session", thinking level "medium", an idle and trusted host,
// a TagStyler theme, config dir name ".pig" and host version "0.5.0".
func NewFake() *Fake {
	ctx, cancel := context.WithCancel(context.Background())
	return &Fake{
		ctx:           ctx,
		cancel:        cancel,
		CwdDir:        "/tmp/test-cwd",
		ModeName:      "tui",
		UI:            true,
		TermWidth:     80,
		TermHeight:    24,
		CallID:        "tc",
		ConfigDir:     ".pig",
		HostVersion:   "0.5.0",
		SessionID:     "test-session",
		ThinkingLevel: "medium",
		Idle:          true,
		Trusted:       true,
		Themer:        TagStyler{},
		Flags:         map[string]any{},
		Errors:        map[string]error{},
		Statuses:      map[string]string{},
		Widgets:       map[string]WidgetCall{},
	}
}

// fail is the scripted failure of method, if any. The caller holds mu.
func (f *Fake) fail(method string) error {
	return f.Errors[method]
}

// Cancel ends the request: Done closes, Err and Signal report
// context.Canceled.
func (f *Fake) Cancel() { f.cancel() }

// Cwd is CwdDir.
func (f *Fake) Cwd() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.CwdDir
}

// Mode is ModeName.
func (f *Fake) Mode() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ModeName
}

// HasUI is UI.
func (f *Fake) HasUI() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.UI
}

// Width is TermWidth.
func (f *Fake) Width() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.TermWidth
}

// Height is TermHeight.
func (f *Fake) Height() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.TermHeight
}

// Done is the request context's Done; Cancel closes it.
func (f *Fake) Done() <-chan struct{} { return f.ctx.Done() }

// Err is the request context's Err.
func (f *Fake) Err() error { return f.ctx.Err() }

// Signal is the request context.
func (f *Fake) Signal() context.Context { return f.ctx }

// ToolCallID is CallID.
func (f *Fake) ToolCallID() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.CallID
}

// ConfigHome is ConfigHomeDir.
func (f *Fake) ConfigHome() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("ConfigHome"); err != nil {
		return "", err
	}
	return f.ConfigHomeDir, nil
}

// AgentDir is AgentDirPath; an empty path is sdk.ErrHostInfoUnavailable.
func (f *Fake) AgentDir() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("AgentDir"); err != nil {
		return "", err
	}
	if f.AgentDirPath == "" {
		return "", sdk.ErrHostInfoUnavailable
	}
	return f.AgentDirPath, nil
}

// ConfigDirName is ConfigDir.
func (f *Fake) ConfigDirName() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("ConfigDirName"); err != nil {
		return "", err
	}
	return f.ConfigDir, nil
}

// Version is HostVersion.
func (f *Fake) Version() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("Version"); err != nil {
		return "", err
	}
	return f.HostVersion, nil
}

// Theme is Themer, or a TagStyler when Themer is nil.
func (f *Fake) Theme() host.Styler {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Themer == nil {
		return TagStyler{}
	}
	return f.Themer
}
