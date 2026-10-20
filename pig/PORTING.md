# Porting rpiv-mono to PiG (Pi in Go): the shared brief

This file is the contract every porting agent and every reviewer works from. Read it fully before touching
`pig/`. It states facts about the target host (PiG), the layout of the Go port, the fidelity rules, the
testing contract, and the per-package deliverable checklist. Where this brief and a package's TypeScript
source disagree about *behavior*, the TypeScript source wins (the port is a translation, not a redesign).
Where they disagree about *mechanics* (how to reach the host, how to test), this brief wins.

## 1. What is being ported, and to what

- Source: the fourteen `@juicesharp/rpiv-*` packages under `packages/` (TypeScript Pi extensions plus two
  libraries and the test fixtures). `packages/rpiv-site` is a website and is NOT ported.
- Target: **PiG** (`github.com/MichaelKinsy/PiG`, docs at pi-in-go.dev), a Go port of Pi. PiG runs extensions
  as subprocesses that speak a length-prefixed JSON wire to the host. A Go extension is an importable Go
  package exposing exactly one factory:

  ```go
  func Extension() *sdk.Extension   // sdk = "github.com/MichaelKinsy/PiG/extensions/sdk"
  ```

  PiG discovers the factory by its exact signature, generates the runner, builds it with `go build`
  (`GOWORK=off`), and talks to it over a Unix socket. The extension's registered name must equal the
  directory name PiG selected (`sdk.New("rpiv-todo")` in `pig/extensions/rpiv-todo/`).
- Composition: a **Piglet** (YAML manifest) selects extensions and skills. Ours is `piglet.yaml` at the
  repository root; it points at `pig/extensions/<name>` and at the unchanged skills in
  `packages/rpiv-pi/skills/<name>`. Skills and agent profiles stay single-sourced in `packages/rpiv-pi/`.
- A local `pig` binary built from the PiG checkout is at
  `/tmp/claude-0/-home-user-rpiv-mono/69046b9c-d7f1-58e6-848f-3c05629a3109/scratchpad/pig-bin/pig`
  (version `0.5.0+1.3.0.1402973a`). Use it with `HOME` pointed at a scratch directory, for example
  `HOME=/tmp/claude-0/-home-user-rpiv-mono/69046b9c-d7f1-58e6-848f-3c05629a3109/scratchpad/pighome`.
  The PiG source checkout is at `/home/user/michaelkinsy/pig` (read it; never modify it).

## 2. Layout of the port

```
piglet.yaml                      — the rpiv Piglet (repo root, so local origins stay inside the repo)
pig/
  PORTING.md                     — this brief
  README.md                      — front door for the Go port
  go.work                        — development workspace over every module below (PiG itself uses GOWORK=off)
  Makefile                       — fmt / vet / test / validate targets
  extensions/
    rpiv-config/                 — library: XDG config paths, JSON config I/O, guidance fields, model-key codec
    rpiv-tui/                    — library: ANSI-aware width/truncate/wrap, key matching, Styler interface
    rpiv-host/                   — library: the narrow, fakeable view of sdk.Context (host.Host) + adapter
    rpiv-testkit/                — library: fakes (testkit.Fake), wire-level host harness (testkit.WireHost), fixtures
    rpiv-rpc/                    — library: client for `pig --mode rpc` child sessions (JSON Lines over stdio)
    rpiv-i18n/                   — extension + library
    rpiv-args/                   — extension
    rpiv-ask-user-question/      — extension
    rpiv-todo/                   — extension
    rpiv-advisor/                — extension
    rpiv-web-tools/              — extension
    rpiv-agents/                 — extension (NEW: the `Agent` subagent dispatcher, replaces @tintinweb/pi-subagents)
    rpiv-workflow/               — extension + library (/wf engine)
    rpiv-btw/                    — extension (opt-in)
    rpiv-voice/                  — extension (opt-in)
    rpiv-warp/                   — extension (opt-in)
    rpiv-telemetry/              — extension (private in the TS world; ported, not selected by piglet.yaml)
    rpiv-pi/                     — extension (rpiv-core: hooks, /rpiv-* commands, lanes, models, built-in workflows)
```

