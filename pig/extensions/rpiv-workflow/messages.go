package workflow

import (
	"regexp"
	"strconv"
	"strings"
)

// User-facing message constants.
//   - Msg* / Err* via ctx.ui.notify — one-shot; may be repainted by the host's
//     session transition.
//   - Fail* — structured terminal-failure descriptors (see FailureText).
//
// Audience split: this file is the UI/runtime constants. The model-facing
// validation-retry prompt lives beside its only consumer (sessions);
// legacy-migration notices live in the loader; `/wf` usage strings live in
// the command package. Every string is byte-identical to the TypeScript
// implementation.

// FailureText is one structured descriptor per terminal-failure kind — the
// Toast (the one-shot notify line) and the Error (what lands in
// state.termination.error + the JSONL row's errMsg) rendered from ONE
// factory so the two channels can't drift.
type FailureText struct {
	Toast string
	Error string
}

// MsgStageFailed renders the stage-failed toast.
func MsgStageFailed(skill string) string { return "✗ " + skill + " failed — stopping workflow" }

// FailStageAborted is the user-abort (ESC) failure pair.
func FailStageAborted(skill string) FailureText {
	return FailureText{
		Toast: "⏸ " + skill + " aborted (ESC) — stopping workflow",
		Error: skill + " aborted by user (ESC)",
	}
}

// FailStageTruncated is the output-cap failure pair.
func FailStageTruncated(skill string) FailureText {
	return FailureText{
		Toast: "✗ " + skill + " truncated — model hit output cap mid-reply, stopping workflow",
		Error: skill + " truncated — model hit output-length cap mid-reply",
	}
}

// FailStageToolStalled is the unsettled-tool-loop failure pair.
func FailStageToolStalled(skill string) FailureText {
	return FailureText{
		Toast: "✗ " + skill + " tool loop did not settle — stopping workflow",
		Error: skill + " tool loop did not settle before the orchestrator inspected the branch",
	}
}

// FailStageNoResponse is the no-assistant-message failure pair.
func FailStageNoResponse(skill string) FailureText {
	return FailureText{
		Toast: "✗ " + skill + " produced no response — stopping workflow",
		Error: skill + " produced no assistant message",
	}
}

// MsgWorkflowComplete renders the end-of-run toast.
func MsgWorkflowComplete(stages int) string {
	return "rpiv: workflow complete (" + strconv.Itoa(stages) + " stages)"
}

// MsgWorkflowCancelled is the user-cancellation toast.
const MsgWorkflowCancelled = "rpiv: workflow cancelled"

// FailWorkflowAborted is the programmatic abort (signal, checked between stages).
func FailWorkflowAborted(stage string) FailureText {
	return FailureText{
		Toast: "rpiv: workflow aborted",
		Error: "workflow aborted before stage \"" + stage + "\" (signal)",
	}
}

// FailValidationExhausted is the retries-spent failure pair.
func FailValidationExhausted(skill, failures string) FailureText {
	return FailureText{
		Toast: "rpiv: " + skill + " output validation exhausted retries",
		Error: skill + " output validation failed after retries: " + failures,
	}
}

// ErrValidateRetryUnchanged is validation-retry mechanism-1: the agent was
// asked to fix the artifact but changed nothing observable in the worktree.
func ErrValidateRetryUnchanged(skill string) string {
	return skill + ": validation retry halted — the working tree was unchanged after the fix prompt (no observable edit detected to tracked files or .rpiv/artifacts/); refusing to re-run the same failing validation"
}

// FailValidateGateSkipped is validation-retry mechanism-2: a schema-validated
// produces stage was re-dispatched against an unchanged worktree.
func FailValidateGateSkipped(skill string) FailureText {
	return FailureText{
		Toast: "✗ " + skill + " re-dispatched with no observable change since its last validation failure — stopping workflow",
		Error: skill + " re-dispatched with an unchanged worktree since its last schema-validation failure (no observable fix detected); halting to avoid a no-op retry loop",
	}
}

