package workflow

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// Control flow as data: the Fanout / Iterate / Assess constructors, LoopSpecOf,
// DescribeFlow, Verify / SynthesizeVerifyLoop and the panel surface
// (loop-constructors.test.ts).

func skillJudge(t *testing.T, skill, outcome string) *Judge {
	t.Helper()
	return Must(NewJudge(Judge{Skill: skill, Outcome: &NamedOutcome{Name: outcome}}))
}

var noUnits = FanoutFn(func(context.Context, FanoutContext) ([]Unit, error) { return nil, nil })
var oneUnit = FanoutFn(func(context.Context, FanoutContext) ([]Unit, error) { return []Unit{{Prompt: "x", Label: "1/1"}}, nil })
var nextNil = IterateFn(func(context.Context, IterateContext) (*Unit, error) { return nil, nil })
var alwaysDone = DoneFn(func(Output) bool { return true })
var again = FeedForwardFn(func(context.Context, FeedForwardContext) (string, error) { return "again", nil })

func TestDescribeFlow(t *testing.T) {
	fanoutLoop := Must(Fanout(FanoutOptions{LoopOptionsBase: LoopOptionsBase{Source: "plans", Unit: &UnitSelector{By: "frontmatter-array", Pattern: "phases"}, Max: Int(32)}, Units: oneUnit}))
	iterateLoop := Must(Iterate(IterateOptions{LoopOptionsBase: LoopOptionsBase{Source: "architecture-reviews", Unit: &UnitSelector{By: "markdown-heading", Pattern: "### Phase {n}"}, Max: Int(8)}, Next: nextNil}))
	assessLoop := Must(Assess(AssessOptions{LoopOptionsBase: LoopOptionsBase{Max: Int(5)}, Judge: skillJudge(t, "grade-breakdown", "verdict"), Done: alwaysDone, FeedForward: again}))
	wf := DefineWorkflow(Workflow{
		Name: "t", Description: "d", Start: "research",
		Stages: map[string]StageDef{
			"research":    Produces(),
			"implement":   Acts(StageDef{Loop: fanoutLoop, Reads: []StageRead{Read("plans")}}),
			"blueprint":   Produces(StageDef{Loop: iterateLoop, Outcome: &Outcome{Name: "plans"}}),
			"breakdown":   Produces(StageDef{Loop: assessLoop, Outcome: &Outcome{Name: "tasks"}}),
			"code-review": Produces(),
			"commit":      Acts(),
		},
		Edges: map[string]EdgeTarget{
			"research": To("implement"), "implement": To("blueprint"), "blueprint": To("breakdown"), "breakdown": To("code-review"),
			"code-review": Route(Must(Gate("blockers_count", []GateBranch{{"blueprint", Gt(0)}, {"commit", Eq(0)}}, "commit"))),
			"commit":      To(Stop),
		},
	})
	byStage := map[string]StageShape{}
	for _, s := range DescribeFlow(wf) {
		byStage[s.Stage] = s
	}
	t.Run("control mode per stage", func(t *testing.T) {
		if byStage["research"].Control.Mode != ControlSingle {
			t.Fatal("research")
		}
		if c := byStage["implement"].Control; c.Mode != "fanout" || !reflect.DeepEqual(c.Spec, LoopSpecOf(fanoutLoop)) {
			t.Fatalf("%+v", c)
		}
		if c := byStage["blueprint"].Control; c.Mode != "iterate" || !reflect.DeepEqual(c.Spec, LoopSpecOf(iterateLoop)) {
			t.Fatalf("%+v", c)
		}
		if c := byStage["breakdown"].Control; c.Mode != "assess" || !reflect.DeepEqual(c.Spec, LoopSpecOf(assessLoop)) {
			t.Fatalf("%+v", c)
		}
	})
	t.Run("edge shape: linear, route, terminal", func(t *testing.T) {
		if e := byStage["research"].Edge; e.Mode != EdgeLinear || strings.Join(e.Targets, ",") != "implement" {
			t.Fatalf("%+v", e)
		}
		if e := byStage["code-review"].Edge; e.Mode != EdgeRoute || strings.Join(e.Targets, ",") != "blueprint,commit" {
			t.Fatalf("%+v", e)
		}
		if e := byStage["commit"].Edge; e.Mode != EdgeTerminal || e.Targets != nil {
			t.Fatalf("%+v", e)
		}
	})
	t.Run("reads facet: bare → all:false, fanin → all:true, declared order; omitted when none", func(t *testing.T) {
		w := Workflow{Name: "fanin-flow", Start: "plan", Stages: map[string]StageDef{
			"plan":       Produces(StageDef{Loop: Must(Fanout(FanoutOptions{Units: oneUnit})), Outcome: &Outcome{Name: "plans"}}),
			"rubric":     Produces(StageDef{Outcome: &Outcome{Name: "rubric"}}),
			"synthesize": Produces(StageDef{Reads: []StageRead{Must(Fanin("plans")), Read("rubric")}}),
		}, Edges: map[string]EdgeTarget{"plan": To("rubric"), "rubric": To("synthesize"), "synthesize": To(Stop)}}
		by := map[string]StageShape{}
		for _, s := range DescribeFlow(w) {
			by[s.Stage] = s
		}
		if !reflect.DeepEqual(by["synthesize"].Reads, []StageReadSpec{{"plans", true}, {"rubric", false}}) {
			t.Fatalf("%+v", by["synthesize"].Reads)
		}
		if by["plan"].Reads != nil || by["rubric"].Reads != nil {
			t.Fatal("reads facet should be omitted")
		}
	})
}