Every directory under `pig/extensions/` is its own Go module:

- Module path: `github.com/juicesharp/rpiv-mono/pig/extensions/<dir>`.
- `go.mod` says `go 1.26`. Extensions require `github.com/MichaelKinsy/PiG/extensions/sdk v0.5.0` with NO
  replace directive (PiG substitutes its own staged SDK at build time; `go test` resolves v0.5.0 from the
  module proxy). Sibling libraries are required at `v0.0.0` with a relative replace:

  ```
  require github.com/juicesharp/rpiv-mono/pig/extensions/rpiv-config v0.0.0
  replace github.com/juicesharp/rpiv-mono/pig/extensions/rpiv-config => ../rpiv-config
  ```

  PiG copies these replace directives into the runner module it generates, so relative replaces work in
  every realization (isolated, packed, fused).
- Extension modules contain `extension.go` (the factory, thin wiring only), `doc.go`, subpackages for the
  logic, and `*_test.go` next to what they test. Libraries have no factory.
- Assets the TypeScript packages ship as files (prompts, locales, bundled agent profiles) are COPIED into the
  Go module and embedded with `//go:embed`. Each such copy gets a lockstep test that reads the original under
  `../../../packages/<pkg>/...` and asserts byte equality (skip the test with `t.Skip` when the original is
  absent, so the module still tests when checked out alone).
- Never import the PiG root module `github.com/MichaelKinsy/PiG` (86 MB module zip). The SDK module and the
  small libraries listed in §5 are the only external code allowed.

## 3. PiG facts you must respect

### 3.1 The SDK surface

`sdk.Context` is a STRUCT (value receiver methods), not an interface. Handlers receive it by value. You
cannot construct one in a test, so logic never takes `sdk.Context` directly (see §4). Key signatures:

