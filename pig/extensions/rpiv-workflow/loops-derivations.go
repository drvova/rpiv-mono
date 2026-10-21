package workflow

// Per-stage derivations — the ONE consults the live + resume paths key off:
// EffectiveLoopOf (loop-vs-verify shape), FreezesEntryArgsOf (the round-0
// producer-arg rule), JudgeSlotOf (the raw judge slot), ForEachJudgeChannel
// (the verdict-channel walk).

// FreezesEntryArgsOf is THE round-0-entry-arg predicate — one spelling of
// "which kinds freeze a producer arg", consulted by the live and resume
// entry sites so they can never drift on the rule.
func FreezesEntryArgsOf(loop LoopDef) bool {
	return LoopDefaults[loop.Kind()].FreezesEntryArgs
}

// EffectiveLoopOf is THE loop-or-verify consult — the one derivation of
// "does this stage run through the loop driver, and with what spec": an
// explicit Loop wins; a Verify synthesises; neither ⇒ nil.
func EffectiveLoopOf(def StageDef) LoopDef {
	if loopPresent(def.Loop) {
		return def.Loop
	}
	if def.Verify != nil {
		return SynthesizeVerifyLoop(def.Verify)
	}
	return nil
}

// loopPresent reports whether a loop slot carries a loop — false for a nil
// interface AND for a typed-nil pointer (the zero a hand-rolled literal can
// hold), so no consumer dereferences nothing through a passing `!= nil`.
func loopPresent(loop LoopDef) bool {
	switch l := loop.(type) {
	case nil:
		return false
	case *FanoutLoop:
		return l != nil
	case *IterateLoop:
		return l != nil
	case *AssessLoop:
		return l != nil
	}
	return true
}

// JudgeSlotOf is THE judge-slot-of-stage derivation — the RAW AnyJudge a
// stage carries (assess loop judge, else verify judge), before any member
// collapse. nil for stages without a judge.
func JudgeSlotOf(stage StageDef) AnyJudge {
	if a, ok := stage.Loop.(*AssessLoop); ok && a != nil {
		return a.Judge
	}
	if stage.Verify != nil {
		return stage.Verify.Judge
	}
	return nil
}

// ForEachJudgeChannel enumerates the verdict channels a stage's judge slot
// publishes, each with the skill that SIGNS it — "" for the manufactured
// panel-fold channel, which is data-only and carries no producer contract.
// THE single judge-publisher walk shared by the load reachability scan and
// the contract adjudicator's publisher index.
func ForEachJudgeChannel(stage StageDef, name string, visit func(channel, signingSkill string)) {
	slot := JudgeSlotOf(stage)
	if slot == nil {
		return
	}
	switch j := slot.(type) {
	case *PanelJudge:
		for _, m := range j.Members {
			if m != nil && m.Outcome != nil && m.Outcome.Name != "" {
				visit(m.Outcome.Name, m.Skill)
			}
		}
		visit(PanelVerdictChannel(j, name), "") // manufactured fold — unsigned
	case *Judge:
		if j.Outcome != nil && j.Outcome.Name != "" {
			visit(j.Outcome.Name, j.Skill)
		}
	}
}
