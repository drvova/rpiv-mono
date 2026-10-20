// Package host is the narrow, fakeable view of the PiG host that every rpiv
// extension codes against instead of sdk.Context.
//
// # Why sdk.Context cannot be faked
//
// sdk.Context is a struct, not an interface. Its methods have value receivers
// and reach the host through unexported fields that point at the live
// extension connection; a zero Context has none of them, so almost every
// method of a zero value panics or returns a transport error. There is no
// constructor an extension can call in a test, and no way to substitute a
// method. Logic that takes sdk.Context directly can therefore only be
// exercised through a real host process.
//
// # The pattern
//
// Host is an interface whose method set has exactly the names and signatures
// of the sdk.Context methods rpiv extensions use. Because the names and
// signatures are identical, the adapter returned by FromContext is a struct
// that embeds sdk.Context and adds nothing for those methods: Go promotes
// them, and the compile-time assertions in adapter.go prove that the
// embedding satisfies Host. If PiG changes a signature, this module stops
// compiling instead of drifting.
//
// A few host facilities are not methods on sdk.Context but struct-typed
// facades returned by it (sdk.EventBus, sdk.SessionManager, sdk.ModelRegistry,
// sdk.UITheme) or package-level functions (sdk.AgentDir, sdk.Version). Those
// cannot be substituted either, so Host wraps each of them in a flat method
// whose dependencies are plain Go values:
//
//   - AgentDir and Version wrap sdk.AgentDir and sdk.Version.
//   - EmitEvent and SubscribeEvent wrap sdk.Context.Events(). A subscribed
//     handler receives only the payload; the request context the bus passes
//     is dropped because a fake has no context to offer.
//   - BuildSessionContext, BuildContextEntries and SessionGetBranch wrap
//     sdk.Context.SessionManager().
//   - Theme wraps sdk.Context.UITheme() behind Styler, the seven text styling
//     methods the rpiv TUI code uses (the same method set as rpiv-tui's
//     Styler, so values convert freely).
//   - AllModels, AvailableModels, ProviderAuthStatus, ProviderDisplayName and
//     APIKeyForProvider wrap sdk.Context.ModelRegistry() accessors; Stream and
//     StreamSimple wrap its streaming calls behind ModelStream, which
//     *sdk.ModelEventStream satisfies.
//
// # How extensions use it
//
// Each extension's extension.go is the only file that touches sdk.Context: a
// registered handler calls host.FromContext(ctx) and hands the result to the
// logic package. Each logic package declares its OWN narrower interface, a
// subset of Host's method set with the same signatures, so its dependencies
// on the host are visible at a glance:
//
//	type Host interface {
//		Notify(message, level string)
//		SetWidget(key string, content any, options ...sdk.WidgetOptions) error
//	}
//
// A host.Host value satisfies every such subset, and so does the testkit Fake
// (rpiv-testkit), which implements all of Host, records every effect, and
// takes scripted answers. Tests run the logic against the Fake; one wire-level
// smoke test per extension runs the real factory against an in-process host.
//
// # Limits
//
// Theme requires a live context: sdk.Context.UITheme reads the extension's
// replicated theme snapshot through the connection, so calling Theme on a
// Host built from a zero sdk.Context panics. The same holds for every
// delegated method that talks to the host; only AgentDir and Version (which
// read the PIG_EXT_HOST_INFO environment the host starts the process with) are
// usable without one. The module depends on nothing but the SDK.
package host
