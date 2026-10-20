package workflow

import "context"

// Judge — the first-class model-judge concept. A Judge names a dispatchable
// grading session: a skill (`/skill:<judge.skill> <producerHandle>`, the
// latest producer artifact auto-injected as the input handle) or a raw prompt
// (the author embeds the handle/output themselves). Exactly one of Skill /
// Prompt is the dispatch discriminator — enforced at CONSTRUCTION by NewJudge.
// A judge session runs as a produces-kind session so its verdict is VALIDATED
// by Outcome and published into its own state.named[outcome.name] channel.
// What the verdict DECIDES lives on the consuming site (assess / verify /
// panel), which is what makes the concept reusable across all three.

// JudgeContext is handed to a dynamic judge prompt.
type JudgeContext struct {
	Cwd string
	// Output is the latest producer output (also the session's input handle).
	Output Output
	// EntryArtifact is the frozen stage-entry primary, referenceable.
	EntryArtifact *Artifact
	State         RunView
	Round         int
}

// JudgePromptFn is a judge's dynamic prompt — receives the producer output + round context.
type JudgePromptFn func(ctx context.Context, jc JudgeContext) (string, error)

// JudgePrompt is a judge's prompt — a static string or a JudgePromptFn. The
// zero value means "no prompt dispatch".
type JudgePrompt struct {
	text string
	fn   JudgePromptFn
	set  bool
}

// StaticJudgePrompt builds a static judge prompt.
func StaticJudgePrompt(text string) JudgePrompt { return JudgePrompt{text: text, set: true} }

// DynamicJudgePrompt builds a judge prompt from a function.
func DynamicJudgePrompt(fn JudgePromptFn) JudgePrompt { return JudgePrompt{fn: fn, set: true} }

// IsSet reports whether the judge carries a prompt dispatch.
func (p JudgePrompt) IsSet() bool { return p.set }

// IsDynamic reports whether the prompt is function-valued.
func (p JudgePrompt) IsDynamic() bool { return p.fn != nil }

// Text is the static text (empty for a dynamic prompt).
func (p JudgePrompt) Text() string { return p.text }

// NamedOutcome is an Outcome whose Name is REQUIRED — a judge's verdict must
// publish to its own dedicated channel.
type NamedOutcome = Outcome

// Judge is the reusable judge shape: Skill XOR Prompt as the dispatch, plus
// the named verdict Outcome. The collector MUST materialise at least one
// artifact; a judge whose collector returns zero artifacts is a fatal halt.
// Authored via NewJudge; hand-rolled values are re-checked at load time
// through the same JudgeShapeIssues rule source.
type Judge struct {
	Skill   string
	Prompt  JudgePrompt
	Outcome *NamedOutcome
}

// FoldFn reduces the N member verdicts of a PanelJudge to the panel's
// decision DATA — any, because the shape is path-dependent (canonical
// PanelVerdict vs a custom author schema), validated downstream by the
// panel's outcome. A fold is pure: live + resume both recompute it from the
// durable member rows. It carries SymbolMarks so the sugar constructors can
// brand it canonical.
type FoldFn struct {
	SymbolMarks
	fn func(verdicts []Output) (any, error)
}

// NewFold wraps a fold function.
func NewFold(fn func(verdicts []Output) (any, error)) *FoldFn { return &FoldFn{fn: fn} }

// Fold runs the fold over the member verdicts.
func (f *FoldFn) Fold(verdicts []Output) (any, error) { return f.fn(verdicts) }

// PanelJudge is N independent judges plus a vote Fold — the adversarial
// generalisation of a single Judge. Members are full judges (no nesting).
// Outcome is the canonical-vs-custom switch: nil ⇒ the built-in PanelVerdict
// shape on the `<stage>-panel` channel (sugar fold); set ⇒ the author's
// schema + channel (raw fold). Never both — the XOR is enforced by Panel.
type PanelJudge struct {
	Members []*Judge
	Fold    *FoldFn
	Outcome *NamedOutcome
}

// AnyJudge is the slot type every judge SITE accepts — a *Judge OR a
// *PanelJudge. A single judge is the panel of one.
type AnyJudge interface {
	isAnyJudge()
}

func (*Judge) isAnyJudge()      {}
func (*PanelJudge) isAnyJudge() {}

// IsPanel discriminates a PanelJudge from a single Judge.
func IsPanel(j AnyJudge) bool {
	p, ok := j.(*PanelJudge)
	return ok && p != nil
}

// PanelMembers is THE expander — the single place a judge value becomes its
// member list: a panel yields its members; a single judge yields itself.
func PanelMembers(j AnyJudge) []*Judge {
	switch v := j.(type) {
	case *PanelJudge:
		return v.Members
	case *Judge:
		return []*Judge{v}
	}
	return nil
}

// CanonicalFoldName names a sugar-fold constructor.
type CanonicalFoldName string

