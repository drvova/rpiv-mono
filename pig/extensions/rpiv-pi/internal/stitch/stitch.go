// Package stitch splices per-phase elaboration docs back into a synthesized plan, deterministically.
//
// It is the fan-in barrier of the elaborate fanout. Each `## Phase N:` section in the plan is replaced,
// one-for-one, by the matching elaboration doc's body. This is a pure swap by phase number — NOT a
// reconcile: `synthesize` already resolved the cross-phase seams; this only injects the code each
// `elaborate` lane wrote.
//
// Elaboration docs live in the sibling bucket .rpiv/artifacts/elaborations/, named
// `<plan-basename-without-ext>__phase-<N>.md` (the `elaborate` skill's output contract). Each carries a
// single `## Phase <N>: <title>` section with implement-ready code; its frontmatter is stripped before
// splicing.
//
// Preserved verbatim: the plan's frontmatter (incl. `phase_count`) and the preamble before the first
// phase (`## Synthesis Notes`, etc.). The trailing `## Whole-Plan Verification` section is preserved as
// well: an authored block is re-appended verbatim after the last phase (the swap of an elaborated last
// phase would otherwise drop it, heading to EOF); with no authored block but a Synthesis-Notes reference
// to one, a block is derived from the per-phase `Automated Verification:` items — never invented
// otherwise. Phase boundaries are detected fence-aware (a `## ` inside a Find/Replace code block is NOT
// a boundary) and each phase owns everything up to the next `## Phase N:` heading, so the swap is
// idempotent — re-stitching can't accumulate duplicate per-phase trailers (Success Criteria / Notes /
// Deferred). An elaboration is refused when a fence it opens never closes (the leak would swallow the
// next phase's heading) or when it carries other than one out-of-fence `## Phase N:` heading, and the
// stitched body is refused before writing if its heading count drifted, so the downstream
// `phase_count == '## Phase N:' headings` derive-check stays valid.
//
// [Stitch] succeeds on every normal run (a plan phase with no elaboration is left as-is and reported —
// partial runs are allowed) and returns an *[Error] on a refused elaboration or a wiring/path error:
// missing plan, or zero elaboration docs found. The command under cmd/stitch-elaborations maps that onto
// the CLI's exit codes: 0 for a normal run, 1 for a refusal or wiring error.
package stitch

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Options tunes how a relative plan path is resolved. The zero value resolves it the way the CLI does:
// against `git rev-parse --show-toplevel` run in the process working directory, falling back to that
// directory when the lookup fails. An absolute plan path is used as given.
type Options struct {
	// Dir is the directory the git lookup runs in and the fallback base for a relative plan path. Empty
	// means the process working directory; a relative Dir is taken relative to it.
	Dir string
	// GitRoot, when non-empty, stands in for the git lookup, which then never runs.
	GitRoot string
}

// WPVMode says how the trailing `## Whole-Plan Verification` section was resolved.
type WPVMode string

const (
	// WPVAuthored means an authored block found in the original last-phase span was re-appended
	// verbatim after the last rebuilt phase.
	WPVAuthored WPVMode = "authored"
	// WPVDerived means the preamble references a whole-plan verification block and one was derived from
	// the final sections' `#### Automated Verification:` items.
	WPVDerived WPVMode = "derived"
	// WPVNone means nothing was emitted; Result.WPVNote carries the fail-open reason when a block was
	// wanted.
	WPVNone WPVMode = "none"
)

// Fail-open reasons reported next to WPVNone.
const (
	// NoteNoVerificationItems: the preamble references a block, nothing authored was found and the
	// final sections carry no checkbox items to derive one from.
	NoteNoVerificationItems = "notes reference but no verification items"
	// NoteBlockAlreadyPresent: a block is already present in the preamble or a kept section, so the
	// candidate was suppressed to keep exactly one section per output.
	NoteBlockAlreadyPresent = "block already present"
)

