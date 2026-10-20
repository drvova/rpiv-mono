package workflow

import (
	"encoding/json"
	"reflect"
	"testing"
)

var baseMeta = OutputMeta{Stage: "research", Skill: "research", StageNumber: 3, Ts: "2026-05-24T08:00:00Z", RunID: "2026-05-24_08-00-00-abcd"}

func TestFinalizeOutput(t *testing.T) {
	t.Run("stamps every meta field", func(t *testing.T) {
		m := FinalizeOutput("artifact-md", []Artifact{{Handle: FS(".rpiv/artifacts/research/r.md"), Role: "primary"}}, map[string]any{"foo": 1}, baseMeta)
		if m.Meta != baseMeta {
			t.Fatalf("meta %+v", m.Meta)
		}
	})
	t.Run("forwards kind, artifacts and data unchanged", func(t *testing.T) {
		artifacts := []Artifact{{Handle: FS(".rpiv/artifacts/prior/x.md")}}
		data := map[string]any{"sha": "deadbeef"}
		m := FinalizeOutput("git-commit", artifacts, data, baseMeta)
		if m.Kind != "git-commit" || !reflect.DeepEqual(m.Data, data) || &m.Artifacts[0] != &artifacts[0] {
			t.Fatalf("got %+v", m)
		}
	})
	t.Run("accepts an empty artifacts list", func(t *testing.T) {
		m := FinalizeOutput("side-effect", []Artifact{}, map[string]any{}, baseMeta)
		if len(m.Artifacts) != 0 || m.Kind != "side-effect" {
			t.Fatalf("got %+v", m)
		}
	})
	t.Run("meta.skill cannot be spoofed through data", func(t *testing.T) {
		m := FinalizeOutput("artifact-md", []Artifact{}, map[string]any{"skill": "evil-skill", "foo": 1}, baseMeta)
		if m.Meta.Skill != "research" || m.Data.(map[string]any)["skill"] != "evil-skill" {
			t.Fatalf("got %+v", m)
		}
	})
	t.Run("preserves payload data by reference", func(t *testing.T) {
		data := map[string]any{"nested": []int{1, 2, 3}}
		m := FinalizeOutput("artifact-md", []Artifact{}, data, baseMeta)
		data["added"] = true
		if _, ok := m.Data.(map[string]any)["added"]; !ok {
			t.Fatal("data was cloned")
		}
	})
}

func TestFailedOutput(t *testing.T) {
	o := FailedOutput(baseMeta, "validation exhausted", "")
	if o.Kind != FailedOutputKind || len(o.Artifacts) != 0 || o.Meta != baseMeta {
		t.Fatalf("got %+v", o)
	}
	raw, _ := json.Marshal(o.Data)
	if string(raw) != `{"reason":"validation exhausted"}` {
		t.Fatalf("dimensionless sentinel data %s", raw)
	}
	withDim := FailedOutput(baseMeta, "boom", "security")
	raw, _ = json.Marshal(withDim.Data)
	if string(raw) != `{"reason":"boom","dimension":"security"}` {
		t.Fatalf("dimension sentinel data %s", raw)
	}
	if !IsFailedOutput(o) || IsFailedOutput(FinalizeOutput("artifacts", []Artifact{{Handle: FS("a.md")}}, map[string]any{}, baseMeta)) {
		t.Fatal("isFailedOutput")
	}
}

func TestNewOutputMeta(t *testing.T) {
	t.Run("omits the skill key when absent (script-row contract)", func(t *testing.T) {
		m := NewOutputMeta(OutputMetaArgs{Stage: "impl", StageNumber: 1, Ts: "t1", RunID: "r1"})
		raw, _ := json.Marshal(m)
		if string(raw) != `{"stage":"impl","stageNumber":1,"ts":"t1","runId":"r1"}` {
			t.Fatalf("got %s", raw)
		}
	})
	t.Run("includes the skill key when supplied, in the persisted order", func(t *testing.T) {
		m := NewOutputMeta(OutputMetaArgs{Stage: "research", Skill: "research", StageNumber: 3, Ts: "t3", RunID: "r3"})
		raw, _ := json.Marshal(m)
		if string(raw) != `{"stage":"research","skill":"research","stageNumber":3,"ts":"t3","runId":"r3"}` {
			t.Fatalf("got %s", raw)
		}
	})
	t.Run("reproduces ts verbatim", func(t *testing.T) {
		fixed := "2026-06-24T23:05:06-0400"
		if NewOutputMeta(OutputMetaArgs{Stage: "x", StageNumber: 9, Ts: fixed, RunID: "rx"}).Ts != fixed {
			t.Fatal("ts minted")
		}
	})
	t.Run("live-vs-resume parity", func(t *testing.T) {
		args := OutputMetaArgs{Stage: "impl (phase-2)", Skill: "impl", StageNumber: 2, Ts: "t2", RunID: "run-id"}
		if NewOutputMeta(args) != NewOutputMeta(args) {
			t.Fatal("not structurally equal")
		}
	})
	t.Run("envelope JSON order matches the TypeScript row", func(t *testing.T) {
		o := FinalizeOutput("artifacts", []Artifact{{Handle: FS("a.md"), Role: "primary"}}, []Artifact{{Handle: FS("a.md")}}, NewOutputMeta(OutputMetaArgs{Stage: "s", StageNumber: 1, Ts: "t", RunID: "r"}))
		raw, _ := json.Marshal(o)
		want := `{"kind":"artifacts","artifacts":[{"handle":{"kind":"fs","path":"a.md"},"role":"primary"}],"data":[{"handle":{"kind":"fs","path":"a.md"}}],"meta":{"stage":"s","stageNumber":1,"ts":"t","runId":"r"}}`
		if string(raw) != want {
			t.Fatalf("got  %s\nwant %s", raw, want)
		}
	})
}