func TestLoopConstructors(t *testing.T) {
	t.Run("fanout fills defaults (halt / entry / no max)", func(t *testing.T) {
		l := Must(Fanout(FanoutOptions{Units: oneUnit}))
		if l.Kind() != LoopFanout || l.OnCap != CapHalt || l.Result != ResultEntry || l.Max != nil {
			t.Fatalf("%+v", l)
		}
	})
	t.Run("iterate fills defaults (halt / last)", func(t *testing.T) {
		l := Must(Iterate(IterateOptions{Next: nextNil}))
		if l.Kind() != LoopIterate || l.OnCap != CapHalt || l.Result != ResultLast {
			t.Fatalf("%+v", l)
		}
	})
	t.Run("assess fills defaults (advance / last / max 8)", func(t *testing.T) {
		l := Must(Assess(AssessOptions{Judge: skillJudge(t, "grade", "verdict"), Done: func(v Output) bool { d, _ := fieldOf(v.Data, "done"); return d == true }, FeedForward: again}))
		if l.Kind() != LoopAssess || l.OnCap != CapAdvance || l.Result != ResultLast || l.RoundCap() != DefaultAssessMax || DefaultAssessMax != 8 {
			t.Fatalf("%+v", l)
		}
	})
	t.Run("honours explicit onCap / result / max", func(t *testing.T) {
		l := Must(Fanout(FanoutOptions{LoopOptionsBase: LoopOptionsBase{OnCap: CapAdvance, Result: ResultLast, Max: Int(4)}, Units: noUnits}))
		if l.OnCap != CapAdvance || l.Result != ResultLast || *l.Max != 4 {
			t.Fatalf("%+v", l)
		}
	})
	t.Run("fanout optional knobs carried and omitted by default", func(t *testing.T) {
		if *Must(Fanout(FanoutOptions{Units: noUnits, Concurrency: Int(1)})).Concurrency != 1 || *Must(Fanout(FanoutOptions{Units: noUnits, Concurrency: Int(3)})).Concurrency != 3 {
			t.Fatal("concurrency")
		}
		plain := Must(Fanout(FanoutOptions{Units: noUnits}))
		if plain.Concurrency != nil || plain.DepArtifactFlag != "" || plain.HaltWhenAllFailed != nil || plain.RetryHaltedUnits != nil || plain.FailFast != nil {
			t.Fatalf("%+v", plain)
		}
		flagged := Must(Fanout(FanoutOptions{Units: noUnits, DepArtifactFlag: strPtr("--upstream"), HaltWhenAllFailed: Bool(true), RetryHaltedUnits: Int(2), FailFast: Bool(true)}))
		if flagged.DepArtifactFlag != "--upstream" || !*flagged.HaltWhenAllFailed || *flagged.RetryHaltedUnits != 2 || !*flagged.FailFast {
			t.Fatalf("%+v", flagged)
		}
	})
	refusals := []struct {
		name    string
		err     error
		pattern string
	}{
		{"concurrency 0", errOf(Fanout(FanoutOptions{Units: noUnits, Concurrency: Int(0)})), "concurrency must be an integer >= 1 \\(got 0\\)"},
		{"concurrency -2", errOf(Fanout(FanoutOptions{Units: noUnits, Concurrency: Int(-2)})), "concurrency must be an integer >= 1"},
		{"retryHaltedUnits 0", errOf(Fanout(FanoutOptions{Units: noUnits, RetryHaltedUnits: Int(0)})), "retryHaltedUnits must be an integer >= 1"},
		{"retryHaltedUnits -2", errOf(Fanout(FanoutOptions{Units: noUnits, RetryHaltedUnits: Int(-2)})), "retryHaltedUnits must be an integer >= 1"},
		{"fanout max 0", errOf(Fanout(FanoutOptions{LoopOptionsBase: LoopOptionsBase{Max: Int(0)}, Units: noUnits})), "fanout\\(\\): max must be an integer >= 1"},
		{"iterate max -1", errOf(Iterate(IterateOptions{LoopOptionsBase: LoopOptionsBase{Max: Int(-1)}, Next: nextNil})), "iterate\\(\\): max must be an integer >= 1"},
		{"assess max -2", errOf(Assess(AssessOptions{LoopOptionsBase: LoopOptionsBase{Max: Int(-2)}, Judge: skillJudge(t, "grade", "verdict"), Done: alwaysDone, FeedForward: again})), "assess\\(\\): max must be an integer >= 1"},
		{"depArtifactFlag empty", errOf(Fanout(FanoutOptions{Units: noUnits, DepArtifactFlag: strPtr("")})), "depArtifactFlag must be a non-empty string \\(got \"\"\\)"},
		{"depArtifactFlag whitespace", errOf(Fanout(FanoutOptions{Units: noUnits, DepArtifactFlag: strPtr("   ")})), "depArtifactFlag must be a non-empty string"},
		{"depArtifactFlag tab/newline", errOf(Fanout(FanoutOptions{Units: noUnits, DepArtifactFlag: strPtr("\t\n")})), "depArtifactFlag must be a non-empty string"},
		{"assess done missing", errOf(Assess(AssessOptions{Judge: skillJudge(t, "grade", "verdict"), FeedForward: again})), "`done` must be a function"},
		{"assess feedForward missing", errOf(Assess(AssessOptions{Judge: skillJudge(t, "grade", "verdict"), Done: alwaysDone})), "`feedForward` must be a function"},
		{"assess invalid judge", errOf(Assess(AssessOptions{Judge: &Judge{Outcome: verdictOutcome}, Done: alwaysDone, FeedForward: again})), "assess\\(\\):.*one is required to dispatch"},
		{"assess nil judge", errOf(Assess(AssessOptions{Done: alwaysDone, FeedForward: again})), "assess\\(\\): a judge object is required"},
	}
	for _, c := range refusals {
		t.Run(c.name, func(t *testing.T) { mustMatch(t, c.err, c.pattern) })
	}
}