// Result describes a completed run.
type Result struct {
	// PlanPath is the resolved path of the plan that was rewritten.
	PlanPath string
	// Stitched counts the plan phases swapped for their elaboration; Total counts the plan's
	// `## Phase N:` headings.
	Stitched, Total int
	// Missing lists, ascending, the phase numbers that had no elaboration and were kept as-is.
	Missing []int
	// WPVMode is the whole-plan verification outcome; WPVNote its fail-open reason, or "".
	WPVMode WPVMode
	WPVNote string
}

// Report returns the summary line the CLI prints on stdout for this run, without a trailing newline:
//
//	stitch-elaborations: stitched 1/2 phases into plan.md — no elaboration for phase(s) 2 — whole-plan verification: none
func (r Result) Report() string {
	var b strings.Builder
	fmt.Fprintf(&b, "stitch-elaborations: stitched %d/%d phases into %s", r.Stitched, r.Total, filepath.Base(r.PlanPath))
	if len(r.Missing) > 0 {
		b.WriteString(" — no elaboration for phase(s) ")
		for i, n := range r.Missing {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(strconv.Itoa(n))
		}
	}
	fmt.Fprintf(&b, " — whole-plan verification: %s", r.WPVMode)
	if r.WPVNote != "" {
		fmt.Fprintf(&b, " (%s)", r.WPVNote)
	}
	return b.String()
}

// Kind classifies a refused run.
type Kind int

const (
	// KindWiring is a wiring/path error: the plan does not exist, or no elaboration doc pairs with it.
	KindWiring Kind = iota + 1
	// KindRefused is a refused elaboration (a leaked fence, other than one `## Phase N:` heading) or a
	// stitched body whose `## Phase N:` heading count drifted from the plan's.
	KindRefused
)

// Error is a refused run. Message is the exact line the CLI prints on stderr before exiting 1.
type Error struct {
	Kind    Kind
	Message string
}

func (e *Error) Error() string { return e.Message }

// The provenance marker emitted as a derived block's first body line — the byte-stable discriminator
// between authored content and a prior stitch's own derived emission.
const derivedMarker = "<!-- derived by stitch-elaborations from per-phase Automated Verification items; re-derived on every stitch -->"

// The whole-plan verification heading, canonical prefix; see isWPVHeading for the suffix tolerance.
const wpvHeading = "## Whole-Plan Verification"

// The plan template's exact `#### Automated Verification:` heading — preserved verbatim by every
// elaborate lane's Success Criteria section.
const avHeading = "#### Automated Verification:"

// The one intro line of a derived block.
const wpvIntro = "Collected from each phase's `Automated Verification:` blocks in plan order; run on the merged tree once every phase has landed."

// The Synthesis-Notes promise probe: the canonical phrase only, matched case-insensitively. A promise
// phrased solely as the skills' shorthand is deliberately not detected — widening the probe would fire
// the derived fallback on non-promises.
const notesReference = "whole-plan verification"

