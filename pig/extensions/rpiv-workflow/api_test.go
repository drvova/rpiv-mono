package workflow

import (
	"context"
	"regexp"
	"strings"
	"testing"
)

// Tests for the authoring surface — factories, route builders and the
// composition smoke, ported case-for-case from api.test.ts.

func mustMatch(t *testing.T, err error, pattern string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error matching %q, got nil", pattern)
	}
	if !regexp.MustCompile(pattern).MatchString(err.Error()) {
		t.Fatalf("error %q does not match %q", err.Error(), pattern)
	}
}

func outputWith(data any) *Output {
	return &Output{Kind: "artifact-md", Artifacts: []Artifact{}, Data: data, Meta: OutputMeta{Stage: "code-review", Skill: "code-review", StageNumber: 1}}
}

func TestDefineWorkflow(t *testing.T) {
	spec := Workflow{
		Name:   "tiny",
		Start:  "research",
		Stages: map[string]StageDef{"research": Produces(), "commit": Acts()},
		Edges:  map[string]EdgeTarget{"research": To("commit"), "commit": To(Stop)},
	}
	got := DefineWorkflow(spec)
	if got.Name != "tiny" || got.Start != "research" || len(got.Stages) != 2 {
		t.Fatalf("identity passthrough lost fields: %+v", got)
	}
	w := DefineWorkflow(Workflow{Name: "demo", Description: "for testing", Start: "a", Stages: map[string]StageDef{"a": Produces()}, Edges: map[string]EdgeTarget{"a": To(Stop)}})
	if w.Description != "for testing" {
		t.Fatalf("description not preserved: %q", w.Description)
	}
}