func strPtr(s string) *string { return &s }

func errOf[T any](_ T, err error) error { return err }

func TestLoopSpecOf(t *testing.T) {
	if LoopSpecOf(nil) != nil {
		t.Fatal("non-loop stage")
	}
	t.Run("fanout without judge", func(t *testing.T) {
		l := Must(Fanout(FanoutOptions{LoopOptionsBase: LoopOptionsBase{Source: "plans", Unit: &UnitSelector{By: "frontmatter-array", Pattern: "phases"}, Max: Int(32)}, Units: noUnits}))
		want := &LoopSpec{Kind: LoopFanout, Source: "plans", Unit: &UnitSelector{By: "frontmatter-array", Pattern: "phases"}, Max: Int(32), OnCap: CapHalt, Result: ResultEntry}
		if !reflect.DeepEqual(LoopSpecOf(l), want) {
			t.Fatalf("%+v", LoopSpecOf(l))
		}
	})
	t.Run("iterate has no judge", func(t *testing.T) {
		spec := LoopSpecOf(Must(Iterate(IterateOptions{Next: nextNil})))
		if spec.Kind != LoopIterate || spec.Judge != nil {
			t.Fatalf("%+v", spec)
		}
	})
	t.Run("assess summarises the judge (prompt: bool)", func(t *testing.T) {
		skill := Must(Assess(AssessOptions{Judge: skillJudge(t, "grade-breakdown", "verdict"), Done: alwaysDone, FeedForward: again}))
		if !reflect.DeepEqual(LoopSpecOf(skill).Judge, JudgeSpec{Skill: "grade-breakdown", Prompt: false, Outcome: "verdict"}) {
			t.Fatalf("%+v", LoopSpecOf(skill).Judge)
		}
		prompt := Must(Assess(AssessOptions{Judge: Must(NewJudge(Judge{Prompt: StaticJudgePrompt("grade this"), Outcome: verdictOutcome})), Done: alwaysDone, FeedForward: again}))
		if !reflect.DeepEqual(LoopSpecOf(prompt).Judge, JudgeSpec{Prompt: true, Outcome: "verdict"}) {
			t.Fatalf("%+v", LoopSpecOf(prompt).Judge)
		}
		j := skillJudge(t, "grade", "verdict")
		l := Must(Assess(AssessOptions{Judge: j, Done: alwaysDone, FeedForward: again}))
		if !reflect.DeepEqual(LoopSpecOf(l).Judge, JudgeSpecOf(j)) {
			t.Fatal("drift between LoopSpecOf and JudgeSpecOf")
		}
	})
}