// Stitch splices the elaboration docs paired with the plan at planPath back into it and rewrites the
// plan in place. See the package documentation for the contract. The error is an *Error for a refused
// elaboration, a heading-count drift or a wiring/path error; any other error is an I/O failure.
func Stitch(planPath string, opts Options) (Result, error) {
	planPath, err := resolvePlanPath(planPath, opts)
	if err != nil {
		return Result{}, err
	}
	if _, err := os.Stat(planPath); err != nil {
		return Result{}, &Error{Kind: KindWiring, Message: "stitch-elaborations: plan not found: " + planPath}
	}

	planBase := strings.TrimSuffix(filepath.Base(planPath), ".md")
	// Elaborations are the sibling bucket of plans/: .rpiv/artifacts/elaborations/.
	elaborationsDir := filepath.Join(filepath.Dir(planPath), "..", "elaborations")

	elaborations, err := collectElaborations(elaborationsDir, planBase)
	if err != nil {
		return Result{}, err
	}
	if len(elaborations) == 0 {
		return Result{}, &Error{Kind: KindWiring, Message: "stitch-elaborations: no elaboration docs for \"" + planBase + "\" in " + elaborationsDir +
			" (expected " + planBase + "__phase-<N>.md) — nothing to stitch"}
	}

	raw, err := os.ReadFile(planPath)
	if err != nil {
		return Result{}, fmt.Errorf("stitch-elaborations: %w", err)
	}
	frontmatter, body := splitFrontmatter(string(raw))

	// `## Phase N:` headings at column 0 and OUTSIDE fenced code blocks are the ONLY section boundaries:
	// each phase owns everything up to the next phase heading (its `### Success Criteria`,
	// `## Notes / Deferred`, and any `## …` that appears inside a Find/Replace fence). Two reasons this
	// matters:
	//   • Fence-aware — an elaboration's Find/Replace blocks legitimately contain `## ` lines (doc
	//     edits); a naive per-`## ` split shreds them into orphan fragments with dangling Replace-with
	//     markers.
	//   • Idempotent — replacing a phase subsumes whatever trailing matter a prior stitch left behind, so
	//     re-elaborate → re-stitch cycles can't accumulate duplicate Success-Criteria / Notes sections.
	// Content before the first phase (Synthesis Notes, etc.) is the preamble, kept verbatim. Content
	// after the last phase is absorbed into that phase — the plan format has no post-phase appendix;
	// trailing matter belongs to the elaboration — EXCEPT the whole-plan verification section, which
	// the stitch strips from the last phase and re-emits itself (see wholePlanVerification).
	starts := phaseStarts(body)

	preamble := body
	if len(starts) > 0 {
		preamble = body[:starts[0].offset]
	}

	// Whole-plan verification extraction runs off the ORIGINAL last-phase span BEFORE the rebuilt
	// sections: a kept-original last phase is stripped at the extracted offset below (an elaborated
	// one drops the whole span via the swap), and the emitter re-appends the resolved tail after the
	// rebuilt sections — authored verbatim, derived when only the Synthesis Notes reference a block,
	// never invented.
	var wpvExtracted *trailingWPV
	if len(starts) > 0 {
		wpvExtracted = extractTrailingWPV(body[starts[len(starts)-1].offset:])
	}

	stitched := 0
	var missing []int
	rebuilt := make([]string, 0, len(starts))
	for i, start := range starts {
		end := len(body)
		if i+1 < len(starts) {
			end = starts[i+1].offset
		}
		original := body[start.offset:end]
		if replacement, ok := elaborations[start.n]; ok {
			stitched++
			rebuilt = append(rebuilt, replacement)
			continue
		}
		missing = append(missing, start.n)
		if wpvExtracted != nil && i == len(starts)-1 {
			rebuilt = append(rebuilt, jsTrim(original[:wpvExtracted.offset]))
			continue
		}
		rebuilt = append(rebuilt, jsTrim(original))
	}

	wpv := wholePlanVerification(preamble, wpvExtracted, rebuilt)

	// The WPV tail joins the body as a peer top-level element, NOT as part of `rebuilt` — it is no
	// phase's section content, and folding it into one would feed the block's own AV-shaped checkbox
	// lines back into the next derivation.
	parts := make([]string, 0, len(rebuilt)+2)
	parts = append(parts, jsTrim(preamble))
	parts = append(parts, rebuilt...)
	if wpv.text != "" {
		parts = append(parts, wpv.text)
	}
	parts = slices.DeleteFunc(parts, func(s string) bool { return s == "" })
	newBody := strings.Join(parts, "\n\n")

	after := len(headingOffsets(newBody, isPhaseHeading))
	if after != len(starts) {
		return Result{}, &Error{Kind: KindRefused, Message: fmt.Sprintf(
			"stitch-elaborations: refusing to write %s — '## Phase N:' headings would drift from %d to %d",
			filepath.Base(planPath), len(starts), after)}
	}
	if err := os.WriteFile(planPath, []byte(jsTrimEnd(frontmatter)+"\n\n"+newBody+"\n"), 0o644); err != nil {
		return Result{}, fmt.Errorf("stitch-elaborations: %w", err)
	}

	slices.Sort(missing)
	return Result{
		PlanPath: planPath,
		Stitched: stitched,
		Total:    len(starts),
		Missing:  missing,
		WPVMode:  wpv.mode,
		WPVNote:  wpv.note,
	}, nil
}

