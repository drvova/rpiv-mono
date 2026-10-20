package workflow

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestLifecycleDispatcher(t *testing.T) {
	t.Cleanup(ResetLifecycleRegistry)
	ResetLifecycleRegistry()
	var order []string
	dispose := RegisterLifecycle(&LifecycleListeners{OnStageStart: func(s StageRef, _ LifecycleContext) error { order = append(order, "global:"+s.Name); return nil }})
	second := RegisterLifecycle(&LifecycleListeners{OnStageStart: func(StageRef, LifecycleContext) error { order = append(order, "second"); return errors.New("boom") }})
	perCall := &LifecycleListeners{OnStageStart: func(StageRef, LifecycleContext) error { order = append(order, "perCall"); panic("listener panic") }}
	ui := &fakeUI{}
	host := &fakeLauncher{ui: ui}
	d := NewLifecycleDispatcher(perCall)
	ctx := LifecycleContext{RunID: "r"}
	fire := func() {
		d.Fire(host, EventStageStart, func(b *LifecycleListeners) error {
			if b.OnStageStart == nil {
				return nil
			}
			return b.OnStageStart(SkillStageRef("plan", 1, "plan"), ctx)
		})
	}
	fire()
	if strings.Join(order, ",") != "global:plan,second,perCall" {
		t.Fatalf("%v", order)
	}
	if len(ui.notes) != 2 || ui.notes[0] != "warning: ⚠ rpiv: lifecycle listener (onStageStart) threw: boom" || ui.notes[1] != "warning: ⚠ rpiv: lifecycle listener (onStageStart) threw: listener panic" {
		t.Fatalf("%v", ui.notes)
	}
	second()
	dispose()
	order = nil
	fire()
	if strings.Join(order, ",") != "perCall" {
		t.Fatalf("disposed bundles still fire: %v", order)
	}
	// A bundle without the slot is skipped silently.
	ui.notes = nil
	NewLifecycleDispatcher(&LifecycleListeners{}).Fire(host, EventRoute, func(b *LifecycleListeners) error {
		if b.OnRoute == nil {
			return nil
		}
		return b.OnRoute(StageRef{}, Stop, ctx)
	})
	if len(ui.notes) != 0 {
		t.Fatal("unexpected notify")
	}
}

func TestLifecycleContextProjections(t *testing.T) {
	state := &RunState{OriginalInputValue: "in"}
	run := &RunContext{Cwd: "/c", RunID: "r", Workflow: Workflow{Name: "w"}, TotalStages: 3, Trigger: RunTrigger{Kind: TriggerKindCommand, Name: "wf"}, State: state}
	want := LifecycleContext{Cwd: "/c", RunID: "r", Workflow: "w", TotalStages: 3, Trigger: run.Trigger, State: state}
	if !reflect.DeepEqual(LifecycleCtxFor(run), want) {
		t.Fatalf("%+v", LifecycleCtxFor(run))
	}
	s := &SessionContext{Cwd: "/c", RunID: "r", RunIdentity: RunIdentity{Workflow: "w", TotalStages: 3, Trigger: run.Trigger}, State: state}
	if !reflect.DeepEqual(LifecycleCtxFromSession(s), want) {
		t.Fatalf("%+v", LifecycleCtxFromSession(s))
	}
	if ScriptStageRef("impl", 2) != (StageRef{Kind: StageRefScript, Name: "impl", StageNumber: 2}) {
		t.Fatal("scriptStageRef")
	}
	if DefaultTrigger.Kind != TriggerKindProgrammatic || jsonStringify(DefaultTrigger) != `{"kind":"programmatic"}` {
		t.Fatal("default trigger")
	}
}
