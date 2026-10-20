package workflow

import "testing"

func TestResolveSkill(t *testing.T) {
	cases := []struct {
		name  string
		def   StageDef
		stage string
		want  string
	}{
		{"stage name when no skill", StageDef{Kind: KindProduces}, "blueprint", "blueprint"},
		{"explicit skill", StageDef{Kind: KindProduces, Skill: "custom-skill"}, "blueprint", "custom-skill"},
		{"skill equal to stage name", StageDef{Kind: KindSideEffect, Skill: "blueprint"}, "blueprint", "blueprint"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ResolveSkill(c.def, c.stage); got != c.want {
				t.Fatalf("got %q", got)
			}
		})
	}
}

func TestPublishNames(t *testing.T) {
	if ResolvePublishName(StageDef{Kind: KindProduces}, "blueprint") != "blueprint" {
		t.Fatal("record key fallback")
	}
	if ResolvePublishName(StageDef{Kind: KindProduces, Outcome: &Outcome{Name: "plans"}}, "blueprint") != "plans" {
		t.Fatal("outcome name")
	}
	if ActsPublishName(StageDef{Kind: KindSideEffect}) != "" || ActsPublishName(StageDef{Kind: KindProduces, Outcome: &Outcome{Name: "x"}}) != "" {
		t.Fatal("acts stays silent without an explicit name / produces excluded")
	}
	if ActsPublishName(StageDef{Kind: KindSideEffect, Outcome: &Outcome{Name: "remediation"}}) != "remediation" {
		t.Fatal("explicit acts outcome name")
	}
}

func TestEffectiveOutputSchemaOf(t *testing.T) {
	own := TypeboxSchema(T.String())
	if EffectiveOutputSchemaOf(StageDef{OutputSchema: own}, "s", nil) != own {
		t.Fatal("stage schema has precedence")
	}
	contracts := SkillContractMap{"research": {Produces: &ProducesSpec{Kind: "produces", Data: Schema{"type": "object", "required": []string{"n"}}}, Source: ContractDeclared}}
	got := EffectiveOutputSchemaOf(StageDef{}, "research", contracts)
	if got == nil {
		t.Fatal("contract schema expected")
	}
	if res, _ := got.Validate(t.Context(), map[string]any{}); res.OK() {
		t.Fatal("contract schema should reject a missing required field")
	}
	if EffectiveOutputSchemaOf(StageDef{}, "other", contracts) != nil || EffectiveOutputSchemaOf(StageDef{Skill: "none"}, "research", contracts) != nil {
		t.Fatal("absent contract degrades to nil")
	}
	if EffectiveOutputSchemaOf(StageDef{}, "x", SkillContractMap{"x": {Produces: &ProducesSpec{Kind: "produces"}}}) != nil {
		t.Fatal("no data degrades to nil")
	}
}