// resolvePlanPath applies the CLI's path rule: an absolute path as given, a relative one against the git
// root, then the working directory.
func resolvePlanPath(raw string, opts Options) (string, error) {
	if filepath.IsAbs(raw) {
		return raw, nil
	}
	dir := opts.Dir
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("stitch-elaborations: %w", err)
		}
		dir = wd
	} else {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", fmt.Errorf("stitch-elaborations: %w", err)
		}
		dir = abs
	}
	root := opts.GitRoot
	if root == "" {
		root = gitRoot(dir)
	}
	if root == "" {
		root = dir
	}
	resolved, err := filepath.Abs(filepath.Join(root, raw))
	if err != nil {
		return "", fmt.Errorf("stitch-elaborations: %w", err)
	}
	return resolved, nil
}

// gitRoot is the trimmed output of `git rev-parse --show-toplevel` run in dir, or "" when git is absent
// or dir is outside a repository.
func gitRoot(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return jsTrim(string(out))
}

// collectElaborations maps phase number -> spliced section text for every elaboration doc paired with
// the plan, keyed off the `<planBase>__phase-<N>.md` filename so the pairing is independent of any
// timestamp slug inside the doc. A missing elaborations directory yields no docs. A doc with no
// `## Phase N:` line at all is skipped; one whose fence never closes, or that carries other than one
// out-of-fence `## Phase N:` heading, refuses the whole run.
//
// Entries are visited in filename order (the CLI visits them in directory order): the only observable
// difference is which of several refused docs is named first.
func collectElaborations(dir, planBase string) (map[int]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("stitch-elaborations: %w", err)
	}
	elaborations := map[int]string{}
	for _, entry := range entries {
		name := entry.Name()
		n, ok := elaborationPhase(name, planBase)
		if !ok {
			continue
		}
		rawBytes, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("stitch-elaborations: %w", err)
		}
		raw := string(rawBytes)
		_, body := splitFrontmatter(raw)
		section, ok := phaseSection(body)
		if !ok {
			continue
		}
		if open := openFenceLine(raw); open > 0 {
			return nil, &Error{Kind: KindRefused, Message: fmt.Sprintf(
				"stitch-elaborations: %s refused — the fence opened at line %d never closes "+
					"(a bare ``` inside a ``` block closes it; open and close the outer block with four backticks)",
				name, open)}
		}
		headings := len(headingOffsets(section, isPhaseHeading))
		if headings != 1 {
			return nil, &Error{Kind: KindRefused, Message: fmt.Sprintf(
				"stitch-elaborations: %s refused — %d '## Phase N:' headings outside fences, expected exactly 1",
				name, headings)}
		}
		elaborations[n] = section
	}
	return elaborations, nil
}

// elaborationPhase parses `<planBase>__phase-<N>.md` into N.
func elaborationPhase(name, planBase string) (int, bool) {
	rest, ok := strings.CutPrefix(name, planBase+"__phase-")
	if !ok {
		return 0, false
	}
	digits, ok := strings.CutSuffix(rest, ".md")
	if !ok || !allDigits(digits) {
		return 0, false
	}
	return parsePhaseNumber(digits), true
}

// splitFrontmatter splits a document into (frontmatter, body); frontmatter ("" when absent) is kept
// verbatim: a leading `---` line through the first following `---` line, newlines included.
func splitFrontmatter(content string) (frontmatter, body string) {
	const open, close = "---\n", "\n---\n"
	if !strings.HasPrefix(content, open) {
		return "", content
	}
	i := strings.Index(content[len(open):], close)
	if i < 0 {
		return "", content
	}
	end := len(open) + i + len(close)
	return content[:end], content[end:]
}

// phaseSection extracts the `## Phase N:` section (heading to EOF, trimmed) from an elaboration body.
// The heading search is line-anchored but not fence-aware; the fence and heading-count checks run on
// the result.
func phaseSection(body string) (string, bool) {
	for from := 0; from <= len(body); {
		i := strings.Index(body[from:], "## Phase ")
		if i < 0 {
			return "", false
		}
		i += from
		if _, ok := phaseHeading(body[i:]); ok && lineStart(body, i) {
			return jsTrim(body[i:]), true
		}
		from = i + 1
	}
	return "", false
}

