package workflow

// Panel — N independent judges + a vote fold (the adversarial generalisation
// of a single judge). Construction + the canonical verdict surface live here;
// execution lands in the runner behind the one PanelMembers expander.

// PanelVerdictSchema is the canonical fold output shape — what Majority /
// All / Any emit and what a downstream DefineRoute / Gate / Match branches
// on. `agreement` (|majority| / N) is the first-class disagreement signal;
// `tie` flags an even split. A custom fold publishes the author's own
// schema instead (the XOR rule).
var PanelVerdictSchema = T.Object(map[string]Schema{
	"pass":      T.Boolean(),
	"votes":     T.Object(map[string]Schema{"pass": T.Integer(), "fail": T.Integer()}, Schema{"required": []string{"pass", "fail"}}),
	"agreement": T.Number(),
	"tie":       T.Boolean(),
}, Schema{"required": []string{"pass", "votes", "agreement", "tie"}})

// PanelVotes is the per-side tally of a PanelVerdict.
type PanelVotes struct {
	Pass int `json:"pass"`
	Fail int `json:"fail"`
}

// PanelVerdict is the canonical fold output (the static type of PanelVerdictSchema).
type PanelVerdict struct {
	Pass      bool       `json:"pass"`
	Votes     PanelVotes `json:"votes"`
	Agreement float64    `json:"agreement"`
	Tie       bool       `json:"tie"`
}

// PanelVerdictOutcome is the built-in named outcome the CANONICAL path
// publishes under. Its Name is a fallback: the live publish overrides it with
// the per-stage `<stage>-panel` channel. The collector is a no-op — the fold
// output is manufactured from the member verdicts, so nothing is collected.
var PanelVerdictOutcome = NamedOutcome{Name: "panel-verdict", Collector: NoopCollector}

// tally folds the members' per-member pred results into the canonical
// verdict shape; only `pass` differs per sugar fold, supplied by passWhen.
func tally(verdicts []Output, pred func(Output) bool, passWhen func(pass, fail int) bool) PanelVerdict {
	n := len(verdicts)
	pass := 0
	for _, v := range verdicts {
		if pred(v) {
			pass++
		}
	}
	fail := n - pass
	agreement := 0.0
	if n > 0 {
		agreement = float64(max(pass, fail)) / float64(n)
	}
	return PanelVerdict{Pass: passWhen(pass, fail), Votes: PanelVotes{Pass: pass, Fail: fail}, Agreement: agreement, Tie: pass == fail}
}

func sugarFold(name CanonicalFoldName, pred func(Output) bool, passWhen func(pass, fail int) bool) *FoldFn {
	return BrandCanonicalFold(name, NewFold(func(verdicts []Output) (any, error) {
		return tally(verdicts, pred, passWhen), nil
	}))
}

// Majority is the sugar fold: the panel passes iff a STRICT majority of
// members pass pred (an even split is a tie ⇒ fail). Branded canonical, so
// pairing it with an explicit outcome is a construction error.
func Majority(pred func(Output) bool) *FoldFn {
	return sugarFold(FoldMajority, pred, func(pass, fail int) bool { return pass > fail })
}

// All is the unanimous sugar fold: the panel passes iff EVERY member passes pred.
func All(pred func(Output) bool) *FoldFn {
	return sugarFold(FoldAll, pred, func(pass, fail int) bool { return pass > 0 && fail == 0 })
}

// Any is the veto/rescue sugar fold: the panel passes iff ANY member passes pred.
func Any(pred func(Output) bool) *FoldFn {
	return sugarFold(FoldAny, pred, func(pass, _ int) bool { return pass > 0 })
}

// panelCandidate normalises the values PanelShapeIssues accepts.
type panelCandidate struct {
	members      []any
	membersValid bool
	fold         *FoldFn
	foldIsFn     bool
	hasOutcome   bool
	outcomeName  string
}

func panelCandidateOf(candidate any) (panelCandidate, bool) {
	switch p := candidate.(type) {
	case nil:
		return panelCandidate{}, false
	case *PanelJudge:
		if p == nil {
			return panelCandidate{}, false
		}
		c := panelCandidate{membersValid: p.Members != nil, fold: p.Fold, foldIsFn: p.Fold != nil, hasOutcome: p.Outcome != nil, outcomeName: outcomeName(p.Outcome)}
		for _, m := range p.Members {
			c.members = append(c.members, m)
		}
		return c, true
	case PanelJudge:
		return panelCandidateOf(&p)
	case map[string]any:
		if p == nil {
			return panelCandidate{}, false
		}
		c := panelCandidate{}
		switch members := p["members"].(type) {
		case []any:
			c.members, c.membersValid = members, true
		case []*Judge:
			c.membersValid = true
			for _, m := range members {
				c.members = append(c.members, m)
			}
		}
		c.fold, c.foldIsFn = p["fold"].(*FoldFn)
		if raw, ok := p["outcome"]; ok {
			c.hasOutcome = true
			switch o := raw.(type) {
			case *Outcome:
				c.outcomeName = outcomeName(o)
			case Outcome:
				c.outcomeName = o.Name
			case map[string]any:
				c.outcomeName, _ = o["name"].(string)
			}
		}
		return c, true
	}
	return panelCandidate{}, false
}