```go
// registration (on *sdk.Extension)
Tool(name, description string, schema sdk.Schema, handler sdk.ToolFunc)          // ToolFunc = func(ctx sdk.Context, params map[string]any) (any, error)
RegisterTool(def sdk.ToolDefinition)   // full form: Label, PromptSnippet, PromptGuidelines, Parameters, RenderCall/RenderResult, ...; re-registering a name replaces it
Command(name, description string, handler sdk.CommandFunc)                       // CommandFunc = func(ctx sdk.Context, args string) error
Flag(name string, options sdk.FlagOptions)                                       // FlagOptions{Description, Type: sdk.FlagBoolean|sdk.FlagString, Default}
Shortcut(key, description string, handler sdk.ShortcutFunc)                      // key ids like "ctrl+shift+t"
OnEvent(eventName string, handler sdk.EventFunc) func()                          // EventFunc = func(ctx sdk.Context, data map[string]any) (any, error)
OnSessionStart(handler sdk.EventFunc) func()   OnSessionShutdown(...)  OnToolResult(...)
MessageRenderer(customType string, handler sdk.RendererFunc)                     // func(ctx, message map[string]any, options, width int) ([]string, error)
EntryRenderer(customType string, handler sdk.EntryRendererFunc)
MarkdownTransformer(func(markdown string, context sdk.MarkdownTransformContext) string)
Events() sdk.EventBus                                                            // bus: On(channel, func(ctx sdk.Context, data any) error) (func(), error); Emit(channel, data any) error

// sdk.Context (selection; see extensions/sdk/context.go for all)
Cwd() string; Mode() string /* "tui"|"rpc"|"json"|"print" */; HasUI() bool; Width() int; Height() int; Done() <-chan struct{}; Err() error; Signal() context.Context
Notify(message, level string) /* "info"|"warning"|"error" */; SetStatus(key, text string); SetTitle(string)
Select(title string, options []string) (string, bool, error); Confirm(title, message string) (bool, error)
Input(title, placeholder string) (string, bool, error); Editor(title, prefill string) (string, bool, error)
Custom(component any, options any) (any, error)                                  // component: sdk.RemoteComponent or sdk.ViewComponent; options: sdk.RemoteOverlayOptions
SetWidget(key string, content any, options ...sdk.WidgetOptions) error           // content []string (lines) or kit.View; options map e.g. {"placement":"aboveEditor"}
SetFooter([]string) error; SetHeader([]string) error; OnTerminalInput(sdk.TerminalInputHandler) (func(), error); OnWidthChange(sdk.WidthChangeHandler) (func(), error)
GetEditorText() (string, error); SetEditorText(string); PasteToEditor(string); SetEditorComponent(sdk.EditorFactory) error; GetToolsExpanded() (bool, error)
SendMessage(customType, content string, display bool, opts sdk.SendMessageOptions) error  // opts{TriggerTurn *bool (sdk.Bool), DeliverAs "steer"|"followUp"|"nextTurn"}
SendCustomMessage(sdk.CustomMessage, sdk.SendMessageOptions) error; SendUserMessage(content any, deliverAs string) error
AppendEntry(customType string, data any) error; GetEntries() ([]json.RawMessage, error); GetBranch() ([]sdk.BranchEntry, error)
GetSessionID() (string, error); GetSessionFile() (*string, error); GetSessionName() (*string, error); SetSessionName(string) error; GetLeafID() (*string, error)
SessionManager() sdk.SessionManager   // GetEntries, GetBranch(fromID), BuildSessionContext() (sdk.SessionContext{...Messages}), BuildContextEntries(), GetTree ...
GetFlag(name string) (any, error); GetSettings() (sdk.Settings, error); GetSystemPrompt() (string, error); GetSystemPromptOptions() (sdk.SystemPromptOptions, error)
GetActiveTools() ([]string, error); SetActiveTools([]string); GetAllTools() ([]sdk.ToolInfo, error); GetCommands() ([]sdk.CommandInfo, error)
ExecuteTool(name string, args any, options *sdk.ExecuteToolOptions) (sdk.AgentToolCallOutcome, error); OnUpdate(partial any) error; ToolCallID() string
Model() string; ModelProvider() string; ModelQualified() string; GetModelInfo() (*sdk.ModelInfo, error); SetModel(string) (bool, error)
GetThinkingLevel() (string, error); SetThinkingLevel(string); GetContextUsage() (*sdk.ContextUsage, error)
ModelRegistry() sdk.ModelRegistry     // GetAll/GetAvailable() ([]map[string]any, error), GetProviderAuthStatus, GetProviderDisplayName, GetApiKeyForProvider, Stream/StreamSimple/Complete(model, request, options)
Complete(model, request, auth map[string]any) (json.RawMessage, error)           // one-shot completion through the host (Pi completeSimple)
Exec(command string, args []string) (*sdk.ExecResult, error); ExecWithOptions(command string, args []string, opts sdk.ExecOptions) (*sdk.ExecResult, error) // ExecResult{Stdout, Stderr, ExitCode, Killed}; ExecOptions{Timeout float64 ms, Cwd}
UITheme() sdk.UITheme                 // Fg(token, text), Bg, Bold, Italic, Underline, Strikethrough, Inverse; tokens are Pi theme tokens ("accent","muted","warning","error","success","dim","border",...)
IsIdle() (bool, error); WaitForIdle() error; IsProjectTrusted() (bool, error); Abort(); Reload() error; Compact(opts map[string]any)
ConfigHome() (string, error)          // PiG's config root (~/.pig); rpiv config stays under ~/.config/rpiv-* via rpiv-config
```

Package-level helpers in the SDK you should REUSE instead of re-porting Pi internals:
`sdk.AgentDir()` (Pi getAgentDir(): `~/.pig/agent`), `sdk.ConvertToLlm`, `sdk.SerializeConversation`,
`sdk.TruncateHead/TruncateTail/TruncateLine/FormatSize` (+ `sdk.TruncationOptions`),
`sdk.EstimateTokens`, `sdk.CalculateContextTokens`, `sdk.FindCutPoint`, `sdk.FindTurnStartIndex`,
`sdk.GetLastAssistantUsage`, `sdk.GetLatestCompactionEntry`, `sdk.ParseFrontmatter/StripFrontmatter`,
`sdk.ParseSkillBlock`, `sdk.Param[T]`, `sdk.NewToolError`, `sdk.Bool`, `sdk.IsBashToolResult` and friends,
`sdk.WithFileMutationQueue`. The `kit` package (`extensions/sdk/kit`) is Pi's component set as a declarative
view (Container, Box, Text, Markdown, SelectList, SettingsList, DynamicBorder, Spacer, HStack/VStack, Lines).