// lineStart reports whether offset i begins a line: the start of text or just after a line terminator
// (LF, CR, U+2028, U+2029).
func lineStart(text string, i int) bool {
	if i == 0 {
		return true
	}
	switch r, _ := utf8.DecodeLastRuneInString(text[:i]); r {
	case '\n', '\r', 0x2028, 0x2029:
		return true
	}
	return false
}

// phaseStart is a `## Phase N:` boundary in a plan body: its byte offset and phase number.
type phaseStart struct {
	offset, n int
}

// phaseStarts finds every column-0 `## Phase N:` heading outside fenced code blocks, in order.
func phaseStarts(body string) []phaseStart {
	var starts []phaseStart
	var state fenceState
	offset := 0
	for _, line := range strings.Split(body, "\n") {
		if !state.step(line) && !state.inFence {
			if n, ok := phaseHeading(line); ok {
				starts = append(starts, phaseStart{offset: offset, n: n})
			}
		}
		offset += len(line) + 1 // +1 for the stripped "\n"
	}
	return starts
}

// --- Fence walk --------------------------------------------------------------------------------------

// fenceState is the fence walk's state across lines: whether the cursor is inside a fenced code block
// and the delimiter length that opened it.
type fenceState struct {
	inFence  bool
	fenceLen int
}

// fenceLine returns the delimiter run length of a fence-shaped line — optional leading whitespace then
// 3+ backticks or 3+ tildes (the CommonMark fence delimiters) — or 0 for any other line. The close
// predicate in step is deliberately LOOSER than the same-char twins in rpiv-core (markdown-fence and
// reconcile-directives): length-only, NOT same-char, so a backtick-opened fence closed by a bare `~~~`
// line is accepted. Closing that gap is a behavior change, not a cleanup — it would strand the walk
// inside the fence and swallow the next real `## Phase N:` heading (boundaries missed by a mis-tracked
// fence walk).
func fenceLine(line string) int {
	s := strings.TrimLeftFunc(line, isJSSpace)
	if s == "" || (s[0] != '`' && s[0] != '~') {
		return 0
	}
	n := 0
	for n < len(s) && s[n] == s[0] {
		n++
	}
	if n < 3 {
		return 0
	}
	return n
}

// step advances the fence walk across one line. It returns true for any fence-shaped line — an opener,
// a closer, or a fence line that does not close the open fence — so the caller can skip heading work
// and apply its own offset policy. The close test is length-only on purpose (see fenceLine): the
// delimiter run must be at least as long as the opener's and alone on the line.
func (s *fenceState) step(line string) bool {
	n := fenceLine(line)
	if n == 0 {
		return false
	}
	if !s.inFence {
		s.inFence = true
		s.fenceLen = n
	} else if n >= s.fenceLen && len(jsTrim(line)) == n {
		s.inFence = false
		s.fenceLen = 0
	}
	return true
}

// openFenceLine returns the 1-based line of a fence opener that never closes, or 0 when balanced.
func openFenceLine(text string) int {
	var state fenceState
	opened := 0
	for i, line := range strings.Split(text, "\n") {
		was := state.inFence
		if state.step(line) && !was {
			opened = i + 1
		}
	}
	if state.inFence {
		return opened
	}
	return 0
}

// eachOutsideFence calls fn for each line of text OUTSIDE fenced code blocks — the same fence rules as
// the phase-boundary walk (an elaboration's fenced Find/Replace blocks may legitimately contain heading-
// and bullet-shaped lines).
func eachOutsideFence(text string, fn func(line string)) {
	var state fenceState
	for _, line := range strings.Split(text, "\n") {
		if state.step(line) {
			continue
		}
		if !state.inFence {
			fn(line)
		}
	}
}

