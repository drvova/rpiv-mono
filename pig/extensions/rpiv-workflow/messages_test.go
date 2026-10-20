package workflow

import (
	"strings"
	"testing"
)

func TestUnifiedLoopMessages(t *testing.T) {
	if got := MsgLoopZeroUnits("blueprint"); got != "rpiv: blueprint iterate loop produced zero units — nothing published, advancing" {
		t.Fatal(got)
	}
	if got := FailLoopCapHalt(5, 5); got != (FailureText{Toast: "rpiv: loop cap exceeded (5/5) — stopping workflow to prevent an unbounded loop", Error: "Loop cap exceeded: 5 units (max 5)"}) {
		t.Fatalf("%+v", got)
	}
	if got := MsgLoopCapAdvance("breakdown", 8); got != "rpiv: breakdown loop reached its cap (8) — projecting the configured result and advancing" {
		t.Fatal(got)
	}
	if !strings.Contains(ErrResumeLoopMismatch("implement"), `cannot resume — loop stage "implement" recomputed a different unit`) {
		t.Fatal(ErrResumeLoopMismatch("implement"))
	}
	if got := MsgResumeLoopMismatch("implement"); got != `rpiv: loop "implement" changed on resume — cannot safely continue` {
		t.Fatal(got)
	}
}

func TestFailBackwardJumpExhausted(t *testing.T) {
	cap4 := BackwardJumpHaltInfo{Stage: "grade", LimitKind: BackwardJumpCap, Count: 4, Max: 3}
	cases := []struct {
		name  string
		info  BackwardJumpHaltInfo
		toast string
		err   string
	}{
		{"cap arm, empty ring", cap4,
			`rpiv: backward-jump limit exceeded — "grade" re-entered 4 times (max 3) — stopping workflow to prevent infinite loop`,
			`Backward-jump limit exceeded: stage "grade" re-entered 4 times (max 3)`},
		{"cap arm, populated ring", with(cap4, []ProgressValue{"regressed"}),
			`rpiv: backward-jump limit exceeded — "grade" re-entered 4 times (max 3); last progress: regressed — stopping workflow to prevent infinite loop`,
			`Backward-jump limit exceeded: stage "grade" re-entered 4 times (max 3); last progress: regressed`},
		{"ring renders oldest → newest", BackwardJumpHaltInfo{Stage: "grade", LimitKind: BackwardJumpCap, Count: 2, Max: 1, Progress: []ProgressValue{"improved", "unknown", "regressed"}},
			`rpiv: backward-jump limit exceeded — "grade" re-entered 2 times (max 1); last progress: improved, unknown, regressed — stopping workflow to prevent infinite loop`,
			`Backward-jump limit exceeded: stage "grade" re-entered 2 times (max 1); last progress: improved, unknown, regressed`},
		{"ceiling arm, empty ring", BackwardJumpHaltInfo{Stage: "a", LimitKind: BackwardJumpCeiling, Count: 9, Max: 8},
			`rpiv: backward-jump limit exceeded — "a" re-entered 9 times, over the absolute lap ceiling (max 8) — stopping workflow to prevent infinite loop`,
			`Backward-jump limit exceeded: stage "a" re-entered 9 times, over the absolute lap ceiling (max 8)`},
		{"ceiling arm, populated ring", BackwardJumpHaltInfo{Stage: "a", LimitKind: BackwardJumpCeiling, Count: 9, Max: 8, Progress: []ProgressValue{"improved", "improved", "improved"}},
			`rpiv: backward-jump limit exceeded — "a" re-entered 9 times, over the absolute lap ceiling (max 8); last progress: improved, improved, improved — stopping workflow to prevent infinite loop`,
			`Backward-jump limit exceeded: stage "a" re-entered 9 times, over the absolute lap ceiling (max 8); last progress: improved, improved, improved`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FailBackwardJumpExhausted(c.info)
			if got.Toast != c.toast || got.Error != c.err {
				t.Fatalf("got  %q / %q\nwant %q / %q", got.Toast, got.Error, c.toast, c.err)
			}
			if len(c.info.Progress) == 0 && strings.Contains(got.Error+got.Toast, "last progress") {
				t.Fatal("bare clause")
			}
		})
	}
	if BackwardJumpLimitHead != "Backward-jump limit exceeded" {
		t.Fatal(BackwardJumpLimitHead)
	}
}

func with(info BackwardJumpHaltInfo, progress []ProgressValue) BackwardJumpHaltInfo {
	info.Progress = progress
	return info
}

