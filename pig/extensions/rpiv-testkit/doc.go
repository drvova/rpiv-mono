// Package testkit is the test fixture library of the rpiv PiG extensions, the
// Go counterpart of @juicesharp/rpiv-test-utils: a scripted, recording fake of
// the host (Fake), an in-process host that speaks PiG's real extension wire
// (WireHost), session-entry builders, tool-contract assertions, and doubles
// for the environment an extension touches (HTTP, shell commands, files,
// standard output). It is imported by *_test.go files only.
//
// # Fake: unit tests of the logic
//
// Fake implements every member of host.Host (rpiv-host), so it satisfies any
// sibling's narrower host interface. NewFake returns one with the defaults the
// TypeScript createMockCtx had: cwd "/tmp/test-cwd", mode "tui", a UI, an
// 80x24 terminal, session "test-session", thinking level "medium", an idle and
// trusted host. Scripted state lives in exported fields (CwdDir, ModeName, UI,
// Flags, Entries, ModelID, ...), answers to the calls that wait on the user or
// the host are func fields (SelectFn, ExecFn, CompleteFn, ...), whose nil
// default cancels a dialog, runs nothing and completes with an empty object,
// and every effect lands in a capture bucket (Notifications, Statuses,
// Widgets, SentMessages, AppendedEntries, Emitted, ...). Errors scripts a
// failure by method name. The event bus is in process: SubscribeEvent
// registers handlers and EmitEvent runs them in registration order on the
// JSON round trip of the payload, as the host delivers it. Every method takes
// the fake's mutex, so handlers may use it from goroutines; Done, Signal and
// Err come from a context that Cancel ends.
//
//	f := testkit.NewFake()
//	f.Flags["plan"] = true
//	out, err := todo.Execute(f, params)
//	if f.Widgets["todo"].Content == nil { ... }
//
// Fake hands its own handlers a zero sdk.Context where the SDK's signature
// requires one (WidthChangeHandler); a handler that needs the host must
// close over the host value it was built with rather than adapt that context.
//
// # WireHost: the smoke test of the factory
//
// Fake cannot exercise extension.go: the factory's registrations and the
// host.FromContext adapters only run against a real sdk.Context, which only a
// host provides. WireHost is that host, in process: NewWireHost runs the
// extension's RunWithConn over one end of a net.Pipe and speaks PiG's
// extension wire on the other (4-byte big-endian length-prefixed JSON
// envelopes: register, ready with its state snapshot, request/response,
// call/call_result, notify, widget_push, request_state), as the subprocess
// host does. Register returns the decoded register payload; InvokeTool,
// InvokeCommand, InvokeEvent and InvokeShortcut send host requests and return
// the response; every host call the extension makes while a request runs is
// answered by a script from WireOptions.Calls or by a default that mirrors a
// quiet host, and recorded for Calls and CallsFor; widget pushes and
// notifications are recorded too. The host side runs on its own goroutine, so
// calls the extension makes outside a request (event-bus subscriptions made
// in the factory, late registrations) are served as well, and an emit on the
// event bus loops back to the extension's own listeners as the shared bus
// does. Close sends shutdown, closes the transport and fails the test when
// the extension does not stop.
//
// The rule for every rpiv extension module: the TypeScript suite's cases are
// ported against Fake, and the module has at least one WireHost smoke test
// that runs its Extension() factory, asserts the registration shape (tool
// names and schemas, commands, flags, shortcuts, handlers) and drives one tool
// or command round trip through the wire. That one test is what proves the
// wiring in extension.go, which no Fake-based test touches.
//
// # Fixtures
//
// UserMessage, AssistantMessage, ToolResultMessage, ToolResultEntry,
// CustomEntry and AssistantEntry build Pi-shaped messages and entries;
// BuildSessionEntries chains them root to leaf with ids, parentIds and
// timestamps as a session file has them, and BranchEntries decodes the same
// chain into sdk.BranchEntry values. AssertToolContract checks a tool's name,
// description and parameter schema against a ToolContract, for an
// sdk.ToolDefinition, a RegisteredTool from a WireHost registration or an
// sdk.ToolInfo. FetchStub is an http.RoundTripper driven by matchers that
// records every request; ExecStub scripts shell results by command prefix for
// Fake.ExecFn, and GitExec is the git stub the session hooks tests use;
// WriteGuidanceTree writes a tree of files under a directory; CaptureStdout
// redirects os.Stdout for one test.
//
// # Host dependencies
//
// The package depends on the SDK (types and, for WireHost, *sdk.Extension)
// and on rpiv-host for the Host interface and the Styler alias. It never
// talks to a running host.
//
// # Config files
//
// None.
//
// # Divergences
//
// The TypeScript package's shape follows Pi's ExtensionAPI (a {pi, captured}
// pair of registration spies); the Go port follows the host.Host interface,
// so registration is observed through WireHost.Register instead of a captured
// map, and the Fake records RegisterTool calls made at run time only.
//
//   - createMockCommandCtx, createMockSessionChain and concurrent-host.ts
//     model rpiv-workflow's spawnChild host port; child sessions are rpiv-rpc
//     processes in PiG, so those fixtures belong to the rpiv-workflow module.
//   - makeSpawnStub scripted a Node child process; the PiG port of rpiv-pi
//     does not spawn `pi install` (it reports missing siblings), and shell
//     commands run through the host's Exec, which ExecStub scripts.
//   - verifyShipManifest checked an npm `files` array; a Go module has no
//     such manifest.
//   - mockStdout pinned process.stdout.isTTY; Go code that must know whether
//     standard output is a terminal takes that as an injectable check, so
//     CaptureStdout only redirects the stream.
//   - makeTui's requestRender spy is InvalidateSpy, the callback a
//     sdk.RemoteComponent receives through SetInvalidate.
//   - WireHost answers the extension's event-bus emit by dispatching to the
//     extension's own listeners; the real host would also reach other
//     extensions' listeners, which a test has none of.
package testkit