// headingOffsets returns the byte offsets of the column-0 lines satisfying match OUTSIDE fenced code
// blocks. The unconditional offset advance is load-bearing: the offsets feed slices of text, so fence
// lines must advance the cursor like any other line.
func headingOffsets(text string, match func(line string) bool) []int {
	var offsets []int
	var state fenceState
	offset := 0
	for _, line := range strings.Split(text, "\n") {
		if !state.step(line) && !state.inFence && match(line) {
			offsets = append(offsets, offset)
		}
		offset += len(line) + 1
	}
	return offsets
}

// --- Whole-Plan Verification -------------------------------------------------------------------------
//
// The trailing `## Whole-Plan Verification` section is owned by the stitch, not by any elaboration
// lane: swapping an elaborated last phase drops the whole original span (heading to EOF), which would
// silently drop the block that carries the whole-tree gates. Authored-first, never invented:
//
//  1. An AUTHORED block — a section in the ORIGINAL last-phase span whose first body line is not the
//     derived provenance marker — is re-appended verbatim after the last rebuilt phase; the Synthesis
//     Notes are never consulted when one exists (authored wins over a concurrent reference).
//  2. Only when nothing authored was extracted AND the preamble references a whole-plan verification
//     block (the canonical phrase, fence-aware) is a block DERIVED from the final post-swap sections'
//     per-phase `#### Automated Verification:` checkbox items: heading, provenance marker, one intro
//     line, deduped items in plan order. A derived block found in the original span is a prior stitch's
//     own emission — dropped and re-derived, never treated as authored.
//  3. Otherwise nothing is emitted.
//
// The append is guarded by one fence-aware scan of the post-strip composition: a block already present
// anywhere in the preamble or a kept section (a degenerate mid-document occurrence) suppresses it, so
// every output carries at most one section.

// trailingWPV is the whole-plan verification section of a phase span: its byte offset in the span, its
// text (heading to the next column-0 `## ` heading or end-of-span, trailing whitespace trimmed) and
// whether its first body line is exactly the derived marker (a prior stitch's emission, not authored
// content).
type trailingWPV struct {
	offset  int
	text    string
	derived bool
}

// extractTrailingWPV finds the whole-plan verification section of a phase span, or nil when the span
// carries none.
func extractTrailingWPV(span string) *trailingWPV {
	heads := headingOffsets(span, isWPVHeading)
	if len(heads) == 0 {
		return nil
	}
	at := heads[0]
	end := len(span)
	for _, o := range headingOffsets(span, isH2Heading) {
		if o > at {
			end = o
			break
		}
	}
	text := jsTrimEnd(span[at:end])
	derived := false
	for _, line := range strings.Split(text, "\n")[1:] {
		content := jsTrim(line)
		if content == "" {
			continue
		}
		derived = content == derivedMarker
		break
	}
	return &trailingWPV{offset: at, text: text, derived: derived}
}

// notesReferenceWPV reports whether the preamble — kept verbatim, where the Synthesis Notes live —
// promises a whole-plan verification block via the canonical phrase, fence-aware.
func notesReferenceWPV(preamble string) bool {
	found := false
	eachOutsideFence(preamble, func(line string) {
		if strings.Contains(asciiLower(line), notesReference) {
			found = true
		}
	})
	return found
}

// collectAVItems gathers the per-phase `#### Automated Verification:` checkbox lines from the FINAL
// post-swap sections: collected after the heading, stopping at the next heading of any level,
// fence-aware; deduped by exact trimmed-line equality, in plan order.
func collectAVItems(sections []string) []string {
	var items []string
	seen := map[string]bool{}
	for _, section := range sections {
		inAV := false
		eachOutsideFence(section, func(line string) {
			switch {
			case line == avHeading:
				inAV = true
			case isAnyHeading(line):
				inAV = false
			case inAV:
				item := jsTrim(line)
				if isCheckboxItem(item) && !seen[item] {
					seen[item] = true
					items = append(items, item)
				}
			}
		})
	}
	return items
}

// deriveWPVBlock builds the canonical derived block: heading + provenance marker + one intro line + the
// collected item lines verbatim.
func deriveWPVBlock(items []string) string {
	lines := append([]string{wpvHeading, "", derivedMarker, wpvIntro}, items...)
	return strings.Join(lines, "\n")
}

