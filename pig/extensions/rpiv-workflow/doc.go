// Package workflow is the Go port of @juicesharp/rpiv-workflow — the engine
// that chains Pi skills into typed multi-stage workflows — for PiG (Pi in Go).
//
// This root package is the VOCABULARY and the public barrel: every authoring
// name (DefineWorkflow, Produces / Acts / Terminal and their Script / Prompt
// builders, Fanin, DefineRoute / Gate / Match / SetRouteNote, Fanout /
// Iterate / Assess / Verify, NewJudge / Panel / Majority / All / Any, the
// Gt / Gte / Lt / Lte / Eq predicates, the T schema builders and
// TypeboxSchema), every host port (WorkflowHost, WorkflowLauncherContext,
// WorkflowHostContext, WorkflowSessionContext, ModelSelection), every
// runtime type (RunState, RunContext, SessionContext, StageSessionContext,
// RunWorkflowOptions / Result), the lifecycle surface (LifecycleListeners,
// RegisterLifecycle), the message vocabulary (Msg* / Err* / Fail*), the
// output envelope (Output, OutputMeta, FinalizeOutput, FailedOutput), the
// outcome contract (ArtifactCollector, ArtifactParser, Outcome), the artifact
// handles (FS / URL / Opaque / Inline), the skill-contract data model, the
// JSON Schema bridge (Schema, StageSchema, JsonSchemaToStandard, CheckSchema,
// IsSchemaCompatible) and the transcript predicates. It depends only on the
// Go standard library and imports NO subpackage: Go forbids import cycles,
// so the TypeScript layout (root files importing subdirectories and vice
// versa) becomes a DAG whose root is this package.
//
// # Go conventions used by the port
//
//   - Exported names are PascalCase mirrors of the TypeScript exports with Go
//     initialisms (runId → RunID, url → URL). SCREAMING_SNAKE constants are
//     PascalCased: STOP → Stop, STATE_SCHEMA_VERSION → StateSchemaVersion,
//     MSG_STAGE_FAILED → MsgStageFailed, FAIL_* → Fail*, ERR_* → Err*,
//     READS_DATA → ReadsData, PANEL_VERDICT → PanelVerdictSchema (the type
//     PanelVerdict keeps the TypeScript type name).
//   - A TypeScript throw is a returned error. The value-returning constructors
//     that validate at construction (Fanin, DefineRoute, Gate, Match, Fanout,
//     Iterate, Assess, Verify, Panel, NewJudge) return (value, error); Must
//     unwraps them inside a workflow literal.
//   - Optional scalars whose "unset" differs from the zero value are pointers
//     (Max, MaxRetries, Concurrency, InheritsArtifacts, BranchOffset, …);
//     Int / Bool build them. Strings use "" for unset where the TypeScript
//     field defaulted on falsy (Skill, Outcome.Name, OnInvalid).
//   - `string | Fn` unions are small value types: Prompt (StaticPrompt /
//     DynamicPrompt) and JudgePrompt (StaticJudgePrompt / DynamicJudgePrompt).
//   - Function objects that carried symbol-keyed properties are pointer types
//     embedding SymbolMarks: *EdgeFn (Pick, Targets, ReadsData / RouteNote
//     marks) and *FoldFn (Fold, CanonicalFold brand). ReadSymbol / MarkSymbol
//     are the one read/write pair.
//   - The TypeScript discriminated unions are single structs discriminated by
//     presence (StageDef: Run set ⇒ script, Prompt set ⇒ prompt, else skill;
//     ArtifactHandle: Kind) or interfaces over pointer arms (LoopDef over
//     *FanoutLoop / *IterateLoop / *AssessLoop; AnyJudge over *Judge /
//     *PanelJudge; EdgeTarget via To / Route).
//   - Dynamic data (`unknown`) is any. Output.Data holds either decoded JSON
//     shapes (map[string]any, []any, float64) or authored Go values; the
//     validator (CheckSchema) and the field readers (Gate / Match) normalise
//     both so authored and decoded data behave identically.
//   - AbortSignal is context.Context; `~standard.validate` is
//     StageSchema.Validate(ctx, value) (StandardResult, error).
//   - Process-global `Symbol.for` slots are GlobalSlot getters over a
//     mutex-guarded table with ResetGlobalSlot / ResetGlobalSlots for tests;
//     the lifecycle registry exposes ResetLifecycleRegistry.
//   - Persisted JSON shapes are byte-identical to the TypeScript rows: struct
//     field order mirrors the TypeScript insertion order, optional keys use
//     omitempty where JSON.stringify dropped undefined, and ArtifactHandle
//     marshals only its kind's fields (inline bytes as the index-keyed object
//     JSON.stringify emits for a Uint8Array).
//
// # TypeScript → Go file map (root package)
//
//	types.ts                   → types.go           RunState, RunTermination, FailureMemo, RunWorkflowOptions/Result,
//	                                                RunContext, SessionContext, UnitRef, StageSessionContext
//	stage-def.ts               → stage-def.go       StageKind/ProgressValue/SessionPolicy/OnInvalid vocab, ScriptContext,
//	                                                Prompt, StageRead/Fanin/ReadName/ReadsAll, StageDef, Workflow, factories
//	stage-identity.ts          → stage-identity.go  ResolvePublishName, ActsPublishName, ResolveSkill, EffectiveOutputSchemaOf,
//	                                                IsDispatchingStage
//	stage-errors.ts            → stage-errors.go    StagePreflightError, HaltPreflight, InvariantPreflight(+Split)
//	loop-def.ts                → loop-def.go        Unit, Fanout/Iterate/FeedForward contexts, JudgedRepetition, VerifySpec,
//	                                                UnitRole, CapPolicy, ResultProjection, UnitSelector, LoopDef + arms, LoopKinds
//	loop-constructors.ts       → (barrel; no file)  its five loops/* modules are root files:
//	loops/constructors.ts      → loops-constructors.go   DefaultAssessMax, LoopDefaults, VerifyLoopDefaults, Fanout, Iterate,
//	                                                     Assess, JudgeSlotShapeIssues, AssessShapeIssues
//	loops/derivations.ts       → loops-derivations.go    FreezesEntryArgsOf, EffectiveLoopOf, JudgeSlotOf, ForEachJudgeChannel
//	loops/introspection.ts     → loops-introspection.go  StageShape, DescribeFlow, JudgeSpec/PanelJudgeSpec/AnyJudgeSpec,
//	                                                     JudgeSpecOf, PanelSpecOf, JudgeSlotSpecOf, LoopSpec, LoopSpecOf
//	loops/panel.ts             → loops-panel.go          PanelVerdictSchema, PanelVerdict, PanelVerdictOutcome, Majority/All/Any,
//	                                                     PanelShapeIssues, PanelSpec, Panel, PanelVerdictChannel, PanelVerdictDef
//	loops/verify.ts            → loops-verify.go         Verify, VerifyShapeIssues, SynthesizeVerifyLoop
//	routing-dsl.ts             → routing-dsl.go     EdgeContext, EdgePredicate, EdgeFn, Stop, EdgeTarget (To/Route), ReadsData,
//	                                                MarksReadsData, DefineRoute, RouteNote, TakeRouteNote, SetRouteNote, Gate, Match
//	predicates.ts              → predicates.go      NumericPredicate, Predicate, Gt, Gte, Lt, Lte, Eq
//	judge.ts                   → judge.go           JudgeContext, JudgePrompt, NamedOutcome, Judge, FoldFn, PanelJudge, AnyJudge,
//	                                                IsPanel, PanelMembers, CanonicalFold brand helpers, JudgeShapeIssues,
//	                                                AssertShape, NewJudge (judge()), ResolveJudgePrompt
//	output-spec.ts             → output-spec.go     SnapshotContext, CollectContext, CollectResult (CollectOK/CollectFatal),
//	                                                ArtifactCollector, SnapshotCollector, ParseContext, ParseResult,
//	                                                ArtifactParser, Outcome, OutputSpec, DefineCollector, DefineParser
//	output.ts                  → output.go          OutputMeta, Output, Verdict, RunView, FinalizeOutput, NewOutputMeta
//	                                                (outputMeta()), FailedOutputKind, FailedOutput, IsFailedOutput
//	host.ts                    → host.go            NotifyLevel, CommandInfo, WorkflowHost, HostUI, HostSessionManager,
//	                                                WorkflowLauncherContext, ModelSelection, SpawnOptions, WorkflowHostContext,
//	                                                WorkflowSessionContext
//	messages.ts                → messages.go        every Msg*/Err*/Fail* constant and function, FailureText, BackwardJumpHaltInfo,
//	                                                IsExhaustionHalt, FatalFailureArgs, FailedArgs(Of), AbortedArgs(Of)
//	layers.ts                  → layers.go          ConfigLayer, RenderConfigLayer
//	internal-utils.ts          → internal-utils.go  AssertNever, InvalidError (throwInvalid), RequireNonEmptyString, FormatError,
//	                                                NowIso, WorkflowAbortError, IsAbortError, WorkflowConfigError, Symbol/SymbolFor,
//	                                                GlobalSlot + resets, SymbolMarks/ReadSymbol/MarkSymbol, LazyProviderRegistry,
//	                                                WithTimeout, ResolveUnderCwd, DeepEqual
//	(JS value semantics)       → js-values.go       unexported Number()/String()/JSON.stringify/typeof/=== mirrors
//	validation-bounds.ts       → validation-bounds.go  the six bounds + ClampRange
//	json-schema.ts             → json-schema.go     Schema, JsonSchemaObject, IsJsonSchemaObject, StandardIssue/Result, StageSchema,
//	                                                JSONSchemaConverter/Options/Target, JsonSchemaCapable, JsonSchemaConverter,
//	                                                ValidateToJsonSchemaIssues, JsonSchemaToStandard, HasJsonSchema, ExtractJsonSchema
//	(TypeBox Value.Check)      → schema-check.go    SchemaError, SchemaDefectError, CheckSchema — the validator behind every
//	                                                StageSchema; schema-guards.go carries TypeBox's own length / multipleOf /
//	                                                format guards
//	schema-compat.ts           → schema-compat.go   SchemaCompatResult, IsSchemaCompatible
//	typebox-adapter.ts         → typebox-adapter.go T (SchemaBuilder: Object/Optional/String/Number/Integer/Boolean/Null/Array/
//	                                                Literal/Enum/Union/Record/Any), TypeboxSchema
//	skill-contract.ts          → skill-contract.go  ContractSource, ConsumesReadSpec, ConsumesSpec, ProducesSpec, SkillContract,
//	                                                SkillContractMap, CompositionComparator
//	triggers.ts                → triggers.go        RunTrigger, DefaultTrigger
//	handle.ts                  → handle.go          ArtifactHandle, Artifact, FS/URL/Opaque/Inline, HandleToString (+ JSON codec)
//	transcript.ts              → transcript.go      StopReason, StopSignal, ClassifyStop, BranchContentPart, BranchEntry, ToolUse,
//	                                                SessionRef (from state/state.ts), ReadBranch, ReadSessionRef,
//	                                                HasAssistantMessage, LastAssistantStopReason, LastAssistantText,
//	                                                MatchMode (MatchGlobal / MatchFirst), MatchText, LastMatchInBranch,
//	                                                IterToolUses
//	events.ts                  → events.go          LifecycleContext, StageRef, LoopStartInfo, UnitEvent, LoopCapInfo,
//	                                                LifecycleListeners, LifecycleDispatcher, RegisterLifecycle,
//	                                                ResetLifecycleRegistry, LifecycleCtxFor, LifecycleCtxFromSession, stage refs
//	outcomes/side-effect.ts    → side-effect.go     NoopCollector, SideEffectOutcome (the panel vocabulary needs them)
//	state/names.ts (partial)   → names.go           MaxNameLength, ValidName, IsValidName (the message vocabulary needs them)
//	api.ts                     → api.go             the barrel is the package; Bool / Int / String / Must helpers
//
// Files that moved INTO the root because types.ts / messages.ts / loops/panel.ts
// reference them and a Go root cannot import a subpackage: events.ts,
// transcript.ts (with SessionRef), outcomes/side-effect.ts, and the name
// policy constants of state/names.ts. The owning packages below re-export
// them (type aliases / vars) so their public surface stays complete.
//
// # Package DAG for the remaining stages
//
// Each package lives under this module and may import the root and any
// package listed BEFORE it; nothing imports a package listed after it. A
// stage may add packages but must not change existing exported API without
// recording the change under "API changes" below.
//
//	workflow (root)      — this package. Imports: stdlib only.
//	validate             — validate/issue.ts, validate/graph.ts, validate/stage-rules.ts, validate/contract-compat.ts,
//	                       validate-workflow.ts (the orchestrator), validate-output.ts (SchemaValidationFailure,
//	                       ValidationResult, SchemaTimeoutError, RunValidationRetryLoop, ValidateOutputData).
//	                       Imports: root, skillcontracts (for AdjudicateChannel / comparators).
//	skillcontracts       — skill-contracts/registry.ts, composition.ts, harvest.ts, extension-points.ts, index.ts
//	                       (registerSkillContracts(Provider), registerCompositionComparator, registerOutcomeDeriver,
//	                       registerBucketKindMapping, canCompose, legalNextSkills, buildEffectiveContracts, resets).
//	                       Imports: root. (Listed after validate above only for reading order: validate imports
//	                       skillcontracts, so skillcontracts is BELOW validate in the DAG.)
//	state                — state/state.ts (row types WorkflowStage, WorkflowHeader, RunSummary, RunRecap, RoutingDecision,
//	                       LoopCapRow, StageStatus, StateSchemaVersion = 3; SessionRef is an alias of the root type),
//	                       state/paths.ts, raw.ts, writes.ts, reads.ts, resolve.ts, names.ts (claimName etc.; MaxNameLength /
//	                       ValidName alias the root). Imports: root.
//	outcomes             — outcomes/index.ts, exec.ts, git-commit.ts, side-effect.ts (re-exports root NoopCollector /
//	                       SideEffectOutcome), collectors/* (text-scan, transcript-path, url, directory-path, tool-call,
//	                       workspace-diff, union, require-opt), parsers/* (json-body). Imports: root.
//	audit                — audit.ts, audit-ctx.ts, audit-rows.ts, chain-state.ts, failure-memos.ts, worktree-digest.ts,
//	                       routing.ts (edge execution), built-ins.ts (registerBuiltIns(Provider), getBuiltIns, flush/drain,
//	                       reset), semaphore.ts, death-scene.ts (needs state + transcript readers), execution-host.ts
//	                       (the executor-provider seam: registerWorkflowExecutionHost, getWorkflowExecutionProvider, reset).
//	                       Imports: root, state, outcomes.
//	loops                — loop-kinds.ts (LoopStrategies, LoopDeps, buildLoopEntry, freshCursor, sequential strategies,
//	                       foldFanoutCompletion), loop-waves.ts (Kahn waves, ensureUnitDeps). Imports: root, audit, state.
//	sessions             — sessions/sessions.ts, spawn.ts, extraction.ts, halt-routing.ts, success-persist.ts,
//	                       bash-strikes.ts (ResetStrikeBudgets), locate.ts, reattach.ts, index.ts.
//	                       Imports: root, validate, audit, state, outcomes.
//	runner               — runner/* (runner.ts, run-stage.ts, chain-advance.ts, resolve-stage.ts, preflight.ts,
//	                       input-validation.ts, script-stage.ts, failure.ts, run-context.ts, errors.ts, resume*.ts,
//	                       by-name.ts, by-run-id.ts, index.ts) plus the root drivers loop.ts, loop-parallel.ts and the
//	                       budget constants MAX_BACKWARD_JUMPS / MAX_LAPS / MAX_ITERATIONS + validateRunBudgets.
//	                       Imports: root, validate, skillcontracts, state, outcomes, audit, loops, sessions.
//	load                 — load/* (issues.ts, paths.ts, legacy.ts, skill-contract-phase.ts, shape-guards.ts, normalize.ts,
//	                       cache.ts, merge.ts, resolve-default.ts, alias.ts, index.ts). The jiti overlay loader has no Go
//	                       equivalent; the load stage decides the overlay mechanism (a Go-authored registry and/or a
//	                       declarative file format) and records it as a divergence. Imports: root, validate,
//	                       skillcontracts, audit (built-ins), rpiv-config.
//	command              — command.ts (flag grammar), command-run.ts, preview.ts, docs-protocol.ts.
//	                       Imports: root, load, runner, state, audit.
//	extension            — extension.ts / registration.ts / startup.ts: the PiG factory (Extension() *sdk.Extension),
//	                       the `/wf` registration over host.Host. The ONLY package importing the PiG SDK.
//	                       Imports: root, command, rpiv-host.
//
// Ownership is exclusive: a TypeScript file named under one package above is
// ported by that stage only. internal.ts (the test-only surface) has no Go
// file: each package exports its own Reset* helpers directly.
//
// # API changes
//
// (none yet — later stages append here.)
//
// # Divergences
//
//   - Construction-time throws are returned errors; Must is provided for
//     literal authoring. Cases that only a dynamically typed input could
//     reach (a non-integer max such as 1.5, a non-string flag) are
//     unrepresentable through the typed constructors; the map-literal arms of
//     the *ShapeIssues rule sources reproduce them for hand-rolled values.
//   - TypeBox's Value.Check / Value.Errors are replaced by CheckSchema, which
//     reproduces typebox@1.1.39's keyword order, en_US messages, instance
//     paths and schema paths (testdata/typebox-fixture.json, generated from
//     the real library, is replayed by the tests). Not ported: $ref / $defs
//     resolution, $recursiveRef / $dynamicRef, additionalItems,
//     unevaluatedItems / unevaluatedProperties, and the formats hostname,
//     idn-email, idn-hostname, iri, iri-reference, uri-reference,
//     uri-template, url, duration, json-pointer, json-pointer-uri-fragment
//     and relative-json-pointer — a schema using one of those is accepted
//     where TypeBox would enforce it. Patterns are RE2: a lookaround or
//     backreference TypeBox accepts is a SchemaDefectError here (the port of
//     TypeBox throwing on a pattern that does not compile), and format
//     "regex" judges RE2 syntax. Go maps are unordered, so properties,
//     pattern keys and object keys are walked in sorted order where TypeBox
//     walks insertion order: the set of errors is identical, their order can
//     differ for sibling failures.
//   - T.Object lists `required` in sorted property order (Go maps are
//     unordered); pass Schema{"required": …} in the options to pin TypeBox's
//     insertion order where the schema is compared as data. T.Literal(nil)
//     emits only `const` (Type.Literal does not accept null).
//   - IsSchemaCompatible walks consumer properties in sorted key order (Go
//     maps are unordered); which of several conflicts is reported first can
//     differ from the TypeScript insertion-order walk. DescribeFlow likewise
//     returns stages in sorted name order.
//   - The optional watchdog methods toolTimeout? / resetToolTimeout? are
//     required members of WorkflowSessionContext; a host without a watchdog
//     returns ("", false) and treats the reset as a no-op.
//   - WithTimeout cancels the context it hands fn when the deadline passes,
//     where the TypeScript promise race could only abandon the promise; what
//     fn returns after the caller has left is discarded.
//   - LastMatchInBranch and MatchText take a MatchMode because Go's regexp
//     has no `g` flag: MatchGlobal is text.match(/re/g) (every occurrence,
//     the last wins) and MatchFirst is text.match(/re/) (the first
//     occurrence, the last capture group). A MatchFirst pattern whose last
//     capture group did not participate reads as no match for that text,
//     where the TypeScript scan returned undefined and stopped.
//   - LazyProviderRegistry runs pending providers sequentially in
//     registration order (the TypeScript Promise.all interleaving is not
//     reproduced); the observable contract — run at most once, later
//     generations after prior flushes, error isolation with onError, an
//     empty flush returning the previous result — is preserved.
//   - RunView is an interface over *RunState; Go has no deep-readonly, so
//     the Named() map can still be mutated by a misbehaving user function.
package workflow
