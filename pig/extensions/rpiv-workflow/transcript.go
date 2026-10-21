package workflow

import (
	"iter"
	"regexp"
	"strings"
)

// Branch-entry shape + predicates. The host's session transcript is a
// discriminated union whose variants are not all public; ReadBranch is the
// single boundary that narrows it to []BranchEntry — every consumer goes
// through it. No artifact-path scanning lives here: discovery is the
// collector's job; LastMatchInBranch and IterToolUses are the shared
// transcript-walking primitives collectors reuse.

// StopReason mirrors the host's stop-reason union attached to assistant messages.
type StopReason string

const (
	StopReasonStop    StopReason = "stop"
	StopReasonLength  StopReason = "length"
	StopReasonToolUse StopReason = "toolUse"
	StopReasonError   StopReason = "error"
	StopReasonAborted StopReason = "aborted"
)

// StopSignal is the normalised stop signal: a StopReason plus "noResponse"
// for branches with no assistant message at all. Pre-stopReason assistant
// messages collapse to "stop".
type StopSignal string

// StopNoResponse is the StopSignal of a branch the model never spoke on.
const StopNoResponse StopSignal = "noResponse"

// ClassifyStop is the single chokepoint mapping (hasAssistantMessage,
// lastAssistantStopReason) → StopSignal. offsetStart < 0 means 0.
func ClassifyStop(branch []BranchEntry, offsetStart int) StopSignal {
	if !HasAssistantMessage(branch, offsetStart) {
		return StopNoResponse
	}
	if reason, ok := LastAssistantStopReason(branch, offsetStart); ok {
		return StopSignal(reason)
	}
	return StopSignal(StopReasonStop)
}