// wpvOutcome is the resolved whole-plan verification tail: the text to append ("" for none), the mode
// and the fail-open note for the summary.
type wpvOutcome struct {
	text string
	mode WPVMode
	note string
}

// wholePlanVerification resolves the whole-plan verification tail from the section extracted off the
// ORIGINAL last-phase span, the preamble and the rebuilt sections. Pure — it mutates nothing; the strip
// of a kept-original last phase is expressed by the caller's rebuilt construction.
func wholePlanVerification(preamble string, extracted *trailingWPV, rebuilt []string) wpvOutcome {
	var candidate string
	var mode WPVMode
	switch {
	case extracted != nil && !extracted.derived:
		candidate = extracted.text
		mode = WPVAuthored
	case notesReferenceWPV(preamble):
		items := collectAVItems(rebuilt)
		if len(items) == 0 {
			return wpvOutcome{mode: WPVNone, note: NoteNoVerificationItems}
		}
		candidate = deriveWPVBlock(items)
		mode = WPVDerived
	default:
		return wpvOutcome{mode: WPVNone}
	}
	hasWPV := func(part string) bool { return len(headingOffsets(part, isWPVHeading)) > 0 }
	if hasWPV(preamble) || slices.ContainsFunc(rebuilt, hasWPV) {
		return wpvOutcome{mode: WPVNone, note: NoteBlockAlreadyPresent}
	}
	return wpvOutcome{text: candidate, mode: mode}
}

// --- Line predicates ---------------------------------------------------------------------------------

// phaseHeading parses a column-0 `## Phase N:` heading (anything may follow the colon) into N.
func phaseHeading(line string) (int, bool) {
	rest, ok := strings.CutPrefix(line, "## Phase ")
	if !ok {
		return 0, false
	}
	i := 0
	for i < len(rest) && isDigit(rest[i]) {
		i++
	}
	if i == 0 || i == len(rest) || rest[i] != ':' {
		return 0, false
	}
	return parsePhaseNumber(rest[:i]), true
}

func isPhaseHeading(line string) bool {
	_, ok := phaseHeading(line)
	return ok
}

// isWPVHeading matches the whole-plan verification heading: column-0, canonical prefix,
// suffix-tolerant at a word boundary (`## Whole-Plan Verification (owned by validate)` is the same
// section; `## Whole-Plan Verifications` is not).
func isWPVHeading(line string) bool {
	rest, ok := strings.CutPrefix(line, wpvHeading)
	return ok && (rest == "" || !isWordByte(rest[0]))
}

// isH2Heading matches any column-0 `## ` heading.
func isH2Heading(line string) bool { return strings.HasPrefix(line, "## ") }

// isAnyHeading matches a column-0 heading of any level: one to six `#` followed by whitespace.
func isAnyHeading(line string) bool {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n < 1 || n > 6 {
		return false
	}
	r, _ := utf8.DecodeRuneInString(line[n:])
	return isJSSpace(r)
}

// isCheckboxItem matches a trimmed `- [ ]` / `- [x]` checklist line.
func isCheckboxItem(item string) bool {
	return strings.HasPrefix(item, "- [ ]") || strings.HasPrefix(item, "- [x]")
}

// parsePhaseNumber converts a run of ASCII digits to its phase number, saturating at math.MaxInt.
func parsePhaseNumber(digits string) int {
	n, err := strconv.Atoi(digits)
	if err != nil {
		return math.MaxInt
	}
	return n
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func isWordByte(b byte) bool {
	return b == '_' || isDigit(b) || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// asciiLower lowercases the ASCII letters of s only, the case folding the CLI's case-insensitive probe
// applies to its ASCII pattern.
func asciiLower(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, s)
}

// isJSSpace is the whitespace set JavaScript's `\s`, `trim` and `trimEnd` use: Unicode space separators
// plus TAB, VT, FF, ZWNBSP and the four line terminators.
func isJSSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x00A0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	return r >= 0x2000 && r <= 0x200A
}

func jsTrim(s string) string { return strings.TrimFunc(s, isJSSpace) }

func jsTrimEnd(s string) string { return strings.TrimRightFunc(s, isJSSpace) }
