package stitch

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func lines(ls ...string) string { return strings.Join(ls, "\n") }

// A two-phase synthesize-style plan (contract-level: prose bullets, no code).
var planDoc = lines(
	"---",
	"status: ready",
	"phase_count: 2",
	"phases:",
	`  - { n: 1, title: "First" }`,
	`  - { n: 2, title: "Second" }`,
	"tags: [plan, synthesized]",
	"---",
	"",
	"# Plan: demo",
	"",
	"## Synthesis Notes",
	"- seam wired between phase 1 and 2",
	"",
	"## Phase 1: First",
	"### Changes",
	"- `a.ts` — add foo",
	"### Success Criteria",
	"#### Automated Verification:",
	"- [ ] npm test",
	"",
	"## Phase 2: Second",
	"### Changes",
	"- `b.ts` — add bar",
	"### Success Criteria",
	"#### Automated Verification:",
	"- [ ] npm test",
	"",
)

// An elaboration doc whose body is one code-bearing `## Phase N:` section.
func elaboration(n int, title, code string) string {
	return lines(
		"---",
		"phase_n: "+itoa(n),
		`phase_title: "`+title+`"`,
		"status: ready",
		"tags: [elaboration]",
		"---",
		"",
		"## Phase "+itoa(n)+": "+title,
		"### Changes",
		"#### `x.ts`",
		"the implementation",
		"```ts",
		code,
		"```",
		"### Success Criteria",
		"#### Automated Verification:",
		"- [ ] npm test",
		"",
	)
}

// An elaboration mirroring the real `elaborate` output that stalled the build workflow: a `## ` heading
// buried inside a Find/Replace code fence (must NOT be read as a section boundary), plus a trailing
// `## Notes / Deferred` that is an H2 SIBLING of `## Phase N:` (must be owned by the phase, not
// preserved as an orphan).
func elaborationWithTrailers(n int, title, code string) string {
	return lines(
		"---",
		"phase_n: "+itoa(n),
		"status: ready",
		"---",
		"",
		"## Phase "+itoa(n)+": "+title,
		"### Changes",
		"#### `x.ts`",
		"```ts",
		code,
		"```",
		"#### `doc.md`",
		"Find:",
		"````markdown",
		"## Substitution Order (byte-equivalent)",
		"1. positional",
		"````",
		"Replace with:",
		"````markdown",
		"## Substitution Order (byte-equivalent)",
		"0. mask",
		"1. positional",
		"````",
		"### Success Criteria",
		"#### Automated Verification:",
		"- [ ] npm test",
		"",
		"## Notes / Deferred",
		"- deferred note for phase "+itoa(n),
		"",
	)
}

// The authored shape: canonical heading + intro prose + whole-tree gate checkboxes (mirrors a real
// synthesized plan's trailing block bytes).
var wpvBlock = lines(
	"## Whole-Plan Verification",
	"",
	"Per-phase `Automated Verification` above is write-scoped to each phase's `files:` set; these whole-tree commands are the final block, owned by `validate` — run once all phases have landed:",
	"",
	"- [ ] `npm run check` exits 0 — Biome (`--write --error-on-warnings`) + `tsc --noEmit -p tsconfig.base.json`",
	"- [ ] `npm test` exits 0 — the single root Vitest runner over `packages/*/**/*.test.ts`",
	"",
)

// A suffix-form heading — the same section under the suffix-tolerant grammar.
var wpvSuffixBlock = lines("## Whole-Plan Verification (owned by validate)", "", "- [ ] `npm test` exits 0", "")

// A plan whose last-phase span carries a trailing authored WPV block; the Synthesis Notes line
// optionally carries the canonical reference phrase.
func planWithAuthoredWPV(block string, withReference bool) string {
	notes := "- seam wired between phase 1 and 2"
	if withReference {
		notes = "- whole-tree gates are collected into the final `## Whole-Plan Verification` block owned by validate"
	}
	return lines(
		"---",
		"status: ready",
		"phase_count: 2",
		"phases:",
		`  - { n: 1, title: "First" }`,
		`  - { n: 2, title: "Second" }`,
		"tags: [plan, synthesized]",
		"---",
		"",
		"# Plan: demo",
		"",
		"## Synthesis Notes",
		notes,
		"",
		"## Phase 1: First",
		"### Changes",
		"- `a.ts` — add foo",
		"### Success Criteria",
		"#### Automated Verification:",
		"- [ ] npm test",
		"",
		"## Phase 2: Second",
		"### Changes",
		"- `b.ts` — add bar",
		"### Success Criteria",
		"#### Automated Verification:",
		"- [ ] npm test",
		"",
		block,
	)
}

