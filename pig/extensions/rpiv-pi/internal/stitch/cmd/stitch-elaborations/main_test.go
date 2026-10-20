package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var planDoc = strings.Join([]string{
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
	"## Phase 1: First",
	"- `a.ts` — add foo",
	"",
	"## Phase 2: Second",
	"- `b.ts` — add bar",
	"",
}, "\n")

func elaboration(n int, body ...string) string {
	return strings.Join(append([]string{"---", "status: ready", "---", ""}, body...), "\n") + "\n"
}

// fixture lays out .rpiv/artifacts/{plans,elaborations} under a temp root with the demo plan written.
type fixture struct {
	root, plansDir, elaborationsDir, planPath string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	f := fixture{
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
	writeFile(t, f.planPath, planDoc)
	return f
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runCLI(t *testing.T, args ...string) (status int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	status = run(args, &out, &errOut)
	return status, out.String(), errOut.String()
}

func TestExits1WithoutPlanPathArgument(t *testing.T) {
	for _, args := range [][]string{nil, {""}} {
		status, stdout, stderr := runCLI(t, args...)
		if status != 1 || stdout != "" || stderr != "stitch-elaborations: missing <plan-path>\n" {
			t.Errorf("args %q: status=%d stdout=%q stderr=%q", args, status, stdout, stderr)
		}
	}
}

func TestExits1WhenPlanIsMissing(t *testing.T) {
	f := newFixture(t)
	missing := filepath.Join(f.plansDir, "missing.md")
	status, stdout, stderr := runCLI(t, missing)
	if status != 1 || stdout != "" || stderr != "stitch-elaborations: plan not found: "+missing+"\n" {
		t.Errorf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
}

func TestExits1WhenNothingToStitch(t *testing.T) {
	f := newFixture(t)
	status, _, stderr := runCLI(t, f.planPath)
	if status != 1 || !strings.Contains(stderr, "nothing to stitch") {
		t.Errorf("status=%d stderr=%q", status, stderr)
	}
}

func TestExits1OnRefusedElaboration(t *testing.T) {
	f := newFixture(t)
	writeFile(t, filepath.Join(f.elaborationsDir, "2026-06-24_demo__phase-1.md"),
		elaboration(1, "## Phase 1: First", "```ts", "export const foo = 1;"))
	status, _, stderr := runCLI(t, f.planPath)
	if status != 1 || !strings.Contains(stderr, "2026-06-24_demo__phase-1.md refused") {
		t.Errorf("status=%d stderr=%q", status, stderr)
	}
	if got, _ := os.ReadFile(f.planPath); string(got) != planDoc {
		t.Error("plan was rewritten by a refused run")
	}
}

func TestExits0OnPartialRunAndPrintsTheSummary(t *testing.T) {
	f := newFixture(t)
	writeFile(t, filepath.Join(f.elaborationsDir, "2026-06-24_demo__phase-1.md"),
		elaboration(1, "## Phase 1: First", "```ts", "export const foo = 1;", "```"))
	status, stdout, stderr := runCLI(t, f.planPath)
	want := "stitch-elaborations: stitched 1/2 phases into 2026-06-24_demo.md — no elaboration for phase(s) 2 — whole-plan verification: none\n"
	if status != 0 || stdout != want || stderr != "" {
		t.Errorf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
}

func TestResolvesARelativePlanPathAgainstTheWorkingDirectory(t *testing.T) {
	f := newFixture(t)
	writeFile(t, filepath.Join(f.elaborationsDir, "2026-06-24_demo__phase-1.md"),
		elaboration(1, "## Phase 1: First", "```ts", "export const foo = 1;", "```"))
	writeFile(t, filepath.Join(f.elaborationsDir, "2026-06-24_demo__phase-2.md"),
		elaboration(2, "## Phase 2: Second", "```ts", "export const bar = 2;", "```"))
	// Keep the git lookup from escaping the temp root, so the fallback to cwd is what resolves the path.
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(f.root))
	t.Chdir(f.root)
	status, stdout, stderr := runCLI(t, filepath.Join(".rpiv", "artifacts", "plans", "2026-06-24_demo.md"))
	if status != 0 || !strings.Contains(stdout, "stitched 2/2 phases") || stderr != "" {
		t.Errorf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
}