// FailInputValidation is the upstream-data-invalid failure pair.
func FailInputValidation(currentSkill, prevSkill, failures string) FailureText {
	return FailureText{
		Toast: "✗ " + currentSkill + " input validation failed — upstream " + prevSkill + " produced invalid data",
		Error: "Input validation failed for '" + currentSkill + "': upstream '" + prevSkill + "' produced invalid data: " + failures,
	}
}

// SchemaSlot names which stage schema a validation bound applies to.
type SchemaSlot string

const (
	SlotOutputSchema SchemaSlot = "outputSchema"
	SlotInputSchema  SchemaSlot = "inputSchema"
)

// ErrSchemaTimeout bounds a single schema-validate call.
func ErrSchemaTimeout(slot SchemaSlot, ms int) string {
	return string(slot) + " validation exceeded " + strconv.Itoa(ms) + "ms — schema's ~standard.validate did not settle"
}

// FailMissingArtifact is the no-upstream-artifact failure pair.
func FailMissingArtifact(currentSkill string, stageNumber int) FailureText {
	return FailureText{
		Toast: "✗ " + currentSkill + " has no upstream artifact to consume — stopping workflow",
		Error: "Stage " + strconv.Itoa(stageNumber) + " (" + currentSkill + ") has no upstream artifactPath; only stage 1 may consume the user's original input",
	}
}

// FailMissingNamedRead is the empty-named-channel failure pair.
func FailMissingNamedRead(currentSkill, name string, stageNumber int) FailureText {
	return FailureText{
		Toast: "✗ " + currentSkill + " reads \"" + name + "\" but no upstream produces stage has published it yet — stopping workflow",
		Error: "Stage " + strconv.Itoa(stageNumber) + " (" + currentSkill + ") reads \"" + name + "\" but state.named[\"" + name + "\"] is empty; check that an upstream produces stage publishes this name",
	}
}

// BackwardJumpLimitKind says which re-entry limit tripped — the waive-aware
// cap or the absolute lap ceiling.
type BackwardJumpLimitKind string

const (
	BackwardJumpCap     BackwardJumpLimitKind = "cap"
	BackwardJumpCeiling BackwardJumpLimitKind = "ceiling"
)

// BackwardJumpHaltInfo is everything the backward-jump halt text needs.
type BackwardJumpHaltInfo struct {
	Stage     string
	LimitKind BackwardJumpLimitKind
	// Count is the tripping count (the cap's re-entry count, or the ceiling's lap count).
	Count int
	Max   int
	// Progress is the destination's most recent progress verdicts, oldest →
	// newest; empty ⇒ clause omitted.
	Progress []ProgressValue
}

// BackwardJumpLimitHead is the shared head of both limit arms; external
// replay tooling greps the failure row's errMsg for exactly this substring.
const BackwardJumpLimitHead = "Backward-jump limit exceeded"

func backwardJumpProgressClause(progress []ProgressValue) string {
	if len(progress) == 0 {
		return ""
	}
	parts := make([]string, len(progress))
	for i, p := range progress {
		parts[i] = string(p)
	}
	return "; last progress: " + strings.Join(parts, ", ")
}

// FailBackwardJumpExhausted renders the cap or ceiling halt pair.
func FailBackwardJumpExhausted(info BackwardJumpHaltInfo) FailureText {
	clause := backwardJumpProgressClause(info.Progress)
	count, maxStr := strconv.Itoa(info.Count), strconv.Itoa(info.Max)
	if info.LimitKind == BackwardJumpCeiling {
		return FailureText{
			Toast: "rpiv: backward-jump limit exceeded — \"" + info.Stage + "\" re-entered " + count + " times, over the absolute lap ceiling (max " + maxStr + ")" + clause + " — stopping workflow to prevent infinite loop",
			Error: BackwardJumpLimitHead + ": stage \"" + info.Stage + "\" re-entered " + count + " times, over the absolute lap ceiling (max " + maxStr + ")" + clause,
		}
	}
	return FailureText{
		Toast: "rpiv: backward-jump limit exceeded — \"" + info.Stage + "\" re-entered " + count + " times (max " + maxStr + ")" + clause + " — stopping workflow to prevent infinite loop",
		Error: BackwardJumpLimitHead + ": stage \"" + info.Stage + "\" re-entered " + count + " times (max " + maxStr + ")" + clause,
	}
}