Event names are the `sdk.Event*` constants (`sdk.EventSessionStart`, `sdk.EventBeforeAgentStart`,
`sdk.EventToolCall`, `sdk.EventToolExecutionEnd`, `sdk.EventAgentEnd`, `sdk.EventSessionCompact`,
`sdk.EventSessionTree`, `sdk.EventInput`, `sdk.EventMessageEnd`, ...). Payloads are `map[string]any`
decoded from Pi's event JSON (see Pi's `extensions/types.ts` mirrored at
`/home/user/michaelkinsy/pig/.upstream/current/packages/coding-agent/src/core/extensions/types.ts`).
Event handler results are returned as `map[string]any` (for example `{"block": true, "reason": ...}` from
`tool_call`, `{"action":"transform","text":...}` from `input`, `{"systemPrompt": ...}` / `{"message": ...}`
from `before_agent_start`).

### 3.2 Lifecycle rules (from PiG's docs; violating them breaks reload and children)

- Do not start timers, watchers, sockets or processes in the factory. Start session-scoped resources from
  `session_start`; release them from an idempotent `session_shutdown`.
- No fire-and-forget goroutines. Awaited TS work is a blocking Go call; `Promise.all` is owned, joined
  concurrency; an intentionally unawaited task needs cancellation, error reporting and shutdown draining.
  The request's cancellation is `ctx.Done()`.
- A handler returns only after its work finishes. Module-level mutable state must be guarded by a mutex and
  must be resettable (export a `ResetState()` for tests where the TS package has `__resetState`).
- Focused UI: implement `sdk.RemoteComponent` (`Render(width int) []string`,
  `HandleInput(data string) (sdk.RemoteComponentResult, error)`), optionally `SetInvalidate(func())` for
  timer-driven redraws and `Dispose()`. Overlays: `sdk.RemoteOverlayOptions{Overlay: true, OverlayOptions:
  &sdk.OverlayOptions{Anchor: "bottom-center", Width: sdk.OverlayPercent(100), ...}}`. Inline (editor
  slot): `Overlay: false`. In `Mode() == "rpc"` prefer the native dialogs (`Select`, `Input`, ...).
- Widgets: `SetWidget(key, lines, sdk.WidgetOptions{"placement": "aboveEditor"})`; clear with `nil` lines.
- Child sessions are NOT available from the SDK. Use `rpiv-rpc` to drive `pig --mode rpc` subprocesses.

### 3.3 Where files live

- rpiv config files keep their exact TypeScript paths and shapes (`~/.config/rpiv-<name>/<file>` honoring
  `XDG_CONFIG_HOME`, mode 0600, one-way legacy fallback) so a user can share config between Pi and PiG.
- Anything that was under Pi's agent dir (`~/.pi/agent/...`: `agents/`, `settings.json`) moves to
  `sdk.AgentDir()` (`~/.pig/agent/...`). Project-level `.pi/` becomes `.pig/` (`sdk.ConfigDirName()`).
- Run state the TS packages kept under `<cwd>/.rpiv/` (workflow runs, artifacts) stays under `<cwd>/.rpiv/`.

## 4. Testability pattern: narrow host interfaces

Because `sdk.Context` cannot be faked, every extension separates wiring from logic:

```go
// extension.go — the only file that touches sdk.Context directly
func Extension() *sdk.Extension {
    ext := sdk.New("rpiv-todo")
    ext.Tool(todo.ToolName, todo.Description, todo.Schema(), func(ctx sdk.Context, params map[string]any) (any, error) {
        return todo.Execute(host.FromContext(ctx), params)
    })
    ...
}
```

