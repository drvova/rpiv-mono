package host

import (
	"context"

	"github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/json"
)

// Host is the view of sdk.Context that rpiv extensions code against. Every
// method up to the "Wrapped members" group has exactly the name and signature
// of the sdk.Context method it delegates to, so the adapter from FromContext
// is pure embedding and the compiler proves conformance. The wrapped members
// flatten the SDK's struct-typed facades (event bus, session manager, theme,
// model registry) and package-level host facts into plain methods a fake can
// implement.
type Host interface {
	// Request and process facts.
	Cwd() string
	Mode() string
	HasUI() bool
	Width() int
	Height() int
	Done() <-chan struct{}
	Err() error
	Signal() context.Context
	ToolCallID() string
	ConfigHome() (string, error)

	// Status surface.
	Notify(message, level string)
	SetStatus(key, text string)
	SetTitle(title string)
	SetWorkingMessage(message string)
	ClearWorkingMessage()

	// Dialogs and focused UI.
	Select(title string, options []string) (string, bool, error)
	Confirm(title, message string) (bool, error)
	Input(title, placeholder string) (string, bool, error)
	Editor(title, prefill string) (string, bool, error)
	Custom(factory any, options any) (any, error)

	// Editor, widgets and terminal surfaces.
	GetEditorText() (string, error)
	SetEditorText(text string)
	PasteToEditor(text string)
	SetEditorComponent(factory sdk.EditorFactory) error
	GetToolsExpanded() (bool, error)
	SetWidget(key string, content any, options ...sdk.WidgetOptions) error
	SetFooter(lines []string) error
	SetHeader(lines []string) error
	OnTerminalInput(handler sdk.TerminalInputHandler) (func(), error)
	OnWidthChange(handler sdk.WidthChangeHandler) (func(), error)

	// Messages and session entries.
	SendMessage(customType, content string, display bool, opts sdk.SendMessageOptions) error
	SendCustomMessage(msg sdk.CustomMessage, opts sdk.SendMessageOptions) error
	SendUserMessage(content any, deliverAs string) error
	AppendEntry(customType string, data any) error
	GetEntries() ([]json.RawMessage, error)
	GetBranch() ([]sdk.BranchEntry, error)

	// Session identity and lifecycle.
	GetSessionID() (string, error)
	GetSessionFile() (*string, error)
	GetSessionName() (*string, error)
	SetSessionName(name string) error
	GetLeafID() (*string, error)
	IsIdle() (bool, error)
	WaitForIdle() error
	HasPendingMessages() (bool, error)
	IsProjectTrusted() (bool, error)
	Abort()
	Reload() error
	Compact(opts map[string]any)

	// Tools and commands.
	GetActiveTools() ([]string, error)
	SetActiveTools(tools []string)
	GetAllTools() ([]sdk.ToolInfo, error)
	GetCommands() ([]sdk.CommandInfo, error)
	RegisterTool(definition sdk.ToolDefinition)
	ExecuteTool(name string, args any, options *sdk.ExecuteToolOptions) (sdk.AgentToolCallOutcome, error)
	OnUpdate(partial any) error

	// Flags, settings and prompts.
	GetFlag(name string) (any, error)
	GetSettings() (sdk.Settings, error)
	GetSystemPrompt() (string, error)
	GetSystemPromptOptions() (sdk.SystemPromptOptions, error)

	// Models.
	Model() string
	ModelProvider() string
	ModelQualified() string
	GetModelInfo() (*sdk.ModelInfo, error)
	SetModel(model string) (bool, error)
	GetThinkingLevel() (string, error)
	SetThinkingLevel(level string)
	GetContextUsage() (*sdk.ContextUsage, error)
	GetModelAuth(providerID, modelID string) (json.RawMessage, error)
	Complete(model, request, auth map[string]any) (json.RawMessage, error)

	// Processes.
	Exec(command string, args []string) (*sdk.ExecResult, error)
	ExecWithOptions(command string, args []string, opts sdk.ExecOptions) (*sdk.ExecResult, error)

	// Wrapped members: host facilities whose SDK form is an unfakeable struct
	// or a package-level function. See the package documentation.

	// AgentDir is sdk.AgentDir: the host's agent configuration directory
	// (~/.pig/agent). It is readable while the factory runs.
	AgentDir() (string, error)
	// Version is sdk.Version: the Pi version the host implements.
	Version() (string, error)

	// EmitEvent publishes data on an event bus channel; every listener's work
	// finishes before it returns.
	EmitEvent(channel string, data any) error
	// SubscribeEvent registers handler on a channel and returns the function
	// that unsubscribes it. The handler sees the emitter's JSON value: objects
	// as map[string]any, arrays as []any, numbers as float64, nil for an emit
	// of undefined.
	SubscribeEvent(channel string, handler func(data any) error) (func(), error)

	// BuildSessionContext is SessionManager().BuildSessionContext.
	BuildSessionContext() (sdk.SessionContext, error)
	// BuildContextEntries is SessionManager().BuildContextEntries.
	BuildContextEntries() ([]map[string]any, error)
	// SessionGetBranch is SessionManager().GetBranch: the entries from the
	// root to fromID (the leaf when fromID is nil).
	SessionGetBranch(fromID *string) ([]map[string]any, error)

	// Theme is the host's active theme as a Styler. It requires a live
	// context.
	Theme() Styler

	// AllModels is ModelRegistry().GetAll.
	AllModels() ([]map[string]any, error)
	// AvailableModels is ModelRegistry().GetAvailable: the models whose
	// provider has configured auth.
	AvailableModels() ([]map[string]any, error)
	// ProviderAuthStatus is ModelRegistry().GetProviderAuthStatus.
	ProviderAuthStatus(provider string) (sdk.ProviderAuthStatus, error)
	// ProviderDisplayName is ModelRegistry().GetProviderDisplayName.
	ProviderDisplayName(provider string) (string, error)
	// APIKeyForProvider is ModelRegistry().GetApiKeyForProvider; nil when the
	// provider has no key.
	APIKeyForProvider(provider string) *string
	// StreamSimple is ModelRegistry().StreamSimple: a streamed completion
	// through the host with Pi's streamSimple semantics.
	StreamSimple(model, request, options map[string]any) ModelStream
	// Stream is ModelRegistry().Stream: a streamed completion through the host
	// with Pi's stream semantics.
	Stream(model, request, options map[string]any) ModelStream
}

// Styler is the text styling surface of a Pi theme: Fg and Bg color text with
// a theme token ("accent", "muted", "warning", "error", "success", "dim",
// "border", ...) and the modifiers wrap text in one SGR attribute. sdk.UITheme
// satisfies it, as does rpiv-tui's Styler (same method set).
type Styler interface {
	Fg(token, text string) string
	Bg(token, text string) string
	Bold(text string) string
	Italic(text string) string
	Underline(text string) string
	Strikethrough(text string) string
	Inverse(text string) string
}

// ModelStream is a streamed model completion. Events yields each provider
// event until the terminal event or ctx is done; Result blocks until the
// stream ends and returns the final assistant message. *sdk.ModelEventStream
// satisfies it.
type ModelStream interface {
	Events(ctx context.Context) <-chan map[string]any
	Result() map[string]any
}