// exhaustionHaltNote matches the meta unit-graph's non-resumable halt
// vocabulary: a spent correction budget, an unchanged failure signature, an
// uncorrectable fatal, or a lost gate record. Resuming such a run re-decides
// the identical stop, so the remedy is a fresh run.
var exhaustionHaltNote = regexp.MustCompile(`\bFATAL —|\brounds spent\b|the same failures survived|integrity stop`)

// IsExhaustionHalt reports whether a route note carries the non-resumable
// halt vocabulary. A nil note never does.
func IsExhaustionHalt(note *string) bool {
	return note != nil && exhaustionHaltNote.MatchString(*note)
}

// FailGateStop renders a decision-edge termination: a match without a
// fallback saw an unexpected value (resumable — the toast names `/wf @<runId>`),
// or a budget-spent meta halt (not resumable — start a fresh run).
func FailGateStop(stage, note, runID string) FailureText {
	if IsExhaustionHalt(&note) {
		return FailureText{
			Toast: "✗ workflow halted at \"" + stage + "\" — " + note + "; see the stage's verdict for findings. The correction budget is spent, so resuming re-decides the same stop — fix the artifact, then start a fresh run.",
			Error: "Workflow halted at \"" + stage + "\" (budget exhausted): " + note,
		}
	}
	return FailureText{
		Toast: "✗ workflow stopped at \"" + stage + "\" — its routing gate matched no branch (" + note + "); see the stage's verdict for findings, then fix and resume with /wf @" + runID,
		Error: "Routing gate after \"" + stage + "\" matched no branch: " + note,
	}
}

// MsgLoopZeroUnits warns that a pull loop's first call returned null.
func MsgLoopZeroUnits(skill string) string {
	return "rpiv: " + skill + " iterate loop produced zero units — nothing published, advancing"
}

// FailLoopCapHalt is the onCap: "halt" failure pair.
func FailLoopCapHalt(count, max int) FailureText {
	return FailureText{
		Toast: "rpiv: loop cap exceeded (" + strconv.Itoa(count) + "/" + strconv.Itoa(max) + ") — stopping workflow to prevent an unbounded loop",
		Error: "Loop cap exceeded: " + strconv.Itoa(count) + " units (max " + strconv.Itoa(max) + ")",
	}
}

// FailFanoutAllFailed is the haltWhenAllFailed failure pair.
func FailFanoutAllFailed(skill string, failed, total int) FailureText {
	return FailureText{
		Toast: "rpiv: " + skill + " fanout failed in full (" + strconv.Itoa(failed) + "/" + strconv.Itoa(total) + " units) — stopping workflow",
		Error: "Fanout all-failed at stage \"" + skill + "\" (" + strconv.Itoa(failed) + "/" + strconv.Itoa(total) + " units failed)",
	}
}

