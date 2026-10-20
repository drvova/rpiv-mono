package host

import "github.com/MichaelKinsy/PiG/extensions/sdk"

// contextHost adapts a live sdk.Context to Host. Every Host method that
// mirrors an sdk.Context method is promoted from the embedded value; only the
// wrapped members are declared here.
type contextHost struct {
	sdk.Context
}

// Compile-time proofs. None of these constructs or calls anything at runtime:
// the typed nil pointer carries the promoted and declared method set, the
// function assertion pins FromContext's signature, and the zero sdk.UITheme
// and nil *sdk.ModelEventStream are only checked for their method sets.
var (
	_ Host                   = (*contextHost)(nil)
	_ func(sdk.Context) Host = FromContext
	_ Styler                 = sdk.UITheme{}
	_ ModelStream            = (*sdk.ModelEventStream)(nil)
)

// FromContext returns the Host view of ctx. It copies the context value (as
// the SDK itself does) and performs no host call, so it is safe to use in a
// factory; the returned Host is as live as ctx is.
func FromContext(ctx sdk.Context) Host {
	return contextHost{Context: ctx}
}

// AgentDir is sdk.AgentDir.
func (contextHost) AgentDir() (string, error) { return sdk.AgentDir() }

// Version is sdk.Version.
func (contextHost) Version() (string, error) { return sdk.Version() }

// EmitEvent publishes on the request-bound event bus.
func (h contextHost) EmitEvent(channel string, data any) error {
	return h.Context.Events().Emit(channel, data)
}

// SubscribeEvent registers handler on the request-bound event bus. The bus
// hands each dispatch its own sdk.Context, which the handler does not see.
func (h contextHost) SubscribeEvent(channel string, handler func(data any) error) (func(), error) {
	return h.Context.Events().On(channel, busHandler(handler))
}

// busHandler adapts a payload-only handler to the event bus's handler type by
// dropping the dispatch context.
func busHandler(handler func(data any) error) sdk.EventBusHandler {
	return func(_ sdk.Context, data any) error { return handler(data) }
}

// BuildSessionContext is SessionManager().BuildSessionContext.
func (h contextHost) BuildSessionContext() (sdk.SessionContext, error) {
	return h.Context.SessionManager().BuildSessionContext()
}

// BuildContextEntries is SessionManager().BuildContextEntries.
func (h contextHost) BuildContextEntries() ([]map[string]any, error) {
	return h.Context.SessionManager().BuildContextEntries()
}

// SessionGetBranch is SessionManager().GetBranch.
func (h contextHost) SessionGetBranch(fromID *string) ([]map[string]any, error) {
	return h.Context.SessionManager().GetBranch(fromID)
}

// Theme is UITheme() as a Styler. It panics on a Host built from a zero
// sdk.Context, because the SDK reads the theme snapshot through the
// extension connection.
func (h contextHost) Theme() Styler { return h.Context.UITheme() }

// AllModels is ModelRegistry().GetAll.
func (h contextHost) AllModels() ([]map[string]any, error) {
	return h.Context.ModelRegistry().GetAll()
}

// AvailableModels is ModelRegistry().GetAvailable.
func (h contextHost) AvailableModels() ([]map[string]any, error) {
	return h.Context.ModelRegistry().GetAvailable()
}

// ProviderAuthStatus is ModelRegistry().GetProviderAuthStatus.
func (h contextHost) ProviderAuthStatus(provider string) (sdk.ProviderAuthStatus, error) {
	return h.Context.ModelRegistry().GetProviderAuthStatus(provider)
}

// ProviderDisplayName is ModelRegistry().GetProviderDisplayName.
func (h contextHost) ProviderDisplayName(provider string) (string, error) {
	return h.Context.ModelRegistry().GetProviderDisplayName(provider)
}

// APIKeyForProvider is ModelRegistry().GetApiKeyForProvider.
func (h contextHost) APIKeyForProvider(provider string) *string {
	return h.Context.ModelRegistry().GetApiKeyForProvider(provider)
}

// StreamSimple is ModelRegistry().StreamSimple. The SDK always returns a
// non-nil stream, so the interface value is never a typed nil.
func (h contextHost) StreamSimple(model, request, options map[string]any) ModelStream {
	return h.Context.ModelRegistry().StreamSimple(model, request, options)
}

// Stream is ModelRegistry().Stream. The SDK always returns a non-nil stream,
// so the interface value is never a typed nil.
func (h contextHost) Stream(model, request, options map[string]any) ModelStream {
	return h.Context.ModelRegistry().Stream(model, request, options)
}