// The no-block shape: the Synthesis Notes reference the canonical phrase, but no trailing block was
// authored.
var referenceOnlyPlan = lines(
	"---",
	"status: ready",
	"phase_count: 2",
	"phases:",
	`  - { n: 1, title: "First" }`,
	`  - { n: 2, title: "Second" }`,
	"tags: [plan, synthesized]",
	"---",
	"",
	"# Plan: demo",
	"",
	"## Synthesis Notes",
	"- Whole-plan gates deferred here: the repo-wide criteria are collected into the final `## Whole-Plan Verification` block owned by validate",
	"",
	"## Phase 1: First",
	"### Changes",
	"- `a.ts` — add foo",
	"### Success Criteria",
	"#### Automated Verification:",
	"- [ ] npm test",
	"",
	"## Phase 2: Second",
	"### Changes",
	"- `b.ts` — add bar",
	"### Success Criteria",
	"#### Automated Verification:",
	"- [ ] npm test",
	"",
)

// An elaboration whose Success Criteria carries explicit AV items.
func elaborationWithAV(n int, title string, av ...string) string {
	head := []string{
		"---",
		"phase_n: " + itoa(n),
		"status: ready",
		"---",
		"",
		"## Phase " + itoa(n) + ": " + title,
		"### Changes",
		"#### `x.ts`",
		"```ts",
		"export const v" + itoa(n) + " = " + itoa(n) + ";",
		"```",
		"### Success Criteria",
		"#### Automated Verification:",
	}
	return lines(append(append(head, av...), "")...)
}

// An elaboration that embeds a whole markdown file under a THREE-backtick fence while that file carries
// its own ``` blocks: the inner closer ends the outer fence early and the intended outer closer reopens
// one that never closes.
func elaborationWithLeakedFence(n int, title string) string {
	return lines(
		"---",
		"phase_n: "+itoa(n),
		"status: ready",
		"---",
		"",
		"## Phase "+itoa(n)+": "+title,
		"### Changes",
		"#### `guide.md`",
		"Add — the package guidance",
		"```markdown",
		"# guide",
		"```ts",
		"export const x = 1;",
		"```",
		"more prose",
		"```",
		"### Success Criteria",
		"#### Automated Verification:",
		"- [ ] npm test",
		"",
	)
}

func itoa(n int) string { return strconv.Itoa(n) }

// fixture lays out .rpiv/artifacts/{plans,elaborations} under a temp root with planDoc written.
type fixture struct {
	root, plansDir, elaborationsDir, planPath string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{
		root:            root,
		plansDir:        filepath.Join(root, ".rpiv", "artifacts", "plans"),
		elaborationsDir: filepath.Join(root, ".rpiv", "artifacts", "elaborations"),
	}
	f.planPath = filepath.Join(f.plansDir, "2026-06-24_demo.md")
	for _, dir := range []string{f.plansDir, f.elaborationsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f.writePlan(t, planDoc)
	return f
}

func (f *fixture) writePlan(t *testing.T, content string) {
	t.Helper()
	writeFile(t, f.planPath, content)
}

// elaborate writes an elaboration doc under its plan-paired name.
func (f *fixture) elaborate(t *testing.T, name, content string) {
	t.Helper()
	writeFile(t, filepath.Join(f.elaborationsDir, name), content)
}

func (f *fixture) readPlan(t *testing.T) string {
	t.Helper()
	got, err := os.ReadFile(f.planPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

func (f *fixture) clearElaborations(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir(f.elaborationsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if err := os.Remove(filepath.Join(f.elaborationsDir, e.Name())); err != nil {
			t.Fatal(err)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// run stitches the plan and returns the summary line; a failure is the CLI's exit 1.
func run(t *testing.T, planPath string) string {
	t.Helper()
	res, err := Stitch(planPath, Options{})
	if err != nil {
		t.Fatalf("Stitch: %v", err)
	}
	return res.Report()
}

// runFail stitches the plan expecting a refusal and returns the stderr line (the error's message).
func runFail(t *testing.T, planPath string) string {
	t.Helper()
	_, err := Stitch(planPath, Options{})
	if err == nil {
		t.Fatal("Stitch succeeded, want a refusal")
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("Stitch returned %T (%v), want *Error", err, err)
	}
	return err.Error()
}

func contains(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("missing %q in:\n%s", sub, s)
	}
}

func notContains(t *testing.T, s, sub string) {
	t.Helper()
	if strings.Contains(s, sub) {
		t.Errorf("unexpected %q in:\n%s", sub, s)
	}
}

// count returns the number of matches of a multiline-anchored pattern, like the TS suite's `/…/gm`.
func count(s, pattern string) int {
	return len(regexp.MustCompile("(?m)"+pattern).FindAllStringIndex(s, -1))
}

func expectCount(t *testing.T, s, pattern string, want int) {
	t.Helper()
	if got := count(s, pattern); got != want {
		t.Errorf("%d matches of %q, want %d in:\n%s", got, pattern, want, s)
	}
}

const phaseHeadingPattern = `^## Phase (\d+):`

// --- stitch-elaborations -------------------------------------------------------------------------------

func TestSplicesEveryPhasesElaborationIntoThePlan(t *testing.T) {
	f := newFixture(t)
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaboration(1, "First", "export const foo = 1;"))
	f.elaborate(t, "2026-06-24_demo__phase-2.md", elaboration(2, "Second", "export const bar = 2;"))

	out := run(t, f.planPath)
	stitched := f.readPlan(t)

	contains(t, out, "stitched 2/2 phases")
	contains(t, stitched, "export const foo = 1;")
	contains(t, stitched, "export const bar = 2;")
	// Both phase headings survive (the splice anchor is preserved 1:1).
	contains(t, stitched, "## Phase 1: First")
	contains(t, stitched, "## Phase 2: Second")
}

func TestPreservesFrontmatterAndNonPhaseSectionsVerbatim(t *testing.T) {
	f := newFixture(t)
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaboration(1, "First", "export const foo = 1;"))
	f.elaborate(t, "2026-06-24_demo__phase-2.md", elaboration(2, "Second", "export const bar = 2;"))

	run(t, f.planPath)
	stitched := f.readPlan(t)

	contains(t, stitched, "phase_count: 2")
	contains(t, stitched, "tags: [plan, synthesized]")
	contains(t, stitched, "## Synthesis Notes")
	contains(t, stitched, "- seam wired between phase 1 and 2")
	// The original contract-level bullets are gone — replaced by real code.
	notContains(t, stitched, "- `a.ts` — add foo")
}

func TestKeepsThePhaseCountHeadingCountDeriveInvariant(t *testing.T) {
	f := newFixture(t)
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaboration(1, "First", "export const foo = 1;"))
	f.elaborate(t, "2026-06-24_demo__phase-2.md", elaboration(2, "Second", "export const bar = 2;"))

	run(t, f.planPath)

	expectCount(t, f.readPlan(t), phaseHeadingPattern, 2)
}

func TestLeavesAPhaseWithNoElaborationAsIsAndReportsIt(t *testing.T) {
	f := newFixture(t)
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaboration(1, "First", "export const foo = 1;"))

	out := run(t, f.planPath)
	stitched := f.readPlan(t)

	contains(t, out, "stitched 1/2 phases")
	contains(t, out, "no elaboration for phase(s) 2")
	contains(t, stitched, "export const foo = 1;")
	// Phase 2's original contract bullet is untouched.
	contains(t, stitched, "- `b.ts` — add bar")
}

func TestIgnoresElaborationDocsBelongingToADifferentPlan(t *testing.T) {
	f := newFixture(t)
	f.elaborate(t, "some-other-plan__phase-1.md", elaboration(1, "First", "WRONG = true;"))
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaboration(1, "First", "export const foo = 1;"))
	f.elaborate(t, "2026-06-24_demo__phase-2.md", elaboration(2, "Second", "export const bar = 2;"))

	run(t, f.planPath)
	stitched := f.readPlan(t)

	notContains(t, stitched, "WRONG = true;")
	contains(t, stitched, "export const foo = 1;")
}

