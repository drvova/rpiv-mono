package workflow

import (
	"regexp"
	"strings"
	"testing"
)

func assistant(stop StopReason, parts ...BranchContentPart) BranchEntry {
	return BranchEntry{Type: "message", Message: &BranchMessage{Role: "assistant", Content: parts, StopReason: stop}}
}

func user(text string) BranchEntry {
	return BranchEntry{Type: "message", Message: &BranchMessage{Role: "user", Content: []BranchContentPart{{Type: "text", Text: text}}}}
}

func text(s string) BranchContentPart { return BranchContentPart{Type: "text", Text: s} }

func TestClassifyStop(t *testing.T) {
	cases := []struct {
		name   string
		branch []BranchEntry
		offset int
		want   StopSignal
	}{
		{"no assistant message", []BranchEntry{user("hi")}, 0, StopNoResponse},
		{"pre-stopReason assistant collapses to stop", []BranchEntry{assistant("")}, 0, "stop"},
		{"last assistant stop reason wins", []BranchEntry{assistant(StopReasonLength), assistant(StopReasonAborted)}, 0, "aborted"},
		{"offset excludes prior entries", []BranchEntry{assistant(StopReasonError), user("x")}, 1, StopNoResponse},
		{"negative offset reads as zero", []BranchEntry{assistant(StopReasonToolUse)}, -5, "toolUse"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClassifyStop(c.branch, c.offset); got != c.want {
				t.Fatalf("got %q", got)
			}
		})
	}
}

func TestLastAssistantText(t *testing.T) {
	branch := []BranchEntry{assistant("", text("first")), user("q"), assistant(StopReasonStop, text("  a  "), BranchContentPart{Type: "thinking", Text: "hidden"}, text("b"))}
	got, ok := LastAssistantText(branch, 0)
	if !ok || got != "a  \nb" {
		t.Fatalf("%q %v", got, ok)
	}
	if _, ok := LastAssistantText([]BranchEntry{assistant(StopReasonStop, text("   "))}, 0); ok {
		t.Fatal("blank text should read false")
	}
	if _, ok := LastAssistantText([]BranchEntry{user("x")}, 0); ok {
		t.Fatal("no assistant")
	}
}

func TestLastMatchInBranch(t *testing.T) {
	pattern := regexp.MustCompile(`\.rpiv/artifacts/\S+\.md`)
	branch := []BranchEntry{
		assistant("", text("wrote .rpiv/artifacts/a/one.md")),
		assistant("", BranchContentPart{Type: "toolCall", Name: "write", Arguments: map[string]any{"path": ".rpiv/artifacts/a/tool.md"}}),
		assistant("", text("then .rpiv/artifacts/a/two.md and .rpiv/artifacts/a/three.md")),
	}
	if got, ok := LastMatchInBranch(branch, pattern, 0); !ok || got != ".rpiv/artifacts/a/three.md" {
		t.Fatalf("%q", got)
	}
	if got, _ := LastMatchInBranch(branch[:2], pattern, 0); got != ".rpiv/artifacts/a/one.md" {
		t.Fatalf("tool parts must be ignored: %q", got)
	}
	if _, ok := LastMatchInBranch(branch, pattern, 3); ok {
		t.Fatal("offset past end")
	}
}

func TestIterToolUses(t *testing.T) {
	branch := []BranchEntry{
		user("x"),
		assistant("", BranchContentPart{Type: "toolCall", Name: "bash", Arguments: map[string]any{"command": "ls"}}, BranchContentPart{Type: "text", Text: "t"}),
		assistant("", BranchContentPart{Type: "tool_use", Name: "write", Input: map[string]any{"path": "a"}}, BranchContentPart{Type: "toolCall"}, BranchContentPart{Type: "toolCall", Name: "noargs"}),
	}
	var seen []string
	for tu := range IterToolUses(branch, 0) {
		seen = append(seen, tu.Name+":"+jsonStringify(tu.Input))
	}
	if strings.Join(seen, " ") != `bash:{"command":"ls"} write:{"path":"a"} noargs:{}` {
		t.Fatalf("%v", seen)
	}
	count := 0
	for range IterToolUses(branch, 2) {
		count++
	}
	if count != 2 {
		t.Fatalf("offset: %d", count)
	}
	for range IterToolUses(branch, 0) {
		break
	}
}

func TestReadBranchAndSessionRef(t *testing.T) {
	decoded := []any{map[string]any{"type": "message", "message": map[string]any{"role": "assistant", "stopReason": "stop", "content": []any{map[string]any{"type": "text", "text": "hi"}, map[string]any{"type": "toolCall", "name": "bash", "arguments": map[string]any{"command": "ls"}}}}}}
	branch := ReadBranch(readerOf{decoded})
	if len(branch) != 1 || branch[0].Message.StopReason != StopReasonStop || branch[0].Message.Content[1].Arguments["command"] != "ls" {
		t.Fatalf("%+v", branch)
	}
	if ReadBranch(readerOf{"garbage"}) != nil {
		t.Fatal("unknown shape reads as empty")
	}
	typed := []BranchEntry{assistant("")}
	if got := ReadBranch(readerOf{typed}); len(got) != 1 {
		t.Fatal("typed passthrough")
	}
	ref := ReadSessionRef(identityOf("id1", "/f.jsonl", true), Int(3))
	if ref != (SessionRef{ID: "id1", File: "/f.jsonl", BranchOffset: ref.BranchOffset}) || *ref.BranchOffset != 3 {
		t.Fatalf("%+v", ref)
	}
	raw := jsonStringify(ref)
	if raw != `{"id":"id1","file":"/f.jsonl","branchOffset":3}` {
		t.Fatal(raw)
	}
	if jsonStringify(ReadSessionRef(identityOf("id2", "", false), nil)) != `{"id":"id2"}` {
		t.Fatal("fresh in-memory session ref must carry only id")
	}
}

type readerOf struct{ v any }

func (r readerOf) GetBranch() any { return r.v }

type identity struct {
	id, file  string
	persisted bool
}

func identityOf(id, file string, persisted bool) identity { return identity{id, file, persisted} }
func (i identity) GetSessionID() string                   { return i.id }
func (i identity) GetSessionFile() (string, bool)         { return i.file, i.persisted }
