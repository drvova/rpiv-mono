package workflow

import (
	"context"
	"testing"
)

// Stage-def read-declaration tests — Fanin and the ReadName / ReadsAll
// normalisers, plus the progress-hook surface (stage-def.test.ts).

func TestFanin(t *testing.T) {
	read, err := Fanin("plans")
	if err != nil || read.Name != "plans" || !read.All {
		t.Fatalf("got %+v %v", read, err)
	}
	if _, err := Fanin(""); err == nil || err.Error() != "fanin(name): name must be a non-empty channel name" {
		t.Fatalf("got %v", err)
	}
}

func TestReadNameAndReadsAll(t *testing.T) {
	cases := []struct {
		name string
		read StageRead
		want string
		all  bool
	}{
		{"bare string unchanged, latest-wins", Read("plans"), "plans", false},
		{"spec name, all:true", StageRead{Name: "plans", All: true}, "plans", true},
		{"fanin spec", Must(Fanin("plans")), "plans", true},
		{"explicit all:false", StageRead{Name: "plans", All: false}, "plans", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if ReadName(c.read) != c.want || ReadsAll(c.read) != c.all {
				t.Fatalf("got %q/%v", ReadName(c.read), ReadsAll(c.read))
			}
		})
	}
}

func TestProgressHookSurface(t *testing.T) {
	hook := ProgressFn(func(context.Context, RunView) (ProgressValue, error) { return ProgressImproved, nil })
	outcome := &Outcome{Collector: NoopCollector}
	if len(ProgressValues) != 4 || ProgressValues[0] != "improved" || ProgressValues[1] != "unchanged" || ProgressValues[2] != "regressed" || ProgressValues[3] != "unknown" {
		t.Fatalf("ProgressValues %v", ProgressValues)
	}
	defs := map[string]StageDef{
		"produces":        Produces(StageDef{Outcome: outcome, Progress: hook}),
		"produces.script": ProducesScript(ProducesScriptOptions{Run: func(context.Context, ScriptContext) (ScriptOutput, error) { return ScriptOutput{Kind: "x"}, nil }, Progress: hook}),
		"produces.prompt": ProducesPrompt(ProducesPromptOptions{Prompt: StaticPrompt("go"), Outcome: outcome, Progress: hook}),
		"acts.script":     ActsScript(ActsScriptOptions{Run: func(context.Context, ScriptContext) error { return nil }, Progress: hook}),
		"literal":         {Kind: KindSideEffect, SessionPolicy: SessionFresh, Progress: hook},
	}
	for name, def := range defs {
		t.Run(name, func(t *testing.T) {
			if def.Progress == nil {
				t.Fatal("progress not threaded")
			}
			if v, _ := def.Progress(context.Background(), nil); v != ProgressImproved {
				t.Fatalf("got %q", v)
			}
		})
	}
	t.Run("prompt factories honour sessionPolicy continue", func(t *testing.T) {
		d := ProducesPrompt(ProducesPromptOptions{Prompt: StaticPrompt("go"), Outcome: outcome, SessionPolicy: SessionContinue})
		if d.SessionPolicy != SessionContinue || !d.Prompt.IsSet() || d.Prompt.Text() != "go" {
			t.Fatalf("got %+v", d)
		}
		a := ActsPrompt(ActsPromptOptions{Prompt: DynamicPrompt(func(context.Context, ScriptContext) (string, error) { return "dyn", nil })})
		if a.SessionPolicy != SessionFresh || !a.Prompt.IsDynamic() {
			t.Fatalf("got %+v", a)
		}
		text, _ := a.Prompt.Resolve(context.Background(), ScriptContext{})
		if text != "dyn" {
			t.Fatalf("got %q", text)
		}
	})
}