func TestTreatsAHeadingInsideACodeFenceAsPhaseContentNotASectionBoundary(t *testing.T) {
	f := newFixture(t)
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaborationWithTrailers(1, "First", "export const foo = 1;"))
	f.elaborate(t, "2026-06-24_demo__phase-2.md", elaborationWithTrailers(2, "Second", "export const bar = 2;"))

	run(t, f.planPath)
	stitched := f.readPlan(t)

	// The fenced Find/Replace blocks survive intact (2 phases × Find+Replace), not shredded into orphan
	// fragments with dangling Replace-with markers.
	if got := strings.Count(stitched, "## Substitution Order"); got != 4 {
		t.Errorf("%d Substitution Order lines, want 4", got)
	}
	// Exactly one Success Criteria and one Notes/Deferred per phase.
	expectCount(t, stitched, `^### Success Criteria$`, 2)
	expectCount(t, stitched, `^## Notes / Deferred$`, 2)
	expectCount(t, stitched, phaseHeadingPattern, 2)
}

func TestIsIdempotentReStitchingDoesNotAccumulateDuplicatePerPhaseTrailers(t *testing.T) {
	f := newFixture(t)
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaborationWithTrailers(1, "First", "export const foo = 1;"))
	f.elaborate(t, "2026-06-24_demo__phase-2.md", elaborationWithTrailers(2, "Second", "export const bar = 2;"))

	run(t, f.planPath)
	first := f.readPlan(t)
	run(t, f.planPath)
	second := f.readPlan(t)

	// Byte-identical: build's code-grade → code loop re-stitches every round; a non-idempotent stitch
	// grew the plan (tripled Success-Criteria / Notes blocks) until the backward-jump guard halted the
	// run.
	if second != first {
		t.Errorf("second stitch differs from the first:\n%s\n--- vs ---\n%s", second, first)
	}
	expectCount(t, second, `^### Success Criteria$`, 2)
	expectCount(t, second, `^## Notes / Deferred$`, 2)
	expectCount(t, second, phaseHeadingPattern, 2)
}

