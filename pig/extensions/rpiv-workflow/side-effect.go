package workflow

// NoopCollector is the framework's "no artifacts produced" primitive: it
// always returns ok with zero artifacts, never fatal. The chain semantics
// then inherit the upstream artifact list forward, so an action skill between
// two produces skills needs no collector of its own. It lives in the root
// package because the panel vocabulary (PanelVerdictOutcome) is built on it;
// the outcomes package re-exports it.
var NoopCollector ArtifactCollector = CollectorFunc(func(CollectContext) (CollectResult, error) {
	return CollectOK([]Artifact{}), nil
})

// SideEffectOutcome is the default outcome for side-effect stages: the
// NoopCollector and no parser, so the output's data is the empty list and
// kind is "artifacts".
var SideEffectOutcome = Outcome{Collector: NoopCollector}