func TestStageFactories(t *testing.T) {
	schema := TypeboxSchema(T.Object(map[string]Schema{"severeIssueCount": T.Integer(Schema{"minimum": 0})}))
	outcome := &Outcome{Collector: DefineCollector(CollectorSpec{
		Snapshot: func(SnapshotContext) (any, error) { return "pre-state", nil },
		Collect:  func(CollectContext) (CollectResult, error) { return CollectOK(nil), nil },
	})}
	cases := []struct {
		name  string
		def   StageDef
		check func(t *testing.T, d StageDef)
	}{
		{"produces applies kind + fresh defaults", Produces(), func(t *testing.T, d StageDef) {
			if d.Kind != KindProduces || d.SessionPolicy != SessionFresh || d.Skill != "" {
				t.Fatalf("got %+v", d)
			}
		}},
		{"produces respects overrides", Produces(StageDef{SessionPolicy: SessionContinue, MaxRetries: Int(3)}), func(t *testing.T, d StageDef) {
			if d.SessionPolicy != SessionContinue || d.MaxRetries == nil || *d.MaxRetries != 3 {
				t.Fatalf("got %+v", d)
			}
			b := Produces()
			if b.SessionPolicy != SessionFresh || b.MaxRetries != nil {
				t.Fatalf("defaults mutated for other calls: %+v", b)
			}
		}},
		{"produces override.skill wins", Produces(StageDef{Skill: "code-review"}), func(t *testing.T, d StageDef) {
			if d.Skill != "code-review" {
				t.Fatalf("got %q", d.Skill)
			}
		}},
		{"produces accepts outputSchema", Produces(StageDef{OutputSchema: schema}), func(t *testing.T, d StageDef) {
			if d.OutputSchema != schema {
				t.Fatal("outputSchema not threaded")
			}
		}},
		{"acts applies kind + fresh defaults", Acts(), func(t *testing.T, d StageDef) {
			if d.Kind != KindSideEffect || d.SessionPolicy != SessionFresh || d.Skill != "" {
				t.Fatalf("got %+v", d)
			}
		}},
		{"acts attaches an Outcome", Acts(StageDef{Outcome: outcome}), func(t *testing.T, d StageDef) {
			if d.Outcome != outcome {
				t.Fatal("outcome not threaded")
			}
		}},
		{"terminal desugars to acts + inheritsArtifacts false", Terminal(), func(t *testing.T, d StageDef) {
			if d.Kind != KindSideEffect || d.SessionPolicy != SessionFresh || d.InheritsArtifacts == nil || *d.InheritsArtifacts {
				t.Fatalf("got %+v", d)
			}
		}},
		{"terminal preserves overrides", Terminal(StageDef{Skill: "cleanup", SessionPolicy: SessionContinue}), func(t *testing.T, d StageDef) {
			if d.Skill != "cleanup" || d.SessionPolicy != SessionContinue || *d.InheritsArtifacts {
				t.Fatalf("got %+v", d)
			}
		}},
		{"terminal cannot be reversed via overrides", Terminal(StageDef{InheritsArtifacts: Bool(true)}), func(t *testing.T, d StageDef) {
			if *d.InheritsArtifacts {
				t.Fatal("opt-out reversed")
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { c.check(t, c.def) })
	}
}

func TestScriptFactories(t *testing.T) {
	noopProduces := ProducesScriptFn(func(context.Context, ScriptContext) (ScriptOutput, error) {
		return ScriptOutput{Kind: "noop", Artifacts: []Artifact{}, Data: map[string]any{}}, nil
	})
	noopActs := ActsScriptFn(func(context.Context, ScriptContext) error { return nil })
	schema := TypeboxSchema(T.Object(map[string]Schema{"count": T.Integer(Schema{"minimum": 0})}))

	t.Run("produces.script returns produces + fresh + run", func(t *testing.T) {
		n := ProducesScript(ProducesScriptOptions{Run: noopProduces})
		if n.Kind != KindProduces || n.SessionPolicy != SessionFresh || n.Run == nil || n.Skill != "" || n.Outcome != nil || n.Loop != nil {
			t.Fatalf("got %+v", n)
		}
		if _, ok := n.Run.(ProducesScriptFn); !ok {
			t.Fatal("run is not the produces script fn")
		}
	})
	t.Run("produces.script threads validation knobs", func(t *testing.T) {
		n := ProducesScript(ProducesScriptOptions{Run: noopProduces, OutputSchema: schema, OnInvalid: OnInvalidHalt, MaxRetries: Int(5), ValidateTimeoutMs: Int(2000), InheritsArtifacts: Bool(false)})
		if n.OutputSchema != schema || n.OnInvalid != OnInvalidHalt || *n.MaxRetries != 5 || *n.ValidateTimeoutMs != 2000 || *n.InheritsArtifacts {
			t.Fatalf("got %+v", n)
		}
	})
	t.Run("acts.script returns side-effect + fresh + run", func(t *testing.T) {
		n := ActsScript(ActsScriptOptions{Run: noopActs})
		if n.Kind != KindSideEffect || n.SessionPolicy != SessionFresh || n.Skill != "" || n.Outcome != nil || n.Loop != nil {
			t.Fatalf("got %+v", n)
		}
		if _, ok := n.Run.(ActsScriptFn); !ok {
			t.Fatal("run is not the acts script fn")
		}
	})
	t.Run("acts.script preserves inputSchema + inheritsArtifacts", func(t *testing.T) {
		n := ActsScript(ActsScriptOptions{Run: noopActs, InputSchema: schema, InheritsArtifacts: Bool(false)})
		if n.InputSchema != schema || *n.InheritsArtifacts {
			t.Fatalf("got %+v", n)
		}
	})
	t.Run("terminal.script opt-out is structural", func(t *testing.T) {
		n := TerminalScript(ActsScriptOptions{Run: noopActs})
		if n.Kind != KindSideEffect || n.SessionPolicy != SessionFresh || *n.InheritsArtifacts || n.Run == nil {
			t.Fatalf("got %+v", n)
		}
		reversed := TerminalScript(ActsScriptOptions{Run: noopActs, InheritsArtifacts: Bool(true)})
		if *reversed.InheritsArtifacts {
			t.Fatal("opt-out reversed")
		}
	})
}

func TestGate(t *testing.T) {
	pick := Must(Gate("severeIssueCount", []GateBranch{{"revise", Gt(0)}, {"commit", Eq(0)}}, "commit"))
	pickOn := func(ctx EdgeContext) string {
		t.Helper()
		got, err := pick.Pick(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	cases := []struct {
		name string
		ctx  EdgeContext
		want string
	}{
		{"first matching branch (value > 0 → revise)", EdgeContext{Output: outputWith(map[string]any{"severeIssueCount": 3})}, "revise"},
		{"exact match (value === 0 → commit)", EdgeContext{Output: outputWith(map[string]any{"severeIssueCount": 0})}, "commit"},
		{"missing value treated as NaN → otherwise", EdgeContext{Output: outputWith(map[string]any{})}, "commit"},
		{"undefined output → otherwise", EdgeContext{Output: nil}, "commit"},
		{"non-numeric field coerces to NaN → otherwise", EdgeContext{Output: outputWith(map[string]any{"severeIssueCount": "not a number"})}, "commit"},
		{"float64-typed data (decoded JSON) matches", EdgeContext{Output: outputWith(map[string]any{"severeIssueCount": 2.0})}, "revise"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pickOn(c.ctx); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
	t.Run("attaches targets", func(t *testing.T) {
		if strings.Join(pick.Targets, ",") != "revise,commit" {
			t.Fatalf("targets %v", pick.Targets)
		}
	})
	t.Run("construction refusals", func(t *testing.T) {
		_, err := Gate("count", nil, "x")
		mustMatch(t, err, "at least one possible return value")
		_, err = Gate("count", []GateBranch{{"a", Gt(0)}}, "")
		mustMatch(t, err, "explicit `otherwise`")
		_, err = Gate("count", []GateBranch{{"2", Eq(2)}, {"other", Gt(0)}}, "other")
		mustMatch(t, err, "integer-like")
	})
	t.Run("includes otherwise in targets when not also a branch", func(t *testing.T) {
		fn := Must(Gate("count", []GateBranch{{"high", Gt(10)}}, "low"))
		if strings.Join(fn.Targets, ",") != "high,low" {
			t.Fatalf("targets %v", fn.Targets)
		}
	})
	t.Run("records a fallback note readable via TakeRouteNote — only on no-match", func(t *testing.T) {
		fn := Must(Gate("count", []GateBranch{{"high", Gt(10)}}, "low"))
		ctx := func(n any) EdgeContext {
			return EdgeContext{Output: &Output{Kind: "x", Artifacts: []Artifact{}, Data: map[string]any{"count": n}, Meta: OutputMeta{Stage: "s", StageNumber: 1}}}
		}
		if got, _ := fn.Pick(ctx(11)); got != "high" {
			t.Fatalf("got %q", got)
		}
		if note, ok := TakeRouteNote(fn); ok {
			t.Fatalf("unexpected note %q", note)
		}
		if got, _ := fn.Pick(ctx(3)); got != "low" {
			t.Fatalf("got %q", got)
		}
		note, ok := TakeRouteNote(fn)
		if !ok || note != `gate("count"): no branch matched value 3 — fell back to "low"` {
			t.Fatalf("note %q (%v)", note, ok)
		}
		if _, ok := TakeRouteNote(fn); ok {
			t.Fatal("note not cleared")
		}
	})
}

func TestDefineRoute(t *testing.T) {
	t.Run("attaches the declared targets", func(t *testing.T) {
		fn := Must(DefineRoute([]string{"good", "bad"}, func(EdgeContext) (string, error) { return "good", nil }, nil))
		if strings.Join(fn.Targets, ",") != "good,bad" {
			t.Fatalf("targets %v", fn.Targets)
		}
	})
	t.Run("preserves the underlying runtime behaviour", func(t *testing.T) {
		fn := Must(DefineRoute([]string{"good", "bad"}, func(ctx EdgeContext) (string, error) {
			if ok, _ := fieldOf(ctx.Output.Data, "ok"); ok == true {
				return "good", nil
			}
			return "bad", nil
		}, nil))
		got, _ := fn.Pick(EdgeContext{Output: &Output{Kind: "test", Data: map[string]any{"ok": true}}})
		if got != "good" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("refuses an empty targets list", func(t *testing.T) {
		_, err := DefineRoute(nil, func(EdgeContext) (string, error) { return "x", nil }, nil)
		mustMatch(t, err, "at least one possible return value")
	})
	t.Run("readsData false opts out of the lint", func(t *testing.T) {
		fn := Must(DefineRoute([]string{"a", "b"}, func(EdgeContext) (string, error) { return "a", nil }, &DefineRouteOptions{ReadsData: Bool(false)}))
		if strings.Join(fn.Targets, ",") != "a,b" || MarksReadsData(fn) {
			t.Fatalf("targets %v marks %v", fn.Targets, MarksReadsData(fn))
		}
	})
	t.Run("never mutates the caller's function — reusing one predicate keeps each route's targets", func(t *testing.T) {
		predicate := func(EdgeContext) (string, error) { return "x", nil }
		first := Must(DefineRoute([]string{"x", "y"}, predicate, nil))
		second := Must(DefineRoute([]string{"x"}, predicate, nil))
		if strings.Join(first.Targets, ",") != "x,y" || strings.Join(second.Targets, ",") != "x" {
			t.Fatalf("aliased targets: %v / %v", first.Targets, second.Targets)
		}
	})
	t.Run("readsData false does not inherit a prior marker", func(t *testing.T) {
		predicate := func(EdgeContext) (string, error) { return "a", nil }
		marked := Must(DefineRoute([]string{"a"}, predicate, nil))
		unmarked := Must(DefineRoute([]string{"a"}, predicate, &DefineRouteOptions{ReadsData: Bool(false)}))
		if !MarksReadsData(marked) || MarksReadsData(unmarked) {
			t.Fatal("marker leaked between routes")
		}
	})
	t.Run("setRouteNote attaches a note the audit reads-and-clears", func(t *testing.T) {
		fn := Must(DefineRoute([]string{"a"}, func(EdgeContext) (string, error) { return "a", nil }, nil))
		SetRouteNote(fn, "because")
		if note, ok := TakeRouteNote(fn); !ok || note != "because" {
			t.Fatalf("note %q", note)
		}
		if _, ok := TakeRouteNote(fn); ok {
			t.Fatal("not cleared")
		}
	})
}

func TestMatch(t *testing.T) {
	route := Must(Match("severity", []MatchBranch{{"escalate", "p0"}, {"fix", "p1"}, {"backlog", "p2"}}, nil))
	ctx := func(data any) EdgeContext { return EdgeContext{Output: outputWith(data)} }
	t.Run("routes on strict equality in declaration order", func(t *testing.T) {
		for value, want := range map[string]string{"p0": "escalate", "p1": "fix", "p2": "backlog"} {
			if got, _ := route.Pick(ctx(map[string]any{"severity": value})); got != want {
				t.Fatalf("%s → %q want %q", value, got, want)
			}
		}
	})
	t.Run("no fallback terminates with a note", func(t *testing.T) {
		got, _ := route.Pick(ctx(map[string]any{"severity": "p9"}))
		if got != Stop {
			t.Fatalf("got %q", got)
		}
		note, _ := TakeRouteNote(route)
		if note != `match("severity"): value "p9" matched no branch — terminated (no fallback)` {
			t.Fatalf("note %q", note)
		}
		if strings.Join(route.Targets, ",") != "escalate,fix,backlog,stop" {
			t.Fatalf("targets %v", route.Targets)
		}
	})
	t.Run("fallback routes with a note; missing field renders null", func(t *testing.T) {
		fb := Must(Match("tie", []MatchBranch{{"escalate", true}}, &MatchOptions{Fallback: "keep"}))
		got, _ := fb.Pick(ctx(map[string]any{}))
		if got != "keep" {
			t.Fatalf("got %q", got)
		}
		note, _ := TakeRouteNote(fb)
		if note != `match("tie"): value null matched no branch — fell back to "keep"` {
			t.Fatalf("note %q", note)
		}
		if !MarksReadsData(fb) {
			t.Fatal("stage-output match should mark ReadsData")
		}
	})
	t.Run("from reads a named channel's latest output and does not mark ReadsData", func(t *testing.T) {
		fromRoute := Must(Match("tie", []MatchBranch{{"escalate", true}}, &MatchOptions{Fallback: "keep", From: "review-panel"}))
		state := &RunState{NamedValue: map[string][]Output{"review-panel": {{Data: map[string]any{"tie": false}}, {Data: map[string]any{"tie": true}}}}}
		got, _ := fromRoute.Pick(EdgeContext{State: state})
		if got != "escalate" || MarksReadsData(fromRoute) {
			t.Fatalf("got %q marks %v", got, MarksReadsData(fromRoute))
		}
	})
	t.Run("0 / \"0\" / false stay distinct values", func(t *testing.T) {
		if _, err := Match("v", []MatchBranch{{"a", 0}, {"b", "0"}, {"c", false}}, nil); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("construction refusals", func(t *testing.T) {
		_, err := Match("v", []MatchBranch{{"a", "x"}, {"b", "x"}}, nil)
		mustMatch(t, err, `value "x" is claimed by both "a" and "b"`)
		_, err = Match("v", nil, nil)
		mustMatch(t, err, "at least one possible return value")
		_, err = Match("v", []MatchBranch{{"3", "x"}}, nil)
		mustMatch(t, err, "integer-like")
		_, err = Match("v", []MatchBranch{{"a", 1}, {"b", 1.0}}, nil)
		mustMatch(t, err, "claimed by both")
	})
}

func TestCompositionSmoke(t *testing.T) {
	w := DefineWorkflow(Workflow{
		Name:  "review-or-ship",
		Start: "research",
		Stages: map[string]StageDef{
			"research":    Produces(),
			"code-review": Produces(StageDef{OutputSchema: TypeboxSchema(T.Object(map[string]Schema{"severeIssueCount": T.Integer(Schema{"minimum": 0})}))}),
			"revise":      Produces(),
			"commit":      Acts(),
		},
		Edges: map[string]EdgeTarget{
			"research":    To("code-review"),
			"code-review": Route(Must(Gate("severeIssueCount", []GateBranch{{"revise", Gt(0)}, {"commit", Eq(0)}}, "commit"))),
			"revise":      To("commit"),
			"commit":      To(Stop),
		},
	})
	if w.Name != "review-or-ship" || w.Start != "research" || len(w.Stages) != 4 {
		t.Fatalf("got %+v", w)
	}
	if w.Edges["code-review"].Fn() == nil || w.Edges["research"].Name() != "code-review" || !w.Edges["commit"].IsStop() {
		t.Fatalf("edges %+v", w.Edges)
	}
}

func TestStageDefArms(t *testing.T) {
	skillArm := StageDef{Kind: KindProduces, SessionPolicy: SessionFresh, Outcome: &Outcome{Collector: NoopCollector}}
	scriptArm := StageDef{Kind: KindSideEffect, SessionPolicy: SessionFresh, Run: ActsScriptFn(func(context.Context, ScriptContext) error { return nil })}
	promptArm := StageDef{Kind: KindSideEffect, SessionPolicy: SessionFresh, Prompt: StaticPrompt("say hi")}
	if !IsDispatchingStage(skillArm) || IsDispatchingStage(scriptArm) || IsDispatchingStage(promptArm) {
		t.Fatal("dispatch discriminator wrong")
	}
}