// MsgLoopCapAdvance is the onCap: "advance" soft-stop warning.
func MsgLoopCapAdvance(skill string, max int) string {
	return "rpiv: " + skill + " loop reached its cap (" + strconv.Itoa(max) + ") — projecting the configured result and advancing"
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// FailVerifyFailed is the verify-budget-exhausted failure pair.
func FailVerifyFailed(stage string, attempts int) FailureText {
	a := strconv.Itoa(attempts)
	return FailureText{
		Toast: "✗ " + stage + " verification failed after " + a + " attempt" + plural(attempts) + " — stopping workflow",
		Error: "Verification failed for \"" + stage + "\": the judge's verdict did not satisfy `pass` after " + a + " attempt" + plural(attempts),
	}
}

// FailAuditWrite is the audit-row-not-written failure pair.
func FailAuditWrite(skill string) FailureText {
	return FailureText{
		Toast: "✗ " + skill + " completed but audit row could not be written — stopping workflow",
		Error: skill + " completed but the JSONL audit row could not be appended; halting to keep in-memory state aligned with disk",
	}
}

// MsgFailureRowDropped is notified when a terminal failure/aborted row could not be appended.
func MsgFailureRowDropped(stage string) string {
	return "⚠ rpiv: failure row for " + stage + " not persisted to audit trail — do not resume this run from disk"
}

// MsgChainAdvanceFailed is the edge-threw-after-success toast.
func MsgChainAdvanceFailed(fromStage, reason string) string {
	return "✗ chain advance after " + fromStage + " failed: " + reason + " — stopping workflow"
}

// MsgStageThrew is the stage-threw-before-recording toast.
func MsgStageThrew(skill, reason string) string {
	return "✗ stage " + skill + " failed: " + reason + " — stopping workflow"
}

// MsgLoopCursorCorrupt is the loop-cursor invariant violation text.
func MsgLoopCursorCorrupt(stage, detail string) string {
	return "✗ loop stage " + stage + ": cursor invariant violated — " + detail
}

// ErrCollectorThrew attributes a collector throw.
func ErrCollectorThrew(skill, reason string) string {
	return skill + ": outcome collector threw: " + reason
}

// ErrParserThrew attributes a parser throw.
func ErrParserThrew(skill, reason string) string {
	return skill + ": outcome parser threw: " + reason
}

// FailSkillNotRegistered is the unknown-skill failure pair.
func FailSkillNotRegistered(skill string, stageNumber int) FailureText {
	return FailureText{
		Toast: "✗ " + skill + " is not a registered Pi skill — stopping workflow",
		Error: "Stage " + strconv.Itoa(stageNumber) + " requires Pi skill \"" + skill + "\" but no skill by that name is registered with Pi (check installed sibling packages and `pi.skills` manifest entries)",
	}
}

// MsgRoutingAuditDropped is notified when a routing-decision row could not be appended.
func MsgRoutingAuditDropped(fromStage, decision string) string {
	return "⚠ rpiv: routing decision " + fromStage + " → " + decision + " not persisted to audit trail (continuing run)"
}

// MsgPartialArtifacts is the recap surfaced on stage failure.
func MsgPartialArtifacts(artifactList string) string {
	return "Artifacts produced before failure:\n" + artifactList
}

// MsgLifecycleThrew is the lifecycle-listener-threw warning.
func MsgLifecycleThrew(event, reason string) string {
	return "⚠ rpiv: lifecycle listener (" + event + ") threw: " + reason
}

// MsgSnapshotFailed is the outcome-snapshot-threw warning.
func MsgSnapshotFailed(stage, reason string) string {
	return "⚠ rpiv: outcome snapshot for " + stage + " threw (" + reason + ") — pre-stage diff degraded for this stage"
}

// FailScriptThrew is the script-body-threw failure pair.
func FailScriptThrew(stage, reason string) FailureText {
	return FailureText{
		Toast: "✗ " + stage + " script threw — stopping workflow: " + reason,
		Error: stage + " script threw: " + reason,
	}
}

// Resume-refusal messages — reconstruct refusals returned in the
// RunWorkflowResult envelope.

// ErrResumeNoRows is the empty-trail refusal.
func ErrResumeNoRows(runID string) string {
	return "rpiv: run " + runID + " has no recorded stages — nothing to resume"
}

// ErrResumeMalformedRow is the deep-shape-guard refusal.
func ErrResumeMalformedRow(detail string) string {
	return "rpiv: cannot resume — the run's trail contains a malformed stage row (" + detail + "); " +
		"resume refuses rather than replay an incomplete history"
}

// ErrResumeVersionMismatch is the schema-version refusal.
func ErrResumeVersionMismatch(detail string, expected int) string {
	return "rpiv: cannot resume — " + detail + ", but this build reads schema v" + strconv.Itoa(expected) + "; " +
		"resume refuses rather than misread the trail"
}

// ErrResumeStageGone is the renamed-or-removed-stage refusal.
func ErrResumeStageGone(stage, workflow string) string {
	return "rpiv: cannot resume — stage \"" + stage + "\" from the run no longer exists in workflow \"" + workflow + "\" " +
		"(renamed or removed)"
}

// ErrResumeLoopMismatch is the loop-drift refusal.
func ErrResumeLoopMismatch(stage string) string {
	return "rpiv: cannot resume — loop stage \"" + stage + "\" recomputed a different unit than the run recorded " +
		"(the unit source must be deterministic w.r.t. the replayed run state + accumulated outputs; " +
		"resume refuses rather than re-run the wrong units)"
}

// MsgResumeLoopMismatch is the loop-drift toast.
func MsgResumeLoopMismatch(stage string) string {
	return "rpiv: loop \"" + stage + "\" changed on resume — cannot safely continue"
}

// Session-backed resume (promotion + reattach) — notify-only progress toasts.

// MsgResumePromoted is the promotion toast.
func MsgResumePromoted(skill string) string {
	return "✓ " + skill + ": interrupted session already produced the artifact — stage promoted to completed"
}

// MsgResumeReattached is the reattach toast.
func MsgResumeReattached(skill string) string {
	return "↻ " + skill + ": reattached to interrupted session"
}

// MsgResumeSessionFallback is the one ladder-rung notice before a
// session-backed resume degrades to a cold re-run.
func MsgResumeSessionFallback(skill, why string) string {
	return "rpiv: " + skill + " — " + why + "; re-running the stage from scratch"
}

// MsgContinueFallback is the no-predecessor-session notice for continue stages.
func MsgContinueFallback(skill string) string {
	return "rpiv: " + skill + " — no prior session to continue; running the stage fresh"
}

// ReattachPrompt is the model-facing prompt sent when a stage reattaches to
// its interrupted session.
func ReattachPrompt(skill string) string {
	return "The previous run of /skill:" + skill + " in this session was interrupted. " +
		"Review the conversation above. If the work is already complete, announce the artifact path. " +
		"Otherwise finish the remaining work and announce the artifact path when done."
}

// Pre-resume guards (resolve → load-gate → find).

// MsgResumeUsage is the @resume usage line.
const MsgResumeUsage = "rpiv: usage — /wf @<run-id | name | path-to.jsonl>"

// MsgRunNotFound is the unresolved-ref notice.
func MsgRunNotFound(ref string) string { return "rpiv: no run found for \"" + ref + "\"" }

// MsgResumeWorkflowGone is the unregistered-workflow notice.
func MsgResumeWorkflowGone(workflow, ref string) string {
	return "rpiv: run \"" + ref + "\" used workflow \"" + workflow + "\", which is no longer registered"
}

// /wf command shell — notify-only.

// MsgInteractiveOnly is the non-interactive refusal.
const MsgInteractiveOnly = "/wf requires interactive mode"

// MsgWorkflowThrew is the unexpected-runner-failure notice.
func MsgWorkflowThrew(reason string) string {
	return "/wf: workflow runner failed unexpectedly: " + reason
}

// MsgNameInvalid is the malformed-run-name notice.
func MsgNameInvalid(name string) string {
	return "rpiv: invalid name \"" + name + "\" — must be 1-" + strconv.Itoa(MaxNameLength) + " chars, start with a letter or underscore, only letters, digits, hyphens, underscores"
}

// MsgNameCollision is the name-already-claimed notice.
func MsgNameCollision(name, runID string) string {
	return "name '" + name + "' already used by run " + runID
}

// MsgNameIndexWriteFailed is the names-index write failure notice.
func MsgNameIndexWriteFailed(name string) string {
	return "/wf: could not persist name \"" + name + "\" to the names index — run aborted (no run started). Check filesystem permissions for .rpiv/workflows/runs/"
}

// MsgHeaderWriteFailed is the header write failure notice.
func MsgHeaderWriteFailed(runID string) string {
	return "rpiv: could not write the run header for " + runID + " — run aborted (no stage executed). Check filesystem permissions for .rpiv/workflows/runs/"
}

// MsgNameIgnoredOnResume is the --name-on-@resume notice.
const MsgNameIgnoredOnResume = "/wf: --name has no effect on @resume (the ref already identifies the run) — ignoring it"

// MsgNameFlagMidInput is the mid-input --name notice.
const MsgNameFlagMidInput = "/wf: --name is only honored as the first or last token — a mid-input --name is treated as workflow input text"

// MsgFlagRepeated is the repeated-flag notice (the first-typed value wins).
func MsgFlagRepeated(flag string) string {
	return "/wf: " + flag + " given more than once — the first value wins, the rest are ignored"
}

// MsgJumpCapAboveLapCeiling warns that the jump cap can never trip.
func MsgJumpCapAboveLapCeiling(cap, ceiling int) string {
	c := strconv.Itoa(ceiling)
	return "/wf: --max-jumps " + strconv.Itoa(cap) + " is at or above the lap ceiling " + c + " — the ceiling halts first, so at most " + c + " re-entries per stage; raise --max-laps to widen the run"
}

// MsgBudgetInvalid is the malformed programmatic budget refusal; value is
// rendered the way JavaScript renders a number (NaN included).
func MsgBudgetInvalid(option string, value float64) string {
	return option + " must be a non-negative integer, got " + formatJSNumber(value)
}

// MsgLoadAborted is the config-errors notice.
func MsgLoadAborted(count int) string {
	noun := "config errors"
	if count == 1 {
		noun = "config error"
	}
	return "/wf: " + strconv.Itoa(count) + " " + noun + " — see warnings above (fix and re-run)"
}

// MsgWorkflowNotFound is the unknown-workflow notice.
func MsgWorkflowNotFound(name string) string { return "/wf: workflow \"" + name + "\" not found" }

// MsgNoWorkflowsRegistered is the empty-registry notice.
const MsgNoWorkflowsRegistered = "/wf: no workflows registered — install a sibling that bundles workflows or author one in `.rpiv/workflows/config.ts`"

// FatalStatus is the terminal status a fatal-failure descriptor records.
type FatalStatus string

const (
	FatalFailed  FatalStatus = "failed"
	FatalAborted FatalStatus = "aborted"
)

// FatalFailureArgs is the toast + JSONL halves of a terminal failure, paired
// by construction. Build via FailedArgs / AbortedArgs so a halt site can't
// mismatch status and notify level.
type FatalFailureArgs struct {
	Status      FatalStatus
	NotifyMsg   string
	NotifyLevel NotifyLevel
	ErrMsg      string
}

// fatalArgsOf is the ONE (status, notifyLevel) pairing — notifyLevel is
// DERIVED from status (failed → error, aborted → warning).
func fatalArgsOf(status FatalStatus, f FailureText) FatalFailureArgs {
	level := NotifyWarning
	if status == FatalFailed {
		level = NotifyError
	}
	return FatalFailureArgs{Status: status, NotifyMsg: f.Toast, NotifyLevel: level, ErrMsg: f.Error}
}

// FailedArgs builds the recordFatalFailure quadruple for a failure (notifies at "error").
func FailedArgs(failure FailureText) FatalFailureArgs { return fatalArgsOf(FatalFailed, failure) }

// FailedArgsOf is the two-string form of FailedArgs.
func FailedArgsOf(notifyMsg, errMsg string) FatalFailureArgs {
	return fatalArgsOf(FatalFailed, FailureText{Toast: notifyMsg, Error: errMsg})
}

// AbortedArgs builds the quadruple for an abort (notifies at "warning").
func AbortedArgs(failure FailureText) FatalFailureArgs { return fatalArgsOf(FatalAborted, failure) }

// AbortedArgsOf is the two-string form of AbortedArgs.
func AbortedArgsOf(notifyMsg, errMsg string) FatalFailureArgs {
	return fatalArgsOf(FatalAborted, FailureText{Toast: notifyMsg, Error: errMsg})
}