// BranchContentPart is one content part inside an assistant message. The
// host's union carries more variants than these; the ones collectors walk
// are modelled and unknown parts pass through structurally. Text parts carry
// Text; tool invocations carry Name plus an argument object under either
// spelling — {type: "toolCall", arguments} (the SDK shape every persisted
// session file carries) or {type: "tool_use", input} (the Anthropic wire
// spelling). IterToolUses normalises both.
type BranchContentPart struct {
	Type string `json:"type"`
	// Text is a "text" part's spoken text — a pointer because the walkers
	// test `typeof part.text === "string"`: a part with no text is skipped,
	// a part with empty text is kept (an empty line).
	Text      *string        `json:"text,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// BranchMessage is the message payload of a "message" branch entry.
type BranchMessage struct {
	Role       string              `json:"role,omitempty"`
	Content    []BranchContentPart `json:"content,omitempty"`
	StopReason StopReason          `json:"stopReason,omitempty"`
}

// BranchEntry is one entry of a session branch.
type BranchEntry struct {
	Type    string         `json:"type"`
	Message *BranchMessage `json:"message,omitempty"`
}

// toolUsePartTypes are the part types that carry a tool invocation.
var toolUsePartTypes = map[string]bool{"toolCall": true, "tool_use": true}

// toolUseInput is the invocation's argument object under either spelling
// (Arguments first — the live SDK shape — then Input); empty when neither is set.
func toolUseInput(part BranchContentPart) map[string]any {
	if part.Arguments != nil {
		return part.Arguments
	}
	if part.Input != nil {
		return part.Input
	}
	return map[string]any{}
}

// ToolUse is a normalised tool invocation yielded by IterToolUses.
type ToolUse struct {
	Name  string
	Input map[string]any
}

// SessionRef is the host session that backed a stage activation — a value
// object the row serialises verbatim (wire shape = domain shape), produced by
// ReadSessionRef. ID is the session identity, forever. File is the session's
// on-disk location at capture time — a HINT, absent for non-persisting
// sessions; a stale path is recoverable through the locate ladder.
// BranchOffset is the offset the activation ran under (continue-policy stages
// only). File and BranchOffset are meaningless without ID, hence nested.
type SessionRef struct {
	ID           string `json:"id"`
	File         string `json:"file,omitempty"`
	BranchOffset *int   `json:"branchOffset,omitempty"`
}

// BranchReader is the narrow host read ReadBranch needs.
type BranchReader interface {
	GetBranch() any
}

// ReadBranch reads the current branch from the host's session manager — the
// single boundary that narrows the host's transcript value to []BranchEntry.
// A []BranchEntry passes through; a []map[string]any (decoded JSON) is
// converted; anything else reads as an empty branch.
func ReadBranch(sm BranchReader) []BranchEntry {
	switch raw := sm.GetBranch().(type) {
	case []BranchEntry:
		return raw
	case []any:
		out := make([]BranchEntry, 0, len(raw))
		for _, e := range raw {
			if m, ok := e.(map[string]any); ok {
				out = append(out, branchEntryFromMap(m))
			}
		}
		return out
	case []map[string]any:
		out := make([]BranchEntry, 0, len(raw))
		for _, m := range raw {
			out = append(out, branchEntryFromMap(m))
		}
		return out
	}
	return nil
}

func branchEntryFromMap(m map[string]any) BranchEntry {
	entry := BranchEntry{}
	entry.Type, _ = m["type"].(string)
	msg, ok := m["message"].(map[string]any)
	if !ok {
		return entry
	}
	bm := &BranchMessage{}
	bm.Role, _ = msg["role"].(string)
	if reason, ok := msg["stopReason"].(string); ok {
		bm.StopReason = StopReason(reason)
	}
	if parts, ok := msg["content"].([]any); ok {
		bm.Content = make([]BranchContentPart, 0, len(parts))
		for _, p := range parts {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			part := BranchContentPart{}
			part.Type, _ = pm["type"].(string)
			if text, ok := pm["text"].(string); ok {
				part.Text = &text
			}
			part.Name, _ = pm["name"].(string)
			part.Input, _ = pm["input"].(map[string]any)
			part.Arguments, _ = pm["arguments"].(map[string]any)
			bm.Content = append(bm.Content, part)
		}
	}
	entry.Message = bm
	return entry
}

// SessionIdentity is the narrow host read ReadSessionRef needs.
type SessionIdentity interface {
	GetSessionID() string
	GetSessionFile() (string, bool)
}

// ReadSessionRef captures the active session's identity as the SessionRef
// value stage rows store. branchOffset is the policy-derived offset the
// activation ran under (nil for fresh sessions, so the key is dropped from
// the serialised row); File is likewise dropped for non-persisting sessions.
func ReadSessionRef(sm SessionIdentity, branchOffset *int) SessionRef {
	ref := SessionRef{ID: sm.GetSessionID(), BranchOffset: branchOffset}
	if file, ok := sm.GetSessionFile(); ok {
		ref.File = file
	}
	return ref
}

func isAssistant(e BranchEntry) bool {
	return e.Type == "message" && e.Message != nil && e.Message.Role == "assistant"
}

// HasAssistantMessage reports whether the branch (from offsetStart) carries
// an assistant message.
func HasAssistantMessage(branch []BranchEntry, offsetStart int) bool {
	for i := max(offsetStart, 0); i < len(branch); i++ {
		if isAssistant(branch[i]) {
			return true
		}
	}
	return false
}

// LastAssistantStopReason is the last assistant message's stop reason; the
// second result is false for empty branches or pre-stopReason messages.
func LastAssistantStopReason(branch []BranchEntry, offsetStart int) (StopReason, bool) {
	for i := len(branch) - 1; i >= max(offsetStart, 0); i-- {
		if !isAssistant(branch[i]) {
			continue
		}
		reason := branch[i].Message.StopReason
		return reason, reason != ""
	}
	return "", false
}

// LastAssistantText is the concatenated text parts of the LAST assistant
// message (joined with newlines, trimmed); false when the branch has no
// assistant message or its text parts are empty.
func LastAssistantText(branch []BranchEntry, offsetStart int) (string, bool) {
	for i := len(branch) - 1; i >= max(offsetStart, 0); i-- {
		if !isAssistant(branch[i]) {
			continue
		}
		parts := []string{}
		for _, part := range branch[i].Message.Content {
			if part.Type == "text" && part.Text != nil {
				parts = append(parts, *part.Text)
			}
		}
		text := strings.TrimSpace(strings.Join(parts, "\n"))
		return text, text != ""
	}
	return "", false
}

// MatchMode says which String.prototype.match shape a pattern scan
// reproduces — the port of the RegExp `g` flag, which Go's regexp cannot
// spell. The flag is caller-owned in the TypeScript collectors: the built-in
// URL and directory-path collectors scan globally; an author's own pattern
// may not.
type MatchMode int

const (
	// MatchGlobal mirrors text.match(/re/g): every whole-match occurrence is
	// found and the LAST one is the result.
	MatchGlobal MatchMode = iota
	// MatchFirst mirrors text.match(/re/) without g: only the first
	// occurrence counts, and with capture groups the result is the LAST
	// group's text (String.match returns [whole, g1, g2, …] and the scan
	// takes the final element). A last group that did not participate reads
	// as no match for that text.
	MatchFirst
)

// MatchText applies pattern to text under mode and returns what the
// TypeScript `matches[matches.length - 1]` yields; false when the pattern
// does not match at all (an empty match is "", true). The shared primitive
// behind LastMatchInBranch and the tool-argument fallback scan.
func MatchText(pattern *regexp.Regexp, mode MatchMode, text string) (string, bool) {
	if mode == MatchGlobal {
		matches := pattern.FindAllString(text, -1)
		if len(matches) == 0 {
			return "", false
		}
		return matches[len(matches)-1], true
	}
	loc := pattern.FindStringSubmatchIndex(text)
	if loc == nil {
		return "", false
	}
	last := len(loc) - 2
	if loc[last] < 0 {
		return "", false
	}
	return text[loc[last]:loc[last+1]], true
}

// LastMatchInBranch scans assistant text blocks in reverse for pattern and
// returns the first block's result under mode (MatchText). The agent's final
// message is usually where the actionable path/URL lands, so iterating from
// the tail short-circuits on the first hit. Thinking, tool-invocation and
// text-less parts are ignored. The second result is false only when no
// block matched.
func LastMatchInBranch(branch []BranchEntry, pattern *regexp.Regexp, mode MatchMode, offsetStart int) (string, bool) {
	for i := len(branch) - 1; i >= max(offsetStart, 0); i-- {
		if !isAssistant(branch[i]) {
			continue
		}
		content := branch[i].Message.Content
		for j := len(content) - 1; j >= 0; j-- {
			part := content[j]
			if part.Type != "text" || part.Text == nil {
				continue
			}
			if hit, ok := MatchText(pattern, mode, *part.Text); ok {
				return hit, true
			}
		}
	}
	return "", false
}

// IterToolUses yields every tool-invocation part the assistant emitted in
// branch order (forward — the "what did the agent do?" scan direction),
// normalised to {Name, Input} under both spellings.
func IterToolUses(branch []BranchEntry, offsetStart int) iter.Seq[ToolUse] {
	return func(yield func(ToolUse) bool) {
		for i := max(offsetStart, 0); i < len(branch); i++ {
			if !isAssistant(branch[i]) {
				continue
			}
			for _, part := range branch[i].Message.Content {
				if !toolUsePartTypes[part.Type] || part.Name == "" {
					continue
				}
				if !yield(ToolUse{Name: part.Name, Input: toolUseInput(part)}) {
					return
				}
			}
		}
	}
}