func TestMessageRenderings(t *testing.T) {
	cases := []struct{ got, want string }{
		{MsgStageFailed("plan"), "✗ plan failed — stopping workflow"},
		{FailStageAborted("plan").Toast, "⏸ plan aborted (ESC) — stopping workflow"},
		{FailStageAborted("plan").Error, "plan aborted by user (ESC)"},
		{MsgWorkflowComplete(3), "rpiv: workflow complete (3 stages)"},
		{FailWorkflowAborted("build").Error, `workflow aborted before stage "build" (signal)`},
		{ErrSchemaTimeout(SlotOutputSchema, 500), "outputSchema validation exceeded 500ms — schema's ~standard.validate did not settle"},
		{FailMissingArtifact("impl", 2).Error, "Stage 2 (impl) has no upstream artifactPath; only stage 1 may consume the user's original input"},
		{FailMissingNamedRead("synth", "plans", 3).Toast, `✗ synth reads "plans" but no upstream produces stage has published it yet — stopping workflow`},
		{FailGateStop("gate", `value "x" matched no branch`, "r1").Toast, `✗ workflow stopped at "gate" — its routing gate matched no branch (value "x" matched no branch); see the stage's verdict for findings, then fix and resume with /wf @r1`},
		{FailGateStop("gate", "3/3 rounds spent", "r1").Error, `Workflow halted at "gate" (budget exhausted): 3/3 rounds spent`},
		{FailFanoutAllFailed("design", 2, 2).Error, `Fanout all-failed at stage "design" (2/2 units failed)`},
		{FailVerifyFailed("build", 1).Toast, "✗ build verification failed after 1 attempt — stopping workflow"},
		{FailVerifyFailed("build", 3).Error, "Verification failed for \"build\": the judge's verdict did not satisfy `pass` after 3 attempts"},
		{MsgLifecycleThrew("onStageEnd", "boom"), "⚠ rpiv: lifecycle listener (onStageEnd) threw: boom"},
		{ErrResumeVersionMismatch("header v2", 3), "rpiv: cannot resume — header v2, but this build reads schema v3; resume refuses rather than misread the trail"},
		{ReattachPrompt("plan"), "The previous run of /skill:plan in this session was interrupted. Review the conversation above. If the work is already complete, announce the artifact path. Otherwise finish the remaining work and announce the artifact path when done."},
		{MsgNameInvalid("9x"), `rpiv: invalid name "9x" — must be 1-64 chars, start with a letter or underscore, only letters, digits, hyphens, underscores`},
		{MsgJumpCapAboveLapCeiling(20, 8), "/wf: --max-jumps 20 is at or above the lap ceiling 8 — the ceiling halts first, so at most 8 re-entries per stage; raise --max-laps to widen the run"},
		{MsgBudgetInvalid("maxLaps", -1), "maxLaps must be a non-negative integer, got -1"},
		{MsgBudgetInvalid("maxLaps", 1.5), "maxLaps must be a non-negative integer, got 1.5"},
		{MsgLoadAborted(1), "/wf: 1 config error — see warnings above (fix and re-run)"},
		{MsgLoadAborted(2), "/wf: 2 config errors — see warnings above (fix and re-run)"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("got  %q\nwant %q", c.got, c.want)
		}
	}
	nan := MsgBudgetInvalid("maxIterations", jsNumber("abc"))
	if nan != "maxIterations must be a non-negative integer, got NaN" {
		t.Fatal(nan)
	}
}

func TestIsExhaustionHalt(t *testing.T) {
	cases := map[string]bool{
		"3/3 rounds spent":                 true,
		"unit FATAL — boom":                true,
		"the same failures survived a fix": true,
		"integrity stop":                   true,
		`value "x" matched no branch`:      false,
		"spent rounds":                     false,
	}
	for note, want := range cases {
		n := note
		if IsExhaustionHalt(&n) != want {
			t.Errorf("%q: want %v", note, want)
		}
	}
	if IsExhaustionHalt(nil) {
		t.Fatal("nil note")
	}
}

func TestFatalArgs(t *testing.T) {
	f := FailedArgs(FailureText{Toast: "t", Error: "e"})
	if f.Status != FatalFailed || f.NotifyLevel != NotifyError || f.NotifyMsg != "t" || f.ErrMsg != "e" {
		t.Fatalf("%+v", f)
	}
	a := AbortedArgsOf("t", "e")
	if a.Status != FatalAborted || a.NotifyLevel != NotifyWarning || a.NotifyMsg != "t" || a.ErrMsg != "e" {
		t.Fatalf("%+v", a)
	}
	if FailedArgsOf("t", "e") != f || AbortedArgs(FailureText{Toast: "t", Error: "e"}) != a {
		t.Fatal("two-string forms diverge")
	}
}