// The mixed-delimiter close pin: a bare `~~~` line CLOSES a backtick-opened fence under the stitch's
// length-only predicate (the same-char twins in extensions/rpiv-core would not). Phase 2 stays
// kept-original with such a fence; the boundary walk must exit at the `~~~` so the following real
// `## Phase 3: Third` heading still reads as a section boundary — the failure class a same-char
// tightening would recreate.
func TestClosesABacktickOpenedFenceAtABareTildeLineSoTheNextPhaseHeadingStillCounts(t *testing.T) {
	f := newFixture(t)
	f.writePlan(t, lines(
		"---",
		"status: ready",
		"phase_count: 3",
		"phases:",
		`  - { n: 1, title: "First" }`,
		`  - { n: 2, title: "Second" }`,
		`  - { n: 3, title: "Third" }`,
		"tags: [plan, synthesized]",
		"---",
		"",
		"# Plan: demo",
		"",
		"## Synthesis Notes",
		"- seam",
		"",
		"## Phase 1: First",
		"### Changes",
		"- `a.ts` — add foo",
		"",
		"## Phase 2: Second",
		"### Changes",
		"#### `doc.md`",
		"Find:",
		"```markdown",
		"fenced example line",
		"~~~",
		"",
		"## Phase 3: Third",
		"### Changes",
		"- `c.ts` — add baz",
		"",
	))
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaboration(1, "First", "export const foo = 1;"))
	f.elaborate(t, "2026-06-24_demo__phase-3.md", elaboration(3, "Third", "export const baz = 3;"))

	out := run(t, f.planPath)
	stitched := f.readPlan(t)

	// The `~~~` closed the backtick fence: all three headings were seen, phases 1 and 3 paired, phase 2
	// reported as kept-original.
	contains(t, out, "stitched 2/3 phases")
	contains(t, out, "no elaboration for phase(s) 2")
	expectCount(t, stitched, phaseHeadingPattern, 3)
	// Phase 3's elaboration spliced — and its original contract bullet replaced, proving the splice
	// landed at the real phase-3 boundary.
	contains(t, stitched, "export const baz = 3;")
	notContains(t, stitched, "- `c.ts` — add baz")
	// Phase 2's kept-original span carries the mixed-delimiter fence through verbatim — the example line
	// survives exactly once.
	expectCount(t, stitched, `^fenced example line$`, 1)
}

func TestFailsWhenNoMatchingElaborationDocsExist(t *testing.T) {
	f := newFixture(t)
	stderr := runFail(t, f.planPath)
	contains(t, stderr, "nothing to stitch")
	if stderr != `stitch-elaborations: no elaboration docs for "2026-06-24_demo" in `+f.elaborationsDir+
		" (expected 2026-06-24_demo__phase-<N>.md) — nothing to stitch" {
		t.Errorf("unexpected message: %s", stderr)
	}
}

func TestFailsWhenThePlanFileDoesNotExist(t *testing.T) {
	f := newFixture(t)
	missing := filepath.Join(f.plansDir, "missing.md")
	stderr := runFail(t, missing)
	contains(t, stderr, "plan not found")
	if stderr != "stitch-elaborations: plan not found: "+missing {
		t.Errorf("unexpected message: %s", stderr)
	}
}

// --- Whole-Plan Verification emission --------------------------------------------------------------------

func TestWPVReAppendsATrailingAuthoredBlockVerbatimEvenWhenTheLastPhaseIsElaborated(t *testing.T) {
	f := newFixture(t)
	f.writePlan(t, planWithAuthoredWPV(wpvBlock, true))
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaboration(1, "First", "export const foo = 1;"))
	f.elaborate(t, "2026-06-24_demo__phase-2.md", elaboration(2, "Second", "export const bar = 2;"))

	out := run(t, f.planPath)
	stitched := f.readPlan(t)

	// Authored wins over the concurrent Synthesis-Notes reference, and the block survives byte-for-byte
	// after the last phase — the swap drops the whole original last-phase span (heading to EOF), block
	// included.
	contains(t, out, "whole-plan verification: authored")
	if !strings.HasSuffix(stitched, wpvBlock) {
		t.Errorf("stitched plan does not end with the authored block:\n%s", stitched)
	}
	expectCount(t, stitched, `^## Whole-Plan Verification`, 1)
	// The notes were never consulted: no derived marker anywhere.
	notContains(t, stitched, "derived by stitch-elaborations")
}

func TestWPVReAppendsTheAuthoredBlockVerbatimWhenTheLastPhaseIsKeptOriginal(t *testing.T) {
	f := newFixture(t)
	f.writePlan(t, planWithAuthoredWPV(wpvBlock, false))
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaboration(1, "First", "export const foo = 1;"))

	out := run(t, f.planPath)
	stitched := f.readPlan(t)

	contains(t, out, "whole-plan verification: authored")
	if !strings.HasSuffix(stitched, wpvBlock) {
		t.Errorf("stitched plan does not end with the authored block:\n%s", stitched)
	}
	// Stripped from the kept section, re-appended as the tail: once, not twice.
	expectCount(t, stitched, `^## Whole-Plan Verification`, 1)
	contains(t, stitched, "## Phase 2: Second")
}

