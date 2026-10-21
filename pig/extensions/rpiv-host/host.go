package host

import (
	"context"

	"github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/json"
	tui "github.com/juicesharp/rpiv-mono/pig/extensions/rpiv-tui"
)

// Host is the view of sdk.Context that rpiv extensions code against. Every
// method up to the "Wrapped members" group has exactly the name and signature
// of the sdk.Context method it delegates to, so the adapter from FromContext
// is pure embedding and the compiler proves conformance. The wrapped members
// flatten the SDK's struct-typed facades (event bus, session manager, theme,
// model registry) and package-level host facts into plain methods a fake can
// implement.
//
// On a Host built from a zero sdk.Context only Done, Err and ToolCallID
// (which return their zero values) and the host facts AgentDir, ConfigDirName
// and Version (which read the process environment) work; every other method
// talks to the host and panics without a live context.
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
	// (~/.pig/agent). It reads the PIG_EXT_HOST_INFO environment, so it works
	// without a live context, while the factory runs.
	AgentDir() (string, error)
	// ConfigDirName is sdk.ConfigDirName: the per-project configuration
	// directory name (".pig", or ".pi" when the host shares Pi's tree) for
	// <cwd>/<name>/... paths such as the project agents directory. It reads
	// the PIG_EXT_HOST_INFO environment, so it works without a live context,
	// while the factory runs.
	ConfigDirName() (string, error)
	// Version is sdk.Version: the Pi version the host implements. It reads
	// the PIG_EXT_HOST_INFO environment, so it works without a live context,
	// while the factory runs.
	Version() (string, error)

	// EmitEvent publishes data on an event bus channel; every listener's work
	// finishes before it returns. It requires a live context.
	EmitEvent(channel string, data any) error
	// SubscribeEvent registers handler on a channel and returns the function
	// that unsubscribes it. The handler sees the emitter's JSON value: objects
	// as map[string]any, arrays as []any, numbers as float64, nil for an emit
	// of undefined. A nil handler is rejected with ErrNilHandler and a no-op
	// unsubscribe before the host is asked. It requires a live context.
	SubscribeEvent(channel string, handler func(data any) error) (func(), error)

	// BuildSessionContext is SessionManager().BuildSessionContext. It requires
	// a live context.
	BuildSessionContext() (sdk.SessionContext, error)
	// BuildContextEntries is SessionManager().BuildContextEntries. It requires
	// a live context.
	BuildContextEntries() ([]map[string]any, error)
	// SessionGetBranch is SessionManager().GetBranch: the entries from the
	// root to fromID (the leaf when fromID is nil). It requires a live
	// context.
	SessionGetBranch(fromID *string) ([]map[string]any, error)

	// Theme is the host's active theme as a tui.Styler. It requires a live
	// context.
	Theme() tui.Styler

	// AllModels is ModelRegistry().GetAll. It requires a live context.
	AllModels() ([]map[string]any, error)
	// AvailableModels is ModelRegistry().GetAvailable: the models whose
	// provider has configured auth. It requires a live context.
	AvailableModels() ([]map[string]any, error)
	// FindModel is ModelRegistry().Find: the registry's model with exactly
	// this provider and model ID, or nil when it has none (Pi's
	// modelRegistry.find returning undefined). It requires a live context.
	FindModel(providerID, modelID string) map[string]any
	// ProviderAuthStatus is ModelRegistry().GetProviderAuthStatus. It requires
	// a live context.
	ProviderAuthStatus(provider string) (sdk.ProviderAuthStatus, error)
	// ProviderDisplayName is ModelRegistry().GetProviderDisplayName. It
	// requires a live context.
	ProviderDisplayName(provider string) (string, error)
	// APIKeyForProvider is ModelRegistry().GetApiKeyForProvider; nil when the
	// provider has no key. It requires a live context.
	APIKeyForProvider(provider string) *string
	// APIKeyAndHeaders is ModelRegistry().GetApiKeyAndHeaders: the request
	// authentication the host resolves for model, a registry model map read
	// through its "provider" and "id" (or "modelId") fields, in the shape of
	// Pi's getApiKeyAndHeaders result ({ok, apiKey, headers} or {ok: false,
	// error}). It requires a live context.
	APIKeyAndHeaders(model map[string]any) (map[string]any, error)
	// StreamSimple is ModelRegistry().StreamSimple: a streamed completion
	// through the host with Pi's streamSimple semantics. It requires a live
	// context.
	StreamSimple(model, request, options map[string]any) ModelStream
	// Stream is ModelRegistry().Stream: a streamed completion through the host
	// with Pi's stream semantics. It requires a live context.
	Stream(model, request, options map[string]any) ModelStream
}

// Styler is rpiv-tui's Styler, the text styling surface of a Pi theme: Fg and
// Bg color text with a theme token ("accent", "muted", "warning", "error",
// "success", "dim", "border", ...) and the modifiers wrap text in one SGR
// attribute. It is a type alias, not a distinct type, so Theme's result and a
// sibling interface's `Theme() tui.Styler` are the same method signature.
// sdk.UITheme satisfies it.
type Styler = tui.Styler

// ModelStream is a streamed model completion. Events yields each provider
// event until the terminal event or ctx is done; Result blocks until the
// stream ends and returns the final assistant message. *sdk.ModelEventStream
// satisfies it.
type ModelStream interface {
	Events(ctx context.Context) <-chan map[string]any
	Result() map[string]any
}