func TestVerifyConstructor(t *testing.T) {
	pass := DoneFn(func(v Output) bool { ok, _ := fieldOf(v.Data, "ok"); return ok == true })
	grade := func() *Judge { return skillJudge(t, "grade", "verdict") }
	t.Run("gate-only verify", func(t *testing.T) {
		v := Must(Verify(VerifySpec{Judge: grade(), Done: pass}))
		if v.Done == nil || v.Max != nil {
			t.Fatalf("%+v", v)
		}
	})
	t.Run("retrying verify", func(t *testing.T) {
		v := Must(Verify(VerifySpec{Judge: grade(), Done: pass, FeedForward: again, Max: Int(3)}))
		if v.FeedForward == nil || *v.Max != 3 {
			t.Fatalf("%+v", v)
		}
	})
	refusals := []struct {
		name    string
		spec    VerifySpec
		pattern string
	}{
		{"judge sets both skill and prompt", VerifySpec{Judge: &Judge{Skill: "grade", Prompt: StaticJudgePrompt("rate it"), Outcome: verdictOutcome}, Done: pass}, "skill XOR prompt"},
		{"judge outcome without a name", VerifySpec{Judge: &Judge{Skill: "grade", Outcome: &Outcome{Collector: NoopCollector}}, Done: pass}, "judge\\.outcome must carry a `name`"},
		{"done missing", VerifySpec{Judge: grade()}, "`done` to be a function"},
		{"max 0", VerifySpec{Judge: grade(), Done: pass, FeedForward: again, Max: Int(0)}, "max.*must be an integer >= 1"},
		{"max -1", VerifySpec{Judge: grade(), Done: pass, FeedForward: again, Max: Int(-1)}, "max.*must be an integer >= 1"},
		{"max > 1 without feedForward", VerifySpec{Judge: grade(), Done: pass, Max: Int(2)}, "max > 1 requires `feedForward`"},
		{"nil judge", VerifySpec{Done: pass}, "a judge object is required"},
	}
	for _, c := range refusals {
		t.Run(c.name, func(t *testing.T) { mustMatch(t, errOf(Verify(c.spec)), c.pattern) })
	}
	t.Run("map literal with a non-function feedForward", func(t *testing.T) {
		issues := VerifyShapeIssues(map[string]any{"judge": map[string]any{"skill": "grade", "outcome": map[string]any{"name": "v"}}, "done": pass, "feedForward": "again", "max": 2})
		if len(issues) != 1 || issues[0] != "verify `feedForward` must be a function building the next attempt's prompt arg" {
			t.Fatalf("%q", issues)
		}
	})
}

