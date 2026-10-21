package workflow

import (
	"context"
	"reflect"
	"testing"
)

var verdictOutcome = &NamedOutcome{Name: "verdict"}

func TestJudgeShapeIssues(t *testing.T) {
	cases := []struct {
		name      string
		candidate any
		want      []string
	}{
		{"valid skill judge", &Judge{Skill: "grade", Outcome: verdictOutcome}, []string{}},
		{"valid prompt judge", Judge{Prompt: StaticJudgePrompt("grade this"), Outcome: verdictOutcome}, []string{}},
		{"missing judge object", nil, []string{"a judge object is required"}},
		{"missing outcome.name", &Judge{Skill: "grade", Outcome: &Outcome{}}, []string{"judge.outcome must carry a `name` so the verdict publishes to its own named channel"}},
		{"skill+prompt", &Judge{Skill: "grade", Prompt: StaticJudgePrompt("x"), Outcome: verdictOutcome}, []string{"judge sets both `skill` and `prompt` — choose one dispatch (skill XOR prompt)"}},
		{"neither", &Judge{Outcome: verdictOutcome}, []string{"judge sets neither `skill` nor `prompt` — one is required to dispatch the judge"}},
		{"map literal", map[string]any{"skill": "grade", "outcome": map[string]any{"name": "v"}}, []string{}},
		{"nil pointer", (*Judge)(nil), []string{"a judge object is required"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := JudgeShapeIssues(c.candidate); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestNewJudge(t *testing.T) {
	spec := Judge{Skill: "grade", Outcome: verdictOutcome}
	j, err := NewJudge(spec)
	if err != nil || j.Skill != "grade" || j.Outcome != verdictOutcome {
		t.Fatalf("got %+v %v", j, err)
	}
	_, err = NewJudge(Judge{Skill: "grade", Outcome: &Outcome{}})
	mustMatch(t, err, `judge\(\):.*name`)
	_, err = NewJudge(Judge{Skill: "grade", Prompt: StaticJudgePrompt("x"), Outcome: verdictOutcome})
	mustMatch(t, err, "skill XOR prompt")
	_, err = NewJudge(Judge{Outcome: verdictOutcome})
	mustMatch(t, err, "one is required to dispatch")
}

func TestPanelMembersIsPanel(t *testing.T) {
	j := Must(NewJudge(Judge{Skill: "grade", Outcome: verdictOutcome}))
	if got := PanelMembers(j); len(got) != 1 || got[0] != j {
		t.Fatalf("got %v", got)
	}
	a := Must(NewJudge(Judge{Skill: "a", Outcome: &NamedOutcome{Name: "va"}}))
	b := Must(NewJudge(Judge{Skill: "b", Outcome: &NamedOutcome{Name: "vb"}}))
	p := &PanelJudge{Members: []*Judge{a, b}, Fold: NewFold(func(vs []Output) (any, error) { return vs, nil })}
	if got := PanelMembers(p); len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("got %v", got)
	}
	if IsPanel(j) || !IsPanel(p) || IsPanel(nil) {
		t.Fatal("isPanel")
	}
	t.Run("a nil slot has no members and never panics", func(t *testing.T) {
		if PanelMembers(nil) != nil || PanelMembers((*PanelJudge)(nil)) != nil || PanelMembers((*Judge)(nil)) != nil {
			t.Fatal("typed-nil slots must expand to nothing")
		}
		if IsPanel((*PanelJudge)(nil)) {
			t.Fatal("a typed-nil panel is not a panel")
		}
	})
}

func TestResolveJudgePrompt(t *testing.T) {
	static := StaticJudgePrompt("grade")
	if got, _ := ResolveJudgePrompt(t.Context(), static, JudgeContext{}); got != "grade" {
		t.Fatalf("got %q", got)
	}
	dynamic := DynamicJudgePrompt(func(_ context.Context, jc JudgeContext) (string, error) { return jc.Output.Kind, nil })
	if got, _ := ResolveJudgePrompt(t.Context(), dynamic, JudgeContext{Output: Output{Kind: "k"}}); got != "k" {
		t.Fatalf("got %q", got)
	}
}