func isPanelValue(m any) bool {
	switch v := m.(type) {
	case *PanelJudge:
		return v != nil
	case PanelJudge:
		return true
	case map[string]any:
		kind, _ := v["kind"].(string)
		return kind == "panel"
	}
	return false
}

// PanelShapeIssues is the single rule source for the panel shape. Enforces: a
// non-empty members list, each member a VALID single judge with NO nesting, a
// fold, and the XOR rule (canonical sugar ⊕ outcome). Accepts *PanelJudge,
// PanelJudge, a map literal or nil.
func PanelShapeIssues(candidate any) []string {
	p, ok := panelCandidateOf(candidate)
	if !ok {
		return []string{"a panel object is required"}
	}
	issues := []string{}
	if !p.membersValid || len(p.members) == 0 {
		issues = append(issues, "panel.members must be a non-empty array of judges")
	} else {
		for _, m := range p.members {
			if isPanelValue(m) {
				issues = append(issues, "panel.members may not nest another panel — members are single judges (skill or prompt)")
				continue
			}
			for _, issue := range JudgeShapeIssues(m) {
				issues = append(issues, "panel member: "+issue)
			}
		}
	}
	if !p.foldIsFn {
		issues = append(issues, "panel.fold must be a function reducing the member verdicts to the panel's decision")
	}
	// The XOR rule — a sugar fold OWNS the canonical verdict (no outcome); a
	// raw fold REQUIRES an outcome to name + validate its channel. Never both.
	isSugar := p.foldIsFn && MarksCanonicalFold(p.fold)
	if isSugar && p.hasOutcome {
		issues = append(issues, "a canonical fold (majority/all/any) publishes the built-in PANEL_VERDICT — drop `outcome` (sugar ⊕ outcome)")
	}
	if p.foldIsFn && !isSugar && !p.hasOutcome {
		issues = append(issues, "a custom (raw) fold requires an `outcome` naming + validating its verdict channel (raw ⊕ outcome)")
	}
	if p.hasOutcome && p.outcomeName == "" {
		issues = append(issues, "panel.outcome must carry a `name` so the folded verdict publishes to its own named channel")
	}
	return issues
}

// PanelSpec is the authoring input for Panel.
type PanelSpec struct {
	Members []*Judge
	Fold    *FoldFn
	Outcome *NamedOutcome
}

// Panel promotes a panel literal to a validated *PanelJudge, failing on the
// first shape issue so a Panel-authored value is correct by construction.
func Panel(spec PanelSpec) (*PanelJudge, error) {
	candidate := &PanelJudge{Members: spec.Members, Fold: spec.Fold, Outcome: spec.Outcome}
	if err := AssertShape("panel", PanelShapeIssues(candidate)); err != nil {
		return nil, err
	}
	return candidate, nil
}

// PanelVerdictChannel is the channel a panel's FOLDED verdict publishes to —
// the author's outcome name on the custom path, or `<stage>-panel` on the
// canonical path. ONE definition so the load gate and the panel-close publish
// can never drift. Member verdicts publish to their OWN outcome channels.
func PanelVerdictChannel(p *PanelJudge, stageName string) string {
	if p.Outcome != nil && p.Outcome.Name != "" {
		return p.Outcome.Name
	}
	return stageName + "-panel"
}

// PanelVerdictDef is the synthetic produces def the panel-close publish lands
// the FOLDED verdict under — the twin of the judge stage def a single MEMBER
// runs on. The channel is resolved ONCE through PanelVerdictChannel and baked
// into the outcome name, so distinct panel stages never collide. ONE
// construction site shared by the live publish and the resume fold.
func PanelVerdictDef(p *PanelJudge, stageName string) StageDef {
	base := PanelVerdictOutcome
	if p.Outcome != nil {
		base = *p.Outcome
	}
	outcome := base
	outcome.Name = PanelVerdictChannel(p, stageName)
	return StageDef{Kind: KindProduces, Outcome: &outcome, SessionPolicy: SessionFresh}
}