const (
	FoldMajority CanonicalFoldName = "majority"
	FoldAll      CanonicalFoldName = "all"
	FoldAny      CanonicalFoldName = "any"
)

// CanonicalFold is the brand carrying the SUGAR-fold name so consumers can
// tell a canonical fold from a raw author closure (the structural signal
// behind the sugar ⊕ outcome XOR) and report which fold it is.
var CanonicalFold = SymbolFor("rpiv.workflow.canonicalFold")

// MarksCanonicalFold is true iff fold was produced by a sugar constructor.
func MarksCanonicalFold(fold *FoldFn) bool {
	_, ok := CanonicalFoldNameOf(fold)
	return ok
}

// CanonicalFoldNameOf is the sugar constructor that produced fold, or false
// for a raw author closure.
func CanonicalFoldNameOf(fold *FoldFn) (CanonicalFoldName, bool) {
	if fold == nil {
		return "", false
	}
	return ReadSymbol[CanonicalFoldName](fold, CanonicalFold)
}

// BrandCanonicalFold attaches the CanonicalFold brand and returns the fold —
// the ONE write site (the sugar constructors).
func BrandCanonicalFold(name CanonicalFoldName, fold *FoldFn) *FoldFn {
	MarkSymbol(fold, CanonicalFold, name)
	return fold
}

// judgeCandidate normalises the values JudgeShapeIssues accepts.
type judgeCandidate struct {
	present     bool
	hasSkill    bool
	hasPrompt   bool
	outcomeName string
}

func judgeCandidateOf(candidate any) (judgeCandidate, bool) {
	switch j := candidate.(type) {
	case nil:
		return judgeCandidate{}, false
	case *Judge:
		if j == nil {
			return judgeCandidate{}, false
		}
		return judgeCandidate{present: true, hasSkill: j.Skill != "", hasPrompt: j.Prompt.IsSet(), outcomeName: outcomeName(j.Outcome)}, true
	case Judge:
		return judgeCandidate{present: true, hasSkill: j.Skill != "", hasPrompt: j.Prompt.IsSet(), outcomeName: outcomeName(j.Outcome)}, true
	case map[string]any:
		if j == nil {
			return judgeCandidate{}, false
		}
		c := judgeCandidate{present: true}
		_, c.hasSkill = j["skill"]
		_, c.hasPrompt = j["prompt"]
		if o, ok := j["outcome"].(map[string]any); ok {
			c.outcomeName, _ = o["name"].(string)
		} else if o, ok := j["outcome"].(*Outcome); ok {
			c.outcomeName = outcomeName(o)
		} else if o, ok := j["outcome"].(Outcome); ok {
			c.outcomeName = o.Name
		}
		return c, true
	}
	return judgeCandidate{}, false
}

func outcomeName(o *Outcome) string {
	if o == nil {
		return ""
	}
	return o.Name
}

// JudgeShapeIssues is the single rule source for the judge shape. It returns
// human-readable violations (empty = valid); NewJudge fails on the first, the
// validator maps each to a load issue. Accepts *Judge, Judge, a map literal
// or nil.
func JudgeShapeIssues(candidate any) []string {
	j, ok := judgeCandidateOf(candidate)
	if !ok {
		return []string{"a judge object is required"}
	}
	issues := []string{}
	if j.outcomeName == "" {
		issues = append(issues, "judge.outcome must carry a `name` so the verdict publishes to its own named channel")
	}
	if j.hasSkill && j.hasPrompt {
		issues = append(issues, "judge sets both `skill` and `prompt` — choose one dispatch (skill XOR prompt)")
	}
	if !j.hasSkill && !j.hasPrompt {
		issues = append(issues, "judge sets neither `skill` nor `prompt` — one is required to dispatch the judge")
	}
	return issues
}

// AssertShape is THE collect-then-fail-first idiom shared by the judge /
// assess / verify / panel factories: the error `<ctor>(): <first issue>` iff
// the rule source found any violation (the rest are deliberately discarded —
// one actionable error at the authoring site).
func AssertShape(ctor string, issues []string) error {
	if len(issues) > 0 {
		return InvalidError(ctor+"()", issues[0])
	}
	return nil
}

// NewJudge promotes a judge literal to a validated *Judge — an identity
// passthrough that fails on an invalid shape, so a NewJudge-authored value is
// correct by construction.
func NewJudge(spec Judge) (*Judge, error) {
	if err := AssertShape("judge", JudgeShapeIssues(&spec)); err != nil {
		return nil, err
	}
	return &spec, nil
}

// ResolveJudgePrompt resolves a static or dynamic judge prompt — the ONE
// resolver for every judge dispatch site.
func ResolveJudgePrompt(ctx context.Context, prompt JudgePrompt, jc JudgeContext) (string, error) {
	if prompt.fn != nil {
		return prompt.fn(ctx, jc)
	}
	return prompt.text, nil
}
