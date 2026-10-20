package workflow

import (
	"context"
	"testing"
)

// The port tripwire: a minimal fake host satisfies every port interface, so
// a port change that breaks the executor contract fails this package's
// compile, not a downstream host's.

type fakeUI struct{ notes []string }

func (u *fakeUI) Notify(message string, level NotifyLevel) {
	u.notes = append(u.notes, string(level)+": "+message)
}

type fakeSessions struct{}

func (fakeSessions) GetBranch() any                 { return []BranchEntry{} }
func (fakeSessions) GetSessionID() string           { return "s1" }
func (fakeSessions) GetSessionFile() (string, bool) { return "", false }

type fakeLauncher struct{ ui *fakeUI }

func (l *fakeLauncher) Cwd() string                        { return "/cwd" }
func (l *fakeLauncher) HasUI() bool                        { return true }
func (l *fakeLauncher) UI() HostUI                         { return l.ui }
func (l *fakeLauncher) SessionManager() HostSessionManager { return fakeSessions{} }
func (l *fakeLauncher) WaitForIdle() error                 { return nil }
func (l *fakeLauncher) Signal() context.Context            { return context.Background() }

type fakeExecutor struct {
	fakeLauncher
	spawned []SpawnOptions
}

func (e *fakeExecutor) MaxConcurrency() int { return 1 }
func (e *fakeExecutor) SpawnChild(_ context.Context, options SpawnOptions, withSession func(WorkflowSessionContext) error) error {
	e.spawned = append(e.spawned, options)
	return withSession(&fakeSession{fakeExecutor: e})
}

type fakeSession struct {
	*fakeExecutor
	sent []string
}

func (s *fakeSession) SendUserMessage(content string) error {
	s.sent = append(s.sent, content)
	return nil
}
func (s *fakeSession) ToolTimeout() (string, bool) { return "", false }
func (s *fakeSession) ResetToolTimeout()           {}

type fakeHost struct{ commands []CommandInfo }

func (h *fakeHost) RegisterCommand(name string, _ RegisterCommandOptions) {
	h.commands = append(h.commands, CommandInfo{Name: name, Source: "test"})
}
func (h *fakeHost) GetCommands() []CommandInfo { return h.commands }

var (
	_ WorkflowHost            = (*fakeHost)(nil)
	_ WorkflowLauncherContext = (*fakeLauncher)(nil)
	_ WorkflowHostContext     = (*fakeExecutor)(nil)
	_ WorkflowSessionContext  = (*fakeSession)(nil)
	_ DispatchHost            = (*fakeLauncher)(nil)
	_ RunView                 = (*RunState)(nil)
	_ BranchReader            = fakeSessions{}
	_ SessionIdentity         = fakeSessions{}
)

func TestHostPortsAreSatisfied(t *testing.T) {
	exec := &fakeExecutor{fakeLauncher: fakeLauncher{ui: &fakeUI{}}}
	var child WorkflowSessionContext
	err := exec.SpawnChild(context.Background(), SpawnOptions{Prompt: "/skill:plan", UnitIndex: Int(2)}, func(c WorkflowSessionContext) error {
		child = c
		return c.SendUserMessage("hi")
	})
	if err != nil || len(exec.spawned) != 1 || *exec.spawned[0].UnitIndex != 2 || child == nil {
		t.Fatalf("%v %+v", err, exec.spawned)
	}
	if reason, fired := child.ToolTimeout(); fired || reason != "" {
		t.Fatal("no watchdog")
	}
	if ReadSessionRef(child.SessionManager(), nil) != (SessionRef{ID: "s1"}) {
		t.Fatal("session ref")
	}
	host := &fakeHost{}
	host.RegisterCommand("wf", RegisterCommandOptions{Description: "d", Handler: func(string, WorkflowLauncherContext) error { return nil }})
	if len(host.GetCommands()) != 1 || host.GetCommands()[0].Name != "wf" {
		t.Fatal("commands")
	}
}
