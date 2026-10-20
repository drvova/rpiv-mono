package workflow

// Stage-identity derivations — the pure projections off a StageDef that the
// runtime and the load-time validators must agree on (effective skill,
// publish name, dispatch-vs-body classification, effective output schema).

// ResolvePublishName resolves the state.named key a produces stage appends
// its Output onto: stage.Outcome.Name, else the stage's record key.
func ResolvePublishName(def StageDef, stageName string) string {
	if def.Outcome != nil && def.Outcome.Name != "" {
		return def.Outcome.Name
	}
	return stageName
}

// ActsPublishName is the state.named key a SIDE-EFFECT stage publishes its
// Output onto, or "" for the silent majority: acts stages publish ONLY under
// an EXPLICIT outcome name — never the record-key fallback produces stages get.
func ActsPublishName(def StageDef) string {
	if def.Kind != KindProduces && def.Outcome != nil {
		return def.Outcome.Name
	}
	return ""
}

// ResolveSkill resolves a stage's effective skill — the contract-registry
// key: def.Skill, else the record key.
func ResolveSkill(def StageDef, stageName string) string {
	if def.Skill != "" {
		return def.Skill
	}
	return stageName
}

// EffectiveOutputSchemaOf resolves the schema a produces stage's output is
// validated against: the stage's own OutputSchema if it declares one, else
// the dispatched skill's contract produces.data (from the registered-contract
// registry). Degrades like the input-side mirror: a non-object produces.data
// is treated as absent. nil when neither source supplies a schema.
func EffectiveOutputSchemaOf(def StageDef, stageName string, skillContracts SkillContractMap) StageSchema {
	if def.OutputSchema != nil {
		return def.OutputSchema
	}
	contract, ok := skillContracts[ResolveSkill(def, stageName)]
	if !ok || contract.Produces == nil {
		return nil
	}
	producesData, ok := IsJsonSchemaObject(contract.Produces.Data)
	if !ok {
		return nil
	}
	return JsonSchemaToStandard(producesData)
}

// IsDispatchingStage reports whether a stage dispatches a `/skill:<name>`:
// exactly when it carries neither a Run body nor a Prompt body. Fanout /
// iterate stages carry neither, so they ARE dispatching stages. The shared
// predicate for every site that treats ResolveSkill's result as a REAL skill
// identity (alias remap, contract harvest, validator lookups).
func IsDispatchingStage(stage StageDef) bool {
	return stage.Run == nil && !stage.Prompt.IsSet()
}