func TestWPVDerivesTheBlockFromTheFinalSectionsAVItemsWhenOnlyTheSynthesisNotesReferenceOne(t *testing.T) {
	f := newFixture(t)
	f.writePlan(t, referenceOnlyPlan)
	f.elaborate(t, "2026-06-24_demo__phase-1.md",
		elaborationWithAV(1, "First", "- [ ] `npx vitest run a.test.ts` — exits 0", "- [ ] npm test"))
	f.elaborate(t, "2026-06-24_demo__phase-2.md",
		elaborationWithAV(2, "Second", "- [ ] npm test", "- [ ] `npx vitest run b.test.ts` — exits 0"))

	out := run(t, f.planPath)
	stitched := f.readPlan(t)

	contains(t, out, "whole-plan verification: derived")
	// Anchor on the column-0 heading — the Synthesis-Notes reference line carries the phrase inside
	// backticks (mid-line, never a section).
	loc := regexp.MustCompile(`(?m)^## Whole-Plan Verification$`).FindStringIndex(stitched)
	if loc == nil {
		t.Fatalf("no column-0 WPV heading in:\n%s", stitched)
	}
	tail := stitched[loc[0]:]
	// Heading + provenance marker + intro, then the deduped items in plan order.
	if !strings.HasPrefix(tail, "## Whole-Plan Verification\n\n<!-- derived by stitch-elaborations") {
		t.Errorf("tail does not open with the derived block:\n%s", tail)
	}
	itemA := strings.Index(tail, "- [ ] `npx vitest run a.test.ts` — exits 0")
	itemNpm := strings.Index(tail, "- [ ] npm test")
	itemB := strings.Index(tail, "- [ ] `npx vitest run b.test.ts` — exits 0")
	if itemA <= -1 || itemNpm <= itemA || itemB <= itemNpm {
		t.Errorf("items out of plan order (a=%d npm=%d b=%d):\n%s", itemA, itemNpm, itemB, tail)
	}
	// The shared "- [ ] npm test" item is deduped across phases.
	expectCount(t, tail, `^- \[ \] npm test$`, 1)
}

func TestWPVReDerivesTheUpdatedBlockWhenAReElaboratedPhasesAVItemsChanged(t *testing.T) {
	f := newFixture(t)
	f.writePlan(t, referenceOnlyPlan)
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaborationWithAV(1, "First", "- [ ] npm test"))
	f.elaborate(t, "2026-06-24_demo__phase-2.md",
		elaborationWithAV(2, "Second", "- [ ] `npx vitest run b.test.ts` — exits 0"))
	run(t, f.planPath)
	first := f.readPlan(t)
	contains(t, first, "`npx vitest run b.test.ts`")

	// Phase 2 re-elaborated with different AV items.
	f.elaborate(t, "2026-06-24_demo__phase-2.md",
		elaborationWithAV(2, "Second", "- [ ] `npx vitest run c.test.ts` — exits 0"))
	out := run(t, f.planPath)
	second := f.readPlan(t)

	contains(t, out, "whole-plan verification: derived")
	contains(t, second, "`npx vitest run c.test.ts`")
	// The stale item was dropped with the stale derived block, not accumulated.
	notContains(t, second, "`npx vitest run b.test.ts`")
	expectCount(t, second, `^## Whole-Plan Verification`, 1)
}

func TestWPVNeverInventsABlockWithNoAuthoredBlockAndNoReference(t *testing.T) {
	f := newFixture(t)
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaboration(1, "First", "export const foo = 1;"))
	f.elaborate(t, "2026-06-24_demo__phase-2.md", elaboration(2, "Second", "export const bar = 2;"))

	// The fixture plan carries no block and no reference.
	out := run(t, f.planPath)
	stitched := f.readPlan(t)

	contains(t, out, "whole-plan verification: none")
	notContains(t, stitched, "## Whole-Plan Verification")
}

func TestWPVNeverInventsABlockWhenAReferenceHasZeroCollectibleAVItems(t *testing.T) {
	f := newFixture(t)
	f.writePlan(t, referenceOnlyPlan)
	// Both phases swapped for sections with an AV heading but no checkbox items.
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaborationWithAV(1, "First"))
	f.elaborate(t, "2026-06-24_demo__phase-2.md", elaborationWithAV(2, "Second"))

	out := run(t, f.planPath)
	stitched := f.readPlan(t)

	contains(t, out, "whole-plan verification: none (notes reference but no verification items)")
	// No column-0 section heading — the reference line's backticked phrase is prose, not a section.
	expectCount(t, stitched, `^## Whole-Plan Verification`, 0)
	notContains(t, stitched, "derived by stitch-elaborations")
}

