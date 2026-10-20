package workflow

// Skill contract data model. A contract declares what a skill consumes and
// produces, as data — the keystone that lets an agent reason about
// composition without scanning prose. The contract is the SKILL's property,
// registered into this framework via the skill-contracts registry; this
// package never reads skill files or parses YAML.
//
// Three field tiers: the data channel (Consumes.Data / Produces.Data, JSON
// Schema) is the ONLY channel the framework adjudicates; framework-native
// structure (Produces.Kind, Consumes.Reads) is typed because the framework
// owns those concepts; opaque consumer metadata (Meta) is stored and surfaced
// but NEVER interpreted.

// ContractSource records where a contract came from, in descending
// authority: "declared" (the skill's own frontmatter) or "harvested" (derived
// from how workflow stages use the skill). Absence is modelled by the skill
// not being in the registry.
type ContractSource string

const (
	ContractDeclared  ContractSource = "declared"
	ContractHarvested ContractSource = "harvested"
)

// ConsumesReadSpec is a named-channel reference a skill reads. The reference
// is recorded but not schema-checked; per-read domain tags live in Meta.
type ConsumesReadSpec struct {
	Meta map[string]any `json:"meta,omitempty"`
}

// ConsumesSpec is what a skill consumes: an optional typed input Data schema,
// optional named-channel references, and an opaque Meta bag.
type ConsumesSpec struct {
	Data  JsonSchemaObject            `json:"data,omitempty"`
	Reads map[string]ConsumesReadSpec `json:"reads,omitempty"`
	Meta  map[string]any              `json:"meta,omitempty"`
}

// ProducesSpec is what a skill produces: the stage Kind ("produces" |
// "side-effect" for harvested contracts; free-form for declared ones, since
// frontmatter is untrusted), an optional typed output Data schema, and an
// opaque Meta bag.
type ProducesSpec struct {
	Kind string           `json:"kind"`
	Data JsonSchemaObject `json:"data,omitempty"`
	Meta map[string]any   `json:"meta,omitempty"`
}

// SkillContract is a skill's contract. Consumes / Produces are each optional
// (a skill may be a pure chat side-effect). Source records provenance.
type SkillContract struct {
	Consumes *ConsumesSpec  `json:"consumes,omitempty"`
	Produces *ProducesSpec  `json:"produces,omitempty"`
	Source   ContractSource `json:"source"`
}

// SkillContractMap is the registry shape: resolved post-alias skill name →
// contract. Treat it as read-only.
type SkillContractMap map[string]SkillContract

// CompositionComparator is a consumer-supplied adjudicator for one named
// channel's Meta compatibility. The framework INVOKES it but never reads
// inside Meta. Conservative by contract: return ok whenever there is nothing
// to compare, so a missing tag degrades, never halts.
type CompositionComparator func(produces ProducesSpec, consumes ConsumesSpec, channelName string) SchemaCompatResult