func TestSynthesizeVerifyLoopAndEffectiveLoopOf(t *testing.T) {
	pass := DoneFn(func(v Output) bool { ok, _ := fieldOf(v.Data, "ok"); return ok == true })
	grade := func() *Judge { return skillJudge(t, "grade", "verdict") }
	t.Run("gate-only synthesis", func(t *testing.T) {
		loop := SynthesizeVerifyLoop(Must(Verify(VerifySpec{Judge: grade(), Done: pass})))
		if loop.Kind() != LoopAssess || loop.RoundCap() != 1 || loop.OnCap != CapHalt || loop.Result != ResultLast || loop.Done == nil {
			t.Fatalf("%+v", loop)
		}
		_, err := loop.FeedForward(context.Background(), FeedForwardContext{})
		mustMatch(t, err, "driver invariant violated")
	})
	t.Run("retrying synthesis keeps the author's feedForward", func(t *testing.T) {
		loop := SynthesizeVerifyLoop(Must(Verify(VerifySpec{Judge: grade(), Done: pass, FeedForward: again, Max: Int(3)})))
		got, _ := loop.FeedForward(context.Background(), FeedForwardContext{})
		if loop.RoundCap() != 3 || got != "again" {
			t.Fatalf("%+v", loop)
		}
	})
	t.Run("effectiveLoopOf: loop wins; verify synthesizes; neither → nil", func(t *testing.T) {
		v := Must(Verify(VerifySpec{Judge: grade(), Done: pass}))
		explicit := Must(Iterate(IterateOptions{Next: nextNil}))
		base := StageDef{Kind: KindProduces, SessionPolicy: SessionFresh}
		both := base
		both.Loop, both.Verify = explicit, v
		if EffectiveLoopOf(both) != LoopDef(explicit) {
			t.Fatal("loop should win")
		}
		only := base
		only.Verify = v
		if EffectiveLoopOf(only).Kind() != LoopAssess || EffectiveLoopOf(base) != nil {
			t.Fatal("verify synthesis / nil")
		}
		if FreezesEntryArgsOf(EffectiveLoopOf(only)) != true || FreezesEntryArgsOf(explicit) {
			t.Fatal("freezesEntryArgs")
		}
	})
	t.Run("describeFlow: a verify stage stays single and carries the projection", func(t *testing.T) {
		w := Workflow{Name: "gated", Start: "build", Stages: map[string]StageDef{"build": {Kind: KindProduces, SessionPolicy: SessionFresh, Outcome: &Outcome{Name: "impl"}, Verify: Must(Verify(VerifySpec{Judge: grade(), Done: pass, FeedForward: again, Max: Int(2)}))}}, Edges: map[string]EdgeTarget{"build": To(Stop)}}
		shape := DescribeFlow(w)[0]
		if shape.Control.Mode != ControlSingle || !reflect.DeepEqual(shape.Verify, &VerifySpecShape{Judge: JudgeSpec{Skill: "grade", Outcome: "verdict"}, Max: 2}) {
			t.Fatalf("%+v", shape.Verify)
		}
	})
	t.Run("judgeSlotOf / forEachJudgeChannel", func(t *testing.T) {
		a := skillJudge(t, "grade-a", "va")
		b := skillJudge(t, "grade-b", "vb")
		p := Must(Panel(PanelSpec{Members: []*Judge{a, b}, Fold: Majority(func(Output) bool { return true })}))
		def := StageDef{Verify: Must(Verify(VerifySpec{Judge: p, Done: pass}))}
		if JudgeSlotOf(def) != AnyJudge(p) {
			t.Fatal("slot")
		}
		var seen []string
		ForEachJudgeChannel(def, "build", func(channel, signing string) { seen = append(seen, channel+":"+signing) })
		if strings.Join(seen, ",") != "va:grade-a,vb:grade-b,build-panel:" {
			t.Fatalf("%v", seen)
		}
		if JudgeSlotOf(StageDef{}) != nil {
			t.Fatal("no judge")
		}
	})
}

func verdict(ok bool) Output { return Output{Data: map[string]any{"ok": ok}} }
func isOk(v Output) bool     { ok, _ := fieldOf(v.Data, "ok"); return ok == true }