func TestWPVIsIdempotentAcrossAllFourShapes(t *testing.T) {
	f := newFixture(t)
	shapes := []struct {
		plan          string
		elaborateLast bool
	}{
		{planWithAuthoredWPV(wpvBlock, true), true},
		{planWithAuthoredWPV(wpvBlock, false), false},
		{referenceOnlyPlan, true},
		{referenceOnlyPlan, false},
	}
	for i, shape := range shapes {
		f.clearElaborations(t)
		f.writePlan(t, shape.plan)
		f.elaborate(t, "2026-06-24_demo__phase-1.md", elaborationWithAV(1, "First", "- [ ] npm test"))
		if shape.elaborateLast {
			f.elaborate(t, "2026-06-24_demo__phase-2.md", elaborationWithAV(2, "Second", "- [ ] npm test"))
		}
		run(t, f.planPath)
		first := f.readPlan(t)
		expectCount(t, first, `^## Whole-Plan Verification`, 1)
		run(t, f.planPath)
		if second := f.readPlan(t); second != first {
			t.Errorf("shape %d: second stitch differs from the first:\n%s\n--- vs ---\n%s", i, second, first)
		}
	}
}

func TestWPVNeverTreatsAFencedHeadingAsTheSection(t *testing.T) {
	f := newFixture(t)
	f.writePlan(t, lines(
		"---",
		"status: ready",
		"phase_count: 2",
		"phases:",
		`  - { n: 1, title: "First" }`,
		`  - { n: 2, title: "Second" }`,
		"---",
		"",
		"# Plan: demo",
		"",
		"## Synthesis Notes",
		"- seam",
		"",
		"## Phase 1: First",
		"### Changes",
		"### Success Criteria",
		"#### Automated Verification:",
		"- [ ] npm test",
		"",
		"## Phase 2: Second",
		"### Changes",
		"#### `doc.md`",
		"Find:",
		"```markdown",
		"## Whole-Plan Verification",
		"(fenced example — never a section)",
		"```",
		"### Success Criteria",
		"#### Automated Verification:",
		"- [ ] npm test",
		"",
	))
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaboration(1, "First", "export const foo = 1;"))

	out := run(t, f.planPath)
	stitched := f.readPlan(t)

	// The fenced line survives verbatim in the kept phase-2 section but is never extracted, never
	// emitted.
	contains(t, stitched, "## Whole-Plan Verification\n(fenced example — never a section)")
	contains(t, out, "whole-plan verification: none")
}

func TestWPVSuppressesTheAppendWhenABlockIsAlreadyPresentMidDocument(t *testing.T) {
	f := newFixture(t)
	f.writePlan(t, lines(
		"---",
		"status: ready",
		"phase_count: 2",
		"phases:",
		`  - { n: 1, title: "First" }`,
		`  - { n: 2, title: "Second" }`,
		"---",
		"",
		"# Plan: demo",
		"",
		"## Synthesis Notes",
		"- whole-tree gates are collected into the final `## Whole-Plan Verification` block owned by validate",
		"",
		"## Phase 1: First",
		"### Changes",
		"### Success Criteria",
		"#### Automated Verification:",
		"- [ ] npm test",
		"",
		"## Whole-Plan Verification",
		"",
		"A degenerate mid-document block (phase 1's span).",
		"",
		"## Phase 2: Second",
		"### Changes",
		"### Success Criteria",
		"#### Automated Verification:",
		"- [ ] npm test",
		"",
	))
	f.elaborate(t, "2026-06-24_demo__phase-2.md", elaboration(2, "Second", "export const bar = 2;"))

	out := run(t, f.planPath)
	stitched := f.readPlan(t)

	// Phase 1 kept-original carries the mid-document block into the final composition → the derived
	// candidate is suppressed with the note.
	contains(t, out, "whole-plan verification: none (block already present)")
	expectCount(t, stitched, `^## Whole-Plan Verification`, 1)
	contains(t, stitched, "A degenerate mid-document block")
	notContains(t, stitched, "derived by stitch-elaborations")
}

func TestWPVKeepsThePhaseCountHeadingCountDeriveInvariantWithAWPVTail(t *testing.T) {
	f := newFixture(t)
	f.writePlan(t, planWithAuthoredWPV(wpvBlock, false))
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaboration(1, "First", "export const foo = 1;"))
	f.elaborate(t, "2026-06-24_demo__phase-2.md", elaboration(2, "Second", "export const bar = 2;"))

	run(t, f.planPath)
	stitched := f.readPlan(t)

	// The WPV heading is not a phase heading — the count is unchanged.
	expectCount(t, stitched, phaseHeadingPattern, 2)
	expectCount(t, stitched, `^## Whole-Plan Verification`, 1)
}

