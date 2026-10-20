package workflow

// Outcome authoring surface — the contract a stage's data-channel
// implementation satisfies, decomposed into two orthogonal halves:
//   - ArtifactCollector — ENUMERATE: which artifacts did the stage produce?
//   - ArtifactParser    — INTERPRET: given the artifacts, what typed data
//     does downstream see?
//
// Outcome is the wired-up pair a stage declares via StageDef.Outcome. When
// Parser is nil the output data IS the artifact list (kind "artifacts").

// SnapshotContext is the pre-stage reference capture context shared by
// collector and parser.
type SnapshotContext struct {
	Cwd        string
	RunID      string
	StageIndex int
	State      RunView
}

// CollectContext is handed to a collector's Collect. It carries the full
// unsliced branch plus a policy-derived BranchOffset — for continue-policy
// stages the offset lets the collector ignore the prior-stage prefix. Snapshot
// is whatever the collector's optional Snapshot hook returned. Skill is filled
// by the runner; collectors MUST NOT set it. UnitLabel is present iff this
// session IS one loop unit (the unit's display label) — collectors may narrow
// on it but must degrade, never fail, when it is absent.
type CollectContext struct {
	SnapshotContext
	Branch       []BranchEntry
	BranchOffset *int
	Snapshot     any
	Skill        string
	UnitLabel    string
}

// CollectResult is the three-way return from Collect:
//   - Fatal == nil and empty Artifacts — the stage produced nothing (honest
//     empty; the runner's completion contract decides whether that halts).
//   - Fatal == nil and N ≥ 1 Artifacts — the parser (or default) shapes the data.
//   - Fatal != nil — the collector cannot satisfy its contract; the runner
//     halts with the carried message.
//
// THE "NOTHING FOUND" CONVENTION: genuinely empty → ok + []; the environment
// broke MID-STAGE (channel worked at snapshot time, fails after) → fatal with
// the real cause; the channel was absent from the START → the collector's
// documented no-signal shape (ok + [] for diff collectors).
type CollectResult struct {
	Artifacts []Artifact
	Fatal     *FatalResult
}

// FatalResult carries a collector's or parser's fatal message.
type FatalResult struct {
	Message string
}

// CollectOK builds the ok arm of a CollectResult.
func CollectOK(artifacts []Artifact) CollectResult {
	if artifacts == nil {
		artifacts = []Artifact{}
	}
	return CollectResult{Artifacts: artifacts}
}

// CollectFatal builds the fatal arm of a CollectResult.
func CollectFatal(message string) CollectResult {
	return CollectResult{Fatal: &FatalResult{Message: message}}
}

// ArtifactCollector is the user-supplyable primitive: it enumerates
// artifacts. Snapshot is optional (nil = no pre-stage capture); Collect is the
// post-stage enumeration. Returned errors are the port of a thrown
// collector — the runner attributes them via ErrCollectorThrew.
type ArtifactCollector interface {
	Collect(ctx CollectContext) (CollectResult, error)
}

// SnapshotCollector is the optional pre-stage capture half of a collector.
// A collector that implements it has its Snapshot called BEFORE the stage
// body; the value flows into CollectContext.Snapshot.
type SnapshotCollector interface {
	Snapshot(ctx SnapshotContext) (any, error)
}

// ParseContext extends CollectContext with the artifacts the matching
// collector just returned, so parsers narrow on artifacts[0].Handle.Kind and
// inspect any Meta the collector attached.
type ParseContext struct {
	CollectContext
	Artifacts []Artifact
}

// ParsePayload is the typed data channel a parser shaped.
type ParsePayload struct {
	Kind string
	Data any
}

// ParseResult is the two-way return from Parse: an ok payload, or a fatal
// message that halts the stage — same posture as CollectResult.
type ParseResult struct {
	Payload *ParsePayload
	Fatal   *FatalResult
}

// ParseOK builds the ok arm of a ParseResult.
func ParseOK(kind string, data any) ParseResult {
	return ParseResult{Payload: &ParsePayload{Kind: kind, Data: data}}
}

// ParseFatal builds the fatal arm of a ParseResult.
func ParseFatal(message string) ParseResult {
	return ParseResult{Fatal: &FatalResult{Message: message}}
}

// ArtifactParser is the optional companion to a collector. When omitted, the
// output's data is the artifact list itself and kind is "artifacts".
type ArtifactParser interface {
	Parse(ctx ParseContext) (ParseResult, error)
}

// Outcome is a stage's collector+parser bundle. Name is the categorical name
// this outcome publishes under in state.named: when set, the runner uses it as
// the default publish name for any stage wired with this outcome. Resolution
// at write time: outcome.name ?? stage record key.
type Outcome struct {
	Name      string
	Collector ArtifactCollector
	Parser    ArtifactParser
}

// OutputSpec is the deprecated pre-rename alias of Outcome. Ships for one release.
type OutputSpec = Outcome

// CollectorFunc adapts a function to ArtifactCollector.
type CollectorFunc func(ctx CollectContext) (CollectResult, error)

// Collect implements ArtifactCollector.
func (f CollectorFunc) Collect(ctx CollectContext) (CollectResult, error) { return f(ctx) }

// ParserFunc adapts a function to ArtifactParser.
type ParserFunc func(ctx ParseContext) (ParseResult, error)

// Parse implements ArtifactParser.
func (f ParserFunc) Parse(ctx ParseContext) (ParseResult, error) { return f(ctx) }

// CollectorSpec is the author-facing collector literal DefineCollector
// promotes: a required Collect and an optional Snapshot.
type CollectorSpec struct {
	Snapshot func(ctx SnapshotContext) (any, error)
	Collect  func(ctx CollectContext) (CollectResult, error)
}

type specCollector struct {
	spec CollectorSpec
}

func (c *specCollector) Collect(ctx CollectContext) (CollectResult, error) {
	return c.spec.Collect(ctx)
}

type specSnapshotCollector struct {
	specCollector
}

func (c *specSnapshotCollector) Snapshot(ctx SnapshotContext) (any, error) {
	return c.spec.Snapshot(ctx)
}

// DefineCollector promotes a CollectorSpec to an ArtifactCollector; the result
// implements SnapshotCollector iff the spec carries a Snapshot.
func DefineCollector(spec CollectorSpec) ArtifactCollector {
	if spec.Snapshot != nil {
		return &specSnapshotCollector{specCollector{spec}}
	}
	return &specCollector{spec}
}

// DefineParser promotes a parse function to an ArtifactParser — the same
// identity idiom as DefineCollector.
func DefineParser(parse func(ctx ParseContext) (ParseResult, error)) ArtifactParser {
	return ParserFunc(parse)
}
