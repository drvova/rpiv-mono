package rpc

import (
	"strings"
	"testing"
	"time"
)

func TestCommandArgs(t *testing.T) {
	opts := Options{
		Args:               []string{"--verbose"},
		Model:              "mock/mock-1",
		Thinking:           "high",
		Piglet:             "/repo/piglet.yaml",
		NoSession:          true,
		SessionDir:         "/tmp/sessions",
		Session:            "abc123",
		Tools:              []string{"read", "bash"},
		NoExtensions:       true,
		Extensions:         []string{"/ext/a", "builtin:plan-mode"},
		SystemPrompt:       "be brief",
		AppendSystemPrompt: []string{"one", "two"},
		Name:               "stage 1",
	}
	want := []string{
		"--mode", "rpc",
		"--piglet", "/repo/piglet.yaml",
		"--model", "mock/mock-1",
		"--thinking", "high",
		"--no-session",
		"--session-dir", "/tmp/sessions",
		"--session", "abc123",
		"--tools", "read,bash",
		"--no-extensions",
		"--extension", "/ext/a",
		"--extension", "builtin:plan-mode",
		"--system-prompt", "be brief",
		"--append-system-prompt", "one",
		"--append-system-prompt", "two",
		"--name", "stage 1",
		"--verbose",
	}
	got := opts.CommandArgs()
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args\n got %q\nwant %q", got, want)
	}
}

func TestCommandArgsZero(t *testing.T) {
	got := Options{}.CommandArgs()
	if len(got) != 2 || got[0] != "--mode" || got[1] != "rpc" {
		t.Fatalf("args %q", got)
	}
}

func TestCloseGraceDefault(t *testing.T) {
	if (Options{}).closeGrace() != DefaultCloseGrace {
		t.Fatal("zero CloseGrace is not the default")
	}
	if (Options{CloseGrace: time.Second}).closeGrace() != time.Second {
		t.Fatal("CloseGrace ignored")
	}
}