func TestWPVCarriesAnAuthoredBlocksBulletShapedProseVerbatimAndKeepsTheFrontmatterByteIdentical(t *testing.T) {
	f := newFixture(t)
	bullet := "- **Scope addition, flagged: `wpv/x.ts`.** bookkeeping prose with a second `token`."
	plan := lines(
		"---",
		"status: ready",
		"phase_count: 2",
		"phases:",
		`  - { n: 1, title: "First", files: ["src/a.ts"] }`,
		`  - { n: 2, title: "Second" }`,
		"---",
		"",
		"# Plan: demo",
		"",
		"## Synthesis Notes",
		"- seam",
		"",
		"## Phase 1: First",
		"### Changes",
		"### Success Criteria",
		"#### Automated Verification:",
		"- [ ] npm test",
		"",
		"## Phase 2: Second",
		"### Changes",
		"### Success Criteria",
		"#### Automated Verification:",
		"- [ ] npm test",
		"",
		"## Whole-Plan Verification",
		"",
		"The block's own bookkeeping notes:",
		"",
		bullet,
		"",
	)
	f.writePlan(t, plan)
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaboration(1, "First", "export const foo = 1;"))
	f.elaborate(t, "2026-06-24_demo__phase-2.md", elaboration(2, "Second", "export const bar = 2;"))

	out := run(t, f.planPath)
	stitched := f.readPlan(t)

	// The bullet rides the re-appended tail verbatim — the stitch moves whole blocks only, never
	// interprets their body prose.
	contains(t, out, "whole-plan verification: authored")
	contains(t, stitched, bullet)
	expectCount(t, stitched, `^## Whole-Plan Verification`, 1)
	// The stitch never rewrites the frontmatter — byte-identical in and out.
	fmOf := func(s string) string { return regexp.MustCompile(`^---\n[\s\S]*\n---\n`).FindString(s) }
	if got, want := fmOf(stitched), fmOf(plan); got != want {
		t.Errorf("frontmatter rewritten:\n%s\n--- want ---\n%s", got, want)
	}
}

func TestWPVDetectsAndReAppendsASuffixFormAuthoredHeadingVerbatim(t *testing.T) {
	f := newFixture(t)
	f.writePlan(t, planWithAuthoredWPV(wpvSuffixBlock, false))
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaboration(1, "First", "export const foo = 1;"))
	f.elaborate(t, "2026-06-24_demo__phase-2.md", elaboration(2, "Second", "export const bar = 2;"))

	out := run(t, f.planPath)
	stitched := f.readPlan(t)

	contains(t, out, "whole-plan verification: authored")
	if !strings.HasSuffix(stitched, wpvSuffixBlock) {
		t.Errorf("stitched plan does not end with the suffix-form block:\n%s", stitched)
	}
	expectCount(t, stitched, `^## Whole-Plan Verification`, 1)
}

// --- Refusals ------------------------------------------------------------------------------------------

func TestRefusesAnElaborationWhoseFenceNeverClosesNamingTheFileAndOpenerLine(t *testing.T) {
	f := newFixture(t)
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaborationWithLeakedFence(1, "First"))
	f.elaborate(t, "2026-06-24_demo__phase-2.md", elaboration(2, "Second", "export const bar = 2;"))

	stderr := runFail(t, f.planPath)

	contains(t, stderr, "2026-06-24_demo__phase-1.md refused")
	contains(t, stderr, "fence opened at line 16 never closes")
	contains(t, stderr, "four backticks")
	if f.readPlan(t) != planDoc {
		t.Error("plan was rewritten by a refused run")
	}
}

func TestRefusesAnElaborationCarryingASecondOutOfFencePhaseHeading(t *testing.T) {
	f := newFixture(t)
	doubled := elaboration(1, "First", "export const foo = 1;") + "\n## Phase 2: Second\n- stray\n"
	f.elaborate(t, "2026-06-24_demo__phase-1.md", doubled)

	stderr := runFail(t, f.planPath)

	contains(t, stderr, "2026-06-24_demo__phase-1.md refused")
	contains(t, stderr, "2 '## Phase N:' headings outside fences, expected exactly 1")
	if f.readPlan(t) != planDoc {
		t.Error("plan was rewritten by a refused run")
	}
}

func TestAcceptsTheSameEmbedUnderAFourBacktickOuterFence(t *testing.T) {
	f := newFixture(t)
	fixed := strings.Replace(elaborationWithLeakedFence(1, "First"), "```markdown", "````markdown", 1)
	fixed = strings.Replace(fixed, "more prose\n```", "more prose\n````", 1)
	f.elaborate(t, "2026-06-24_demo__phase-1.md", fixed)
	f.elaborate(t, "2026-06-24_demo__phase-2.md", elaboration(2, "Second", "export const bar = 2;"))

	out := run(t, f.planPath)
	stitched := f.readPlan(t)

	contains(t, out, "stitched 2/2 phases")
	expectCount(t, stitched, phaseHeadingPattern, 2)
	contains(t, stitched, "````markdown")
}

// --- Go API ----------------------------------------------------------------------------------------------

func TestResultCarriesTheRunCountsAndTheWPVOutcome(t *testing.T) {
	f := newFixture(t)
	f.writePlan(t, referenceOnlyPlan)
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaborationWithAV(1, "First"))
	f.elaborate(t, "2026-06-24_demo__phase-2.md", elaborationWithAV(2, "Second"))

	res, err := Stitch(f.planPath, Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := Result{PlanPath: f.planPath, Stitched: 2, Total: 2, WPVMode: WPVNone, WPVNote: NoteNoVerificationItems}
	if res.PlanPath != want.PlanPath || res.Stitched != want.Stitched || res.Total != want.Total ||
		len(res.Missing) != 0 || res.WPVMode != want.WPVMode || res.WPVNote != want.WPVNote {
		t.Errorf("got %+v, want %+v", res, want)
	}
	if got, want := res.Report(), "stitch-elaborations: stitched 2/2 phases into 2026-06-24_demo.md — whole-plan verification: none (notes reference but no verification items)"; got != want {
		t.Errorf("Report() = %q, want %q", got, want)
	}
}

