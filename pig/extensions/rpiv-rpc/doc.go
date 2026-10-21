// Package rpc drives child PiG sessions: it starts `pig --mode rpc` as a
// subprocess, speaks JSON Lines with it over standard input and output, and
// exposes the protocol as typed Go calls, an event subscription and a relay
// for the dialogs the child's extensions open. The rpiv workflow engine runs
// its stages and the Agent tool runs its subagents through this package.
//
// # Design
//
// Start launches the process and returns a Client. Three goroutines serve it:
// a reader that splits standard output into records (LF only, one trailing CR
// removed, Unicode line separators never split a record) and parses each as
// JSON; a drain that copies standard error to Options.Stderr or discards it,
// so a full pipe never blocks the child; and an exit watcher that reaps the
// process, lets the readers drain, releases the pipes, joins the goroutines
// answering dialogs and closes Done.
//
// Every command carries an id and the child repeats it in the response, so
// Call correlates by id: several commands may be in flight and answers may
// arrive in any order, as the child answers a `parse` failure before earlier
// commands of the same burst. A cancelled context returns at once and forgets
// the id; an answer that still arrives is handed to subscribers as a response
// event. The typed wrappers (Prompt, GetState, Bash, Compact, ...) cover every
// command of PiG's rpc-commands reference and turn a success false response
// into a *CommandError whose Message is the child's free-text error. Once the
// child is gone a command fails with ErrClosed or ErrExited; the latter names
// the exit status, which the client waits a moment for, since the output
// closes just before the child is reaped.
//
// Every record that is not the answer to a pending command is an Event, which
// Subscribe delivers in subscription order on the reader goroutine. Event
// carries the type and the raw record; the typed decoders (AgentSettled,
// Message, ToolExecution, BashExecutionUpdate, ...) read the fields the rpiv
// packages use and keep the rest in Raw. Prompt returns when the child accepts
// the prompt; the answer streams as events and ends with agent_settled.
// WaitSettled blocks for that event, and Run combines prompt, event streaming
// and accounting (last assistant text, tool calls, usage, session stats) into
// one call, which is what a workflow stage needs.
//
// The child's extensions open dialogs (select, confirm, input, editor) as
// extension_ui_request records and wait for an extension_ui_response with the
// same id. The Client relays each dialog to the UIHandler on its own
// goroutine, with a context that ends on Close and carries the request's
// timeout, and writes the matching response; with no handler every dialog is
// cancelled at once, as the child does when input closes. An answer that
// cannot be written while the child still runs is reported to
// Options.OnError, the one failure no caller is waiting for. Fire-and-forget
// requests (notify, setStatus, setWidget, setTitle, set_editor_text) go to the
// UIObserver. Both kinds are also events, so a consumer can mirror the child's
// UI into its own.
//
// Close is the orderly shutdown: it stops event delivery, then closing
// standard input tells the child to finish, which is the protocol's own
// end-of-session signal; after Options.CloseGrace the process group is
// killed. On Unix the child leads its own process group, so the kill reaches
// the tools and extension processes it spawned and leaves no orphan;
// elsewhere only the child is killed. Close returns once the child is gone
// and never joins the readers, so it is safe to call from any callback, as
// PiG's own client's Stop is; Done and Wait join the readers and the dialog
// handlers, so they are awaited outside callbacks. The Start context bounds
// the process's lifetime: when it ends, the client closes.
//
// ResolvePig locates the executable: an explicit override, then $PIG_BIN,
// then pig on PATH, then (Linux) the first ancestor process whose executable
// is named pig, which is the binary hosting the extension that asks.
//
// # Host dependencies
//
// None from the SDK. The package is standard library only and needs a pig
// executable to run. The process boundary is the host: this package neither
// imports nor is imported by the extension runtime, so it can be used from an
// extension, a test or a plain program alike.
//
// # Config files
//
// None. PIG_BIN is the only environment variable the package reads; the child
// reads pig's own configuration (~/.pig, PIG_CODING_AGENT_DIR, provider keys)
// from the environment Options.Env extends.
//
// # Divergences
//
// The TypeScript packages create child sessions in process: rpiv-pi's SDK
// workflow host and pi-subagents build an AgentSession from Pi's
// createAgentSession and drive it through its event emitter. PiG's SDK offers
// no child sessions, so a child here is a `pig --mode rpc` process.
//
//   - A child costs a process start plus the model and extension loading of a
//     fresh pig, not a shared runtime; its extensions, tools and settings come
//     from its own command line (Options.Piglet, Extensions, Tools, ...) and
//     configuration, not from the parent's registry.
//   - Events are wire snapshots rather than live objects: a message_start may
//     already carry buffered content, intermediate records may carry provider
//     parser fields, and message_end is the authoritative message. Consumers
//     must not join deltas with message_end text.
//   - The parent cannot reach into the child's session manager. Session data
//     is read through the commands (GetEntries, GetTree, GetMessages,
//     GetSessionStats), and a prompt's result through events or
//     GetLastAssistantText.
//   - Dialogs the child's extensions open arrive as UI requests to relay
//     instead of being rendered by the parent's own UI context. The parent
//     answers them through UIHandler, so parked questions are routed where the
//     TypeScript host relayed ask_user_question prompts.
//   - The child's standard error is diagnostics, not protocol; the parent sees
//     it through Options.Stderr only.
//   - Shutdown is a process shutdown: Close closes standard input and kills
//     the process group after a grace period, where the TypeScript host
//     disposed an in-process session.
package rpc
