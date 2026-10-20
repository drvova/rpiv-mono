package workflow

import "sort"

// Loop introspection — pure-data projections of a stage's control flow +
// edge shape. A workflow's structure is read entirely from attached data (no
// probing), so an analysing/suggesting agent can render or reason about a
// flow without running it.

// EdgeMode is the outgoing-edge shape of a StageShape.
type EdgeMode string

const (
	EdgeLinear   EdgeMode = "linear"
	EdgeRoute    EdgeMode = "route"
	EdgeTerminal EdgeMode = "terminal"
)

// ControlMode is "single" or a LoopKind.
type ControlMode string

// ControlSingle is the control mode of a stage without a loop.
const ControlSingle ControlMode = "single"

// StageControl is the control-flow facet of a StageShape.
type StageControl struct {
	Mode ControlMode
	Spec *LoopSpec
}

// StageEdge is the outgoing-edge facet of a StageShape. Mode "terminal" is
// the GRAPH-SINK sense (no outgoing edge OR an explicit Stop) — unrelated to
// the Terminal stage factory.
type StageEdge struct {
	Mode    EdgeMode
	Targets []string
}

// StageReadSpec is one normalised `reads:` entry of a StageShape.
type StageReadSpec struct {
	Name string
	All  bool
}

// VerifySpecShape is the verify projection of a StageShape: the judge-slot
// summary plus the attempt budget.
type VerifySpecShape struct {
	Judge AnyJudgeSpec
	Max   int
}

// StageShape is the per-stage control-flow + edge shape, read entirely from
// attached data. Control.Mode stays "single" for verify stages — verify is a
// stage property in the introspection model, not a loop kind.
type StageShape struct {
	Stage   string
	Skill   string
	Control StageControl
	Verify  *VerifySpecShape
	Reads   []StageReadSpec
	Edge    StageEdge
}

// DescribeFlow describes a workflow's structure stage-by-stage from attached
// metadata alone, in stage-name order.
func DescribeFlow(w Workflow) []StageShape {
	names := make([]string, 0, len(w.Stages))
	for name := range w.Stages {
		names = append(names, name)
	}
	sort.Strings(names)
	shapes := make([]StageShape, 0, len(names))
	for _, name := range names {
		stage := w.Stages[name]
		control := StageControl{Mode: ControlSingle}
		if stage.Loop != nil {
			control = StageControl{Mode: ControlMode(stage.Loop.Kind()), Spec: LoopSpecOf(stage.Loop)}
		}
		target := w.Edges[name]
		var edge StageEdge
		switch {
		case target.IsZero() || target.IsStop():
			edge = StageEdge{Mode: EdgeTerminal}
		case target.Fn() == nil:
			edge = StageEdge{Mode: EdgeLinear, Targets: []string{target.Name()}}
		default:
			edge = StageEdge{Mode: EdgeRoute, Targets: target.Fn().Targets}
		}
		shape := StageShape{Stage: name, Skill: stage.Skill, Control: control, Edge: edge}
		if stage.Verify != nil {
			maxCap := 1
			if stage.Verify.Max != nil {
				maxCap = *stage.Verify.Max
			}
			shape.Verify = &VerifySpecShape{Judge: JudgeSlotSpecOf(stage.Verify.Judge), Max: maxCap}
		}
		if len(stage.Reads) > 0 {
			shape.Reads = make([]StageReadSpec, len(stage.Reads))
			for i, r := range stage.Reads {
				shape.Reads[i] = StageReadSpec{Name: ReadName(r), All: ReadsAll(r)}
			}
		}
		shapes = append(shapes, shape)
	}
	return shapes
}

// JudgeSpec is the pure-data judge summary — Prompt true means a prompt judge
// (the text/closure stays opaque); Outcome is the verdict channel.
type JudgeSpec struct {
	Skill   string
	Prompt  bool
	Outcome string
}

// PanelJudgeSpec is the pure-data summary of an N-member panel: one JudgeSpec
// per member; Fold is the sugar name for a canonical fold, "custom" for a raw
// author fold; Outcome is the custom verdict channel, or "" when the panel
// uses the canonical `<stage>-panel` default.
type PanelJudgeSpec struct {
	Panel   []JudgeSpec
	Fold    string
	Outcome string
}

// FoldCustom is the Fold value of a raw author fold.
const FoldCustom = "custom"

// AnyJudgeSpec is the pure-data summary of a judge SLOT — a JudgeSpec or a
// PanelJudgeSpec.
type AnyJudgeSpec interface {
	isJudgeSpec()
}

func (JudgeSpec) isJudgeSpec()      {}
func (PanelJudgeSpec) isJudgeSpec() {}

// JudgeSpecOf projects the introspectable facet off a Judge. Defensive on
// the outcome: lints run over UNVALIDATED configs, so it never fails.
func JudgeSpecOf(judge *Judge) JudgeSpec {
	if judge == nil {
		return JudgeSpec{}
	}
	return JudgeSpec{Skill: judge.Skill, Prompt: judge.Prompt.IsSet(), Outcome: outcomeName(judge.Outcome)}
}

// PanelSpecOf projects the introspectable facet off a panel — member
// summaries + fold flavour + verdict channel. Defensive like JudgeSpecOf.
func PanelSpecOf(p *PanelJudge) PanelJudgeSpec {
	spec := PanelJudgeSpec{Panel: []JudgeSpec{}, Fold: FoldCustom}
	if p == nil {
		return spec
	}
	for _, m := range p.Members {
		spec.Panel = append(spec.Panel, JudgeSpecOf(m))
	}
	if name, ok := CanonicalFoldNameOf(p.Fold); ok {
		spec.Fold = string(name)
	}
	spec.Outcome = outcomeName(p.Outcome)
	return spec
}

// JudgeSlotSpecOf projects a judge SLOT — a panel through PanelSpecOf, a
// single judge through JudgeSpecOf.
func JudgeSlotSpecOf(slot AnyJudge) AnyJudgeSpec {
	switch j := slot.(type) {
	case *PanelJudge:
		return PanelSpecOf(j)
	case *Judge:
		return JudgeSpecOf(j)
	}
	return JudgeSpec{}
}

// LoopSpec is the pure-data projection of a LoopDef — what DescribeFlow,
// the preview and lints consume. Judge summarises an assess dispatch without
// exposing functions.
type LoopSpec struct {
	Kind   LoopKind
	Source string
	Unit   *UnitSelector
	Max    *int
	OnCap  CapPolicy
	Result ResultProjection
	Judge  AnyJudgeSpec
}

// LoopSpecOf projects the introspectable facet off a stage's loop, or nil
// for non-loop stages.
func LoopSpecOf(loop LoopDef) *LoopSpec {
	if loop == nil {
		return nil
	}
	c := loop.Common()
	spec := &LoopSpec{Kind: loop.Kind(), Source: c.Source, Unit: c.Unit, Max: c.Max, OnCap: c.OnCap, Result: c.Result}
	if a, ok := loop.(*AssessLoop); ok {
		spec.Judge = JudgeSlotSpecOf(a.Judge)
	}
	return spec
}