func TestSugarFolds(t *testing.T) {
	fold := func(f *FoldFn, vs ...Output) PanelVerdict {
		t.Helper()
		out, err := f.Fold(vs)
		if err != nil {
			t.Fatal(err)
		}
		return out.(PanelVerdict)
	}
	if v := fold(Majority(isOk), verdict(true), verdict(true), verdict(false)); v != (PanelVerdict{Pass: true, Votes: PanelVotes{2, 1}, Agreement: 2.0 / 3, Tie: false}) {
		t.Fatalf("%+v", v)
	}
	if v := fold(Majority(isOk), verdict(true), verdict(false)); v.Pass || !v.Tie || v.Agreement != 0.5 {
		t.Fatalf("%+v", v)
	}
	if !fold(All(isOk), verdict(true), verdict(true)).Pass || fold(All(isOk), verdict(true), verdict(false)).Pass {
		t.Fatal("all")
	}
	if !fold(Any(isOk), verdict(false), verdict(true)).Pass || fold(Any(isOk), verdict(false), verdict(false)).Pass {
		t.Fatal("any")
	}
	if v := fold(Majority(isOk)); v.Agreement != 0 || v.Pass || !v.Tie {
		t.Fatalf("empty %+v", v)
	}
	raw := NewFold(func(vs []Output) (any, error) { return vs, nil })
	if !MarksCanonicalFold(Majority(isOk)) || !MarksCanonicalFold(All(isOk)) || !MarksCanonicalFold(Any(isOk)) || MarksCanonicalFold(raw) || MarksCanonicalFold(nil) {
		t.Fatal("brands")
	}
	raw2, _ := Majority(isOk).Fold(nil)
	if raw2.(PanelVerdict).Tie != true {
		t.Fatal("tie on empty")
	}
}

func TestPanel(t *testing.T) {
	memberA := skillJudge(t, "grade-a", "va")
	memberB := skillJudge(t, "grade-b", "vb")
	custom := &NamedOutcome{Name: "score"}
	rawFold := NewFold(func(vs []Output) (any, error) { return len(vs), nil })
	t.Run("accepts a canonical panel", func(t *testing.T) {
		if issues := PanelShapeIssues(&PanelJudge{Members: []*Judge{memberA, memberB}, Fold: Majority(isOk)}); len(issues) != 0 {
			t.Fatalf("%q", issues)
		}
		p := Must(Panel(PanelSpec{Members: []*Judge{memberA, memberB}, Fold: Majority(isOk)}))
		if !IsPanel(p) || len(p.Members) != 2 || p.Outcome != nil {
			t.Fatalf("%+v", p)
		}
	})
	t.Run("accepts a custom panel", func(t *testing.T) {
		if Must(Panel(PanelSpec{Members: []*Judge{memberA, memberB}, Fold: rawFold, Outcome: custom})).Outcome != custom {
			t.Fatal("outcome")
		}
	})
	refusals := []struct {
		name    string
		spec    PanelSpec
		pattern string
	}{
		{"sugar ⊕ outcome", PanelSpec{Members: []*Judge{memberA, memberB}, Fold: Majority(isOk), Outcome: custom}, "sugar ⊕ outcome"},
		{"raw ⊕ outcome", PanelSpec{Members: []*Judge{memberA, memberB}, Fold: rawFold}, "raw ⊕ outcome"},
		{"bad member shape", PanelSpec{Members: []*Judge{{Outcome: &NamedOutcome{Name: "x"}}}, Fold: Majority(isOk)}, "panel member:.*one is required"},
		{"empty members", PanelSpec{Members: nil, Fold: Majority(isOk)}, "non-empty"},
		{"missing fold", PanelSpec{Members: []*Judge{memberA}}, "panel\\.fold must be a function"},
		{"outcome without a name", PanelSpec{Members: []*Judge{memberA}, Fold: rawFold, Outcome: &NamedOutcome{}}, "panel\\.outcome must carry a `name`"},
	}
	for _, c := range refusals {
		t.Run(c.name, func(t *testing.T) { mustMatch(t, errOf(Panel(c.spec)), c.pattern) })
	}
	t.Run("rejects a nested panel member (map literal path)", func(t *testing.T) {
		inner := Must(Panel(PanelSpec{Members: []*Judge{memberA, memberB}, Fold: Majority(isOk)}))
		issues := PanelShapeIssues(map[string]any{"kind": "panel", "members": []any{inner}, "fold": Majority(isOk)})
		if len(issues) != 1 || !strings.Contains(issues[0], "may not nest") {
			t.Fatalf("%q", issues)
		}
	})
	if got := PanelShapeIssues(nil); !reflect.DeepEqual(got, []string{"a panel object is required"}) {
		t.Fatalf("%q", got)
	}
	t.Run("verdict channel + def", func(t *testing.T) {
		canonical := Must(Panel(PanelSpec{Members: []*Judge{memberA}, Fold: Majority(isOk)}))
		if PanelVerdictChannel(canonical, "review") != "review-panel" {
			t.Fatal("canonical channel")
		}
		def := PanelVerdictDef(canonical, "review")
		if def.Kind != KindProduces || def.SessionPolicy != SessionFresh || def.Outcome.Name != "review-panel" {
			t.Fatalf("%+v", def)
		}
		if res, err := def.Outcome.Collector.Collect(CollectContext{}); err != nil || res.Fatal != nil || len(res.Artifacts) != 0 {
			t.Fatalf("canonical def must carry the no-op collector: %+v %v", res, err)
		}
		if PanelVerdictOutcome.Name != "panel-verdict" {
			t.Fatal(PanelVerdictOutcome.Name)
		}
		customPanel := Must(Panel(PanelSpec{Members: []*Judge{memberA}, Fold: rawFold, Outcome: custom}))
		if PanelVerdictChannel(customPanel, "review") != "score" || PanelVerdictDef(customPanel, "review").Outcome.Name != "score" {
			t.Fatal("custom channel")
		}
	})
	t.Run("PanelVerdictSchema validates a canonical verdict", func(t *testing.T) {
		if errs := CheckSchema(PanelVerdictSchema, PanelVerdict{Pass: true, Votes: PanelVotes{2, 1}, Agreement: 2.0 / 3}); len(errs) != 0 {
			t.Fatalf("%+v", errs)
		}
		if errs := CheckSchema(PanelVerdictSchema, map[string]any{"pass": true}); len(errs) == 0 {
			t.Fatal("missing required fields accepted")
		}
	})
}