func TestResultListsMissingPhasesAscending(t *testing.T) {
	f := newFixture(t)
	f.writePlan(t, lines(
		"---",
		"phase_count: 3",
		"---",
		"",
		"## Phase 3: Third",
		"- c",
		"",
		"## Phase 1: First",
		"- a",
		"",
		"## Phase 2: Second",
		"- b",
		"",
	))
	f.elaborate(t, "2026-06-24_demo__phase-2.md", elaboration(2, "Second", "export const bar = 2;"))

	res, err := Stitch(f.planPath, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Missing, []int{1, 3}) || res.Stitched != 1 || res.Total != 3 {
		t.Errorf("got %+v", res)
	}
	contains(t, res.Report(), "stitched 1/3 phases into 2026-06-24_demo.md — no elaboration for phase(s) 1, 3 — whole-plan verification: none")
}

func TestRefusalsAreTypedErrors(t *testing.T) {
	f := newFixture(t)
	_, err := Stitch(filepath.Join(f.plansDir, "missing.md"), Options{})
	var e *Error
	if !errors.As(err, &e) || e.Kind != KindWiring {
		t.Errorf("missing plan: got %v", err)
	}
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaborationWithLeakedFence(1, "First"))
	_, err = Stitch(f.planPath, Options{})
	if !errors.As(err, &e) || e.Kind != KindRefused {
		t.Errorf("leaked fence: got %v", err)
	}
}

func TestRefusesToWriteWhenTheStitchedHeadingCountWouldDrift(t *testing.T) {
	f := newFixture(t)
	// The raw doc is fence-balanced and its section carries one out-of-fence heading (the second one
	// sits inside the fence the section opens and never closes), so the doc is accepted — but spliced
	// in, that open fence swallows the plan's own `## Phase 2:` heading.
	f.elaborate(t, "2026-06-24_demo__phase-1.md", lines(
		"---",
		"status: ready",
		"---",
		"",
		"```",
		"## Phase 1: First",
		"```",
		"## Phase 2: Second",
		"",
	))

	stderr := runFail(t, f.planPath)

	if stderr != "stitch-elaborations: refusing to write 2026-06-24_demo.md — '## Phase N:' headings would drift from 2 to 1" {
		t.Errorf("unexpected message: %s", stderr)
	}
	if f.readPlan(t) != planDoc {
		t.Error("plan was rewritten by a refused run")
	}
}

func TestResolvesARelativePlanPathAgainstTheGitRootOption(t *testing.T) {
	f := newFixture(t)
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaboration(1, "First", "export const foo = 1;"))
	f.elaborate(t, "2026-06-24_demo__phase-2.md", elaboration(2, "Second", "export const bar = 2;"))

	res, err := Stitch(filepath.Join(".rpiv", "artifacts", "plans", "2026-06-24_demo.md"),
		Options{GitRoot: f.root, Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if res.PlanPath != f.planPath {
		t.Errorf("PlanPath = %q, want %q", res.PlanPath, f.planPath)
	}
	contains(t, f.readPlan(t), "export const bar = 2;")
}

func TestResolvesARelativePlanPathAgainstDirWhenThereIsNoGitRoot(t *testing.T) {
	f := newFixture(t)
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaboration(1, "First", "export const foo = 1;"))
	// Keep the git lookup from escaping the temp root, so Dir is what resolves the path.
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(f.root))

	res, err := Stitch(filepath.Join(".rpiv", "artifacts", "plans", "2026-06-24_demo.md"), Options{Dir: f.root})
	if err != nil {
		t.Fatal(err)
	}
	if res.PlanPath != f.planPath {
		t.Errorf("PlanPath = %q, want %q", res.PlanPath, f.planPath)
	}
}

func TestResolvesARelativePlanPathAgainstTheGitRootBeforeDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	f := newFixture(t)
	if out, err := exec.Command("git", "-C", f.root, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git init: %v: %s", err, out)
	}
	f.elaborate(t, "2026-06-24_demo__phase-1.md", elaboration(1, "First", "export const foo = 1;"))
	sub := filepath.Join(f.root, "sub", "dir")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := Stitch(filepath.Join(".rpiv", "artifacts", "plans", "2026-06-24_demo.md"), Options{Dir: sub})
	if err != nil {
		t.Fatal(err)
	}
	// Symlinked temp roots are reported by git in resolved form; compare the resolved paths.
	got, _ := filepath.EvalSymlinks(res.PlanPath)
	want, _ := filepath.EvalSymlinks(f.planPath)
	if got != want {
		t.Errorf("PlanPath = %q, want %q", res.PlanPath, f.planPath)
	}
}
