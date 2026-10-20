package rpc

import (
	"io"
	"strings"
	"time"
)

// DefaultCloseGrace is how long Close waits for the child to exit after its
// standard input is closed before the process group is killed, when
// Options.CloseGrace is zero.
const DefaultCloseGrace = 5 * time.Second

// Options configures one child pig session. Every field is optional; the zero
// value starts `pig --mode rpc` in the current directory with the parent's
// environment and pig's own defaults.
type Options struct {
	// Pig is the executable to run. Empty means ResolvePig's lookup chain.
	Pig string
	// Args are extra command-line arguments appended verbatim after the
	// flags the other fields produce.
	Args []string
	// Cwd is the child's working directory; empty means the parent's.
	Cwd string
	// Env entries ("KEY=value") are appended to the parent's environment. A
	// later entry for the same key replaces an inherited one.
	Env []string
	// Stderr receives the child's diagnostics; nil discards them. It is
	// written from a goroutine until the child's stderr closes.
	Stderr io.Writer

	// Model is the --model argument (provider/id, optionally with
	// ":<thinking>").
	Model string
	// Thinking is the --thinking level.
	Thinking string
	// Piglet is the --piglet name or path.
	Piglet string
	// NoSession passes --no-session (an ephemeral session).
	NoSession bool
	// SessionDir is the --session-dir directory.
	SessionDir string
	// Session is the --session file path or partial UUID to resume.
	Session string
	// Tools is the --tools allowlist, joined with commas.
	Tools []string
	// NoExtensions passes --no-extensions.
	NoExtensions bool
	// Extensions are --extension paths (or builtin:<name>), one flag each.
	Extensions []string
	// SystemPrompt is the --system-prompt text.
	SystemPrompt string
	// AppendSystemPrompt are --append-system-prompt values, one flag each.
	AppendSystemPrompt []string
	// Name is the --name session display name.
	Name string

	// CloseGrace bounds Close's wait for a voluntary exit; zero means
	// DefaultCloseGrace.
	CloseGrace time.Duration
}

// CommandArgs returns the arguments Start passes to the pig executable: the
// RPC mode flag, one flag per set field in the order the fields are declared,
// then Args.
func (o Options) CommandArgs() []string {
	args := []string{"--mode", "rpc"}
	if o.Piglet != "" {
		args = append(args, "--piglet", o.Piglet)
	}
	if o.Model != "" {
		args = append(args, "--model", o.Model)
	}
	if o.Thinking != "" {
		args = append(args, "--thinking", o.Thinking)
	}
	if o.NoSession {
		args = append(args, "--no-session")
	}
	if o.SessionDir != "" {
		args = append(args, "--session-dir", o.SessionDir)
	}
	if o.Session != "" {
		args = append(args, "--session", o.Session)
	}
	if len(o.Tools) > 0 {
		args = append(args, "--tools", strings.Join(o.Tools, ","))
	}
	if o.NoExtensions {
		args = append(args, "--no-extensions")
	}
	for _, path := range o.Extensions {
		args = append(args, "--extension", path)
	}
	if o.SystemPrompt != "" {
		args = append(args, "--system-prompt", o.SystemPrompt)
	}
	for _, text := range o.AppendSystemPrompt {
		args = append(args, "--append-system-prompt", text)
	}
	if o.Name != "" {
		args = append(args, "--name", o.Name)
	}
	return append(args, o.Args...)
}

// closeGrace returns the effective Close wait.
func (o Options) closeGrace() time.Duration {
	if o.CloseGrace > 0 {
		return o.CloseGrace
	}
	return DefaultCloseGrace
}