- `rpiv-host` defines `host.Host`, an interface mirroring the `sdk.Context` methods the siblings use (exact
  same names and signatures as `sdk.Context`, so the adapter is a one-line delegation per method), plus a few
  wrapped members whose SDK form is an unfakeable struct: `EmitEvent(channel string, data any) error`,
  `SubscribeEvent(channel string, handler func(data any) error) (func(), error)`,
  `BuildSessionContext() (sdk.SessionContext, error)`, `BuildContextEntries() ([]map[string]any, error)`,
  `Theme() tui.Styler`, `AvailableModels()/AllModels() ([]map[string]any, error)`,
  `ProviderAuthStatus(provider string) (sdk.ProviderAuthStatus, error)`, `AgentDir() (string, error)`,
  `Version() (string, error)`. `host.FromContext(ctx sdk.Context) host.Host` is the adapter.
- Each package declares its OWN narrower interface (a subset of `host.Host`'s method set, same signatures)
  so its dependencies are visible, for example `type Host interface { Notify(message, level string); SetWidget(...) error; ... }`.
  Both `host.Host` values and `*testkit.Fake` satisfy it.
- `rpiv-testkit` provides `testkit.NewFake()` (`*testkit.Fake` implements all of `host.Host`, records every
  effect into capture buckets, and takes scripted answers through exported fields and func hooks) and
  `testkit.NewWireHost(t, ext, opts)` (an in-process host over `net.Pipe` speaking PiG's real wire, so the
  factory's registration and the SDK adapters are exercised for real: `InvokeTool`, `InvokeCommand`,
  `InvokeEvent`, recorded host calls and widget pushes). The plan-mode example at
  `/home/user/michaelkinsy/pig/examples/extensions/plan-mode/main_test.go` shows the wire shapes; the SDK's own
  `*_test.go` files under `/home/user/michaelkinsy/pig/extensions/sdk/` are the authoritative spec.
- Tests: port the TS test suite's CASES (every `it(...)` that tests behavior, as table-driven Go tests)
  against `*testkit.Fake`; add one `WireHost` smoke test per extension (registration shape + one tool or
  command round trip). `go test -race ./...` must pass in each module with `GOWORK=off`.

## 5. Allowed third-party Go modules

`github.com/MichaelKinsy/PiG/extensions/sdk v0.5.0`, `github.com/rivo/uniseg`, `github.com/mattn/go-runewidth`,
`gopkg.in/yaml.v3` (frontmatter), `github.com/gen2brain/malgo` and `github.com/k2-fsa/sherpa-onnx-go`
(voice only, behind the build tag described in that package's plan), `golang.org/x/text`, `golang.org/x/net`
(HTML parsing in web-tools). Anything else: ask by writing a note in your report; do not add it.

## 6. Fidelity rules

1. Mirror the TypeScript structure and identifiers 1:1 (camelCase to PascalCase for exports). One TS file
   maps to one Go file of the same base name where sensible; a TS directory maps to a Go subpackage.
2. Preserve every externally observable contract: tool names, parameter schemas (write the JSON Schema by
   hand as `sdk.Schema`), result envelopes (`{content:[{type:"text",text}], details}`), custom entry types
   (`rpiv-todo-state`, `subagents:record`, ...), event channel names (`rpiv:ask-user:answer`,
   `subagents:completed`, ...), config file names and JSON keys, persisted JSONL shapes (workflow state
   schema version 3), slash command names and flags, user-visible strings and notification texts, prompt
   text (bytes identical where cache parity matters), environment variables, exit codes.
3. Port semantics, not syntax: account for `undefined` vs `null` (use pointers / `omitempty` deliberately),
   insertion order of object keys where the TS code relies on it, UTF-16 lengths where the TS code slices
   strings for display or truncation (`sdk.TruncateLine` counts UTF-16 units), JS number formatting,
   `Array.sort` stability, regex differences (Go RE2 has no lookaround: restructure, do not approximate).
4. Errors: TS `throw` becomes a returned `error`; a tool's user-facing failure becomes the same text in the
   tool result (`sdk.NewToolError(text)` or an error result envelope exactly as the TS code shaped it).
5. Concurrency: see §3.2. Use `sync.Mutex` for state the TS code kept in module scope; keep the same reset
   hooks.
6. Where the Pi host feature does not exist in PiG's subprocess model, implement the closest faithful
   behavior and record it in the package's `doc.go` under a `Divergences` heading (for example: child
   sessions run as `pig --mode rpc` processes instead of in-process `AgentSession`s; `/rpiv-setup` reports
   missing siblings and prints how to select them instead of spawning `pi install`).
7. Comments state current behavior and non-obvious invariants, translated from the TS comments where they
   carry contract. No phase/plan/ticket codes, no narration of the porting process.
8. Go style: `gofmt`-clean, `go vet`-clean, Go 1.26 idioms (`slices`, `maps`, `strings.Cut`, `min`/`max`,
   `for i := range n`), no `panic` for expected failures, exported identifiers documented.

## 7. Child sessions, subagents and the lane relay

- `rpiv-rpc` starts `pig --mode rpc [--piglet <path>] [--model p/m] [--thinking lvl] [--no-session]
  [--session-dir d] [--tools a,b] [--system-prompt t] [--append-system-prompt t] [--no-extensions]
  [--extension path]` in a working directory, speaks JSON Lines (`{"id","type":"prompt","message":...}` in;
  responses `{"id","type":"response","command","success","data"}` and events out), and exposes typed
  commands plus an event subscription. It relays `extension_ui_request` records (select/confirm/input/
  editor dialogs from the child's extensions) to a handler and writes `extension_ui_response`. It locates
  the binary via `$PIG_BIN`, then `pig` on `PATH`, then (Linux) the parent process chain; callers may override.
  Reference: `/home/user/michaelkinsy/pig/docs/site/docs/rpc.md`, `rpc-commands.md`, `rpc-extension-ui.md`,
  and PiG's own client `/home/user/michaelkinsy/pig/coding/rpcclient/` (read-only reference).
- `rpiv-agents` registers the `Agent` tool with pi-subagents' parameter schema (`prompt`, `description`,
  `subagent_type`, optional `name`, `model`, `thinking`, `max_turns`, `run_in_background`, `resume`,
  `isolated`, `inherit_context`), loads agent profiles (`~/.pig/agent/agents/*.md`, `<cwd>/.pig/agents/*.md`,
  `<cwd>/.agents/agents/*.md`; frontmatter `name, description, tools, model, thinking, isolated,
  disallowed_tools, max_turns, prompt_mode, ...`) plus the three embedded defaults (general-purpose, Explore,
  Plan), runs each agent as a `rpiv-rpc` child, and emits `subagents:started/completed/failed` with
  pi-subagents' payload shape (`{id,type,description,status,result,error,toolUses,durationMs,tokens:{input,output,total}}`),
  appends `subagents:record`, and supports `list_subagents`, `get_subagent_result`, `steer_subagent`,
  `stop_subagent`. The pi-subagents source is at
  `/tmp/claude-0/-home-user-rpiv-mono/69046b9c-d7f1-58e6-848f-3c05629a3109/scratchpad/npmpkgs/pi-subagents/package/src/`.
- `rpiv-pi`'s SDK workflow host (`sdk-workflow-host.ts`) becomes an `rpiv-rpc`-backed executor; parked
  questions arrive as `extension_ui_request` dialogs and are relayed into the lane dock exactly where the TS
  version relayed `ask_user_question` prompts.

## 8. Deliverable checklist (per package)

1. `cd pig/extensions/<name> && GOWORK=off go build ./... && GOWORK=off go vet ./... && gofmt -l . (empty)`.
2. `GOWORK=off go test -race ./...` passes; the test count is in the same order of magnitude as the TS
   suite's and covers the same behaviors.
3. For extensions: `HOME=<scratch> <pig-bin> install ./ --validate-only --json` reports `"valid": true` with
   the expected tools/commands/handlers/flags/shortcuts (paste the JSON in your report).
4. `doc.go` documents the package, its host dependencies, its config files and any divergences.
5. `README.md` in the module root (extensions only), ≤ 120 lines, following `docs/readme-standard.md`'s
   section order (title `# rpiv-<name> (PiG)`, lede, Install (how to select it: piglet or `pig -e`),
   Quick start, What you get, Configuration, Reference, License).
6. Your final report lists: files written, TS files not ported and why, divergences recorded, test counts,
   and the validate-only JSON.