func TestPanelIntrospection(t *testing.T) {
	memberA := skillJudge(t, "grade-a", "va")
	memberB := skillJudge(t, "grade-b", "vb")
	custom := &NamedOutcome{Name: "score"}
	memberSpecs := []JudgeSpec{{Skill: "grade-a", Outcome: "va"}, {Skill: "grade-b", Outcome: "vb"}}
	canonical := Must(Panel(PanelSpec{Members: []*Judge{memberA, memberB}, Fold: Majority(isOk)}))
	if !reflect.DeepEqual(PanelSpecOf(canonical), PanelJudgeSpec{Panel: memberSpecs, Fold: "majority", Outcome: ""}) {
		t.Fatalf("%+v", PanelSpecOf(canonical))
	}
	if PanelSpecOf(Must(Panel(PanelSpec{Members: []*Judge{memberA, memberB}, Fold: All(isOk)}))).Fold != "all" || PanelSpecOf(Must(Panel(PanelSpec{Members: []*Judge{memberA, memberB}, Fold: Any(isOk)}))).Fold != "any" {
		t.Fatal("fold names")
	}
	customPanel := Must(Panel(PanelSpec{Members: []*Judge{memberA, memberB}, Fold: NewFold(func(vs []Output) (any, error) { return len(vs), nil }), Outcome: custom}))
	if !reflect.DeepEqual(PanelSpecOf(customPanel), PanelJudgeSpec{Panel: memberSpecs, Fold: "custom", Outcome: "score"}) {
		t.Fatalf("%+v", PanelSpecOf(customPanel))
	}
	if !reflect.DeepEqual(JudgeSlotSpecOf(memberA), JudgeSpecOf(memberA)) || !reflect.DeepEqual(JudgeSlotSpecOf(canonical), PanelSpecOf(canonical)) {
		t.Fatal("judgeSlotSpecOf")
	}
	loop := Must(Assess(AssessOptions{Judge: canonical, Done: alwaysDone, FeedForward: again}))
	if !reflect.DeepEqual(LoopSpecOf(loop).Judge, PanelSpecOf(canonical)) {
		t.Fatal("loopSpecOf through the slot summary")
	}
	w := Workflow{Name: "paneled", Start: "build", Stages: map[string]StageDef{"build": {Kind: KindProduces, SessionPolicy: SessionFresh, Outcome: &Outcome{Name: "impl"}, Verify: Must(Verify(VerifySpec{Judge: canonical, Done: alwaysDone}))}}, Edges: map[string]EdgeTarget{"build": To(Stop)}}
	shape := DescribeFlow(w)[0]
	if shape.Control.Mode != ControlSingle || !reflect.DeepEqual(shape.Verify, &VerifySpecShape{Judge: PanelJudgeSpec{Panel: memberSpecs, Fold: "majority"}, Max: 1}) {
		t.Fatalf("%+v", shape.Verify)
	}
	if PanelSpecOf(nil).Fold != FoldCustom || len(PanelSpecOf(nil).Panel) != 0 {
		t.Fatal("defensive nil panel")
	}
}
