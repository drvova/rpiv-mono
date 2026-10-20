package rpc

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// collector records every event in order.
type collector struct {
	mu     sync.Mutex
	events []Event
	seen   chan string
}

func collect(c *Client) *collector {
	col := &collector{seen: make(chan string, 1024)}
	c.Subscribe(func(event Event) {
		col.mu.Lock()
		col.events = append(col.events, event)
		col.mu.Unlock()
		col.seen <- event.Type
	})
	return col
}

func (col *collector) types() []string {
	col.mu.Lock()
	defer col.mu.Unlock()
	out := make([]string, len(col.events))
	for i, event := range col.events {
		out[i] = event.Type
	}
	return out
}

// waitFor blocks until an event of typ was dispatched.
func (col *collector) waitFor(t *testing.T, typ string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case seen := <-col.seen:
			if seen == typ {
				return
			}
		case <-deadline:
			t.Fatalf("no %s event (saw %v)", typ, col.types())
		}
	}
}

func TestCallCorrelatesOutOfOrderResponses(t *testing.T) {
	c := startFake(t)
	ctx := testContext(t)
	type outcome struct {
		name     string
		response Response
		err      error
	}
	finished := make(chan outcome, 2)
	send := func(name string, delay int) {
		response, err := c.Call(ctx, map[string]any{"type": "echo", "delayMs": delay, "payload": name})
		finished <- outcome{name, response, err}
	}
	go send("slow", 400)
	go send("fast", 0)
	first := <-finished
	second := <-finished
	if first.name != "fast" || second.name != "slow" {
		t.Fatalf("responses arrived in send order: %s then %s", first.name, second.name)
	}
	for _, o := range []outcome{first, second} {
		if o.err != nil {
			t.Fatalf("%s: %v", o.name, o.err)
		}
		if o.response.Command != "echo" || !o.response.Success {
			t.Fatalf("%s: response %+v", o.name, o.response)
		}
		if !strings.Contains(string(o.response.Data), `"payload":"`+o.name+`"`) {
			t.Fatalf("%s: data %s is another command's answer", o.name, o.response.Data)
		}
	}
	c.mu.Lock()
	pending := len(c.pending)
	c.mu.Unlock()
	if pending != 0 {
		t.Fatalf("%d pending slots left after both answered", pending)
	}
}

func TestCallRequiresType(t *testing.T) {
	c := startFake(t)
	if _, err := c.Call(testContext(t), map[string]any{"message": "x"}); err == nil {
		t.Fatal("a command without a type was sent")
	}
}

func TestCallKeepsCallerID(t *testing.T) {
	c := startFake(t)
	response, err := c.Call(testContext(t), map[string]any{"type": "get_state", "id": "mine-7"})
	if err != nil {
		t.Fatal(err)
	}
	if response.ID != "mine-7" {
		t.Fatalf("id %q, want the caller's", response.ID)
	}
}

func TestPromptStreamsEventsUntilSettled(t *testing.T) {
	c := startFake(t)
	ctx := testContext(t)
	col := collect(c)
	disposition, err := c.Prompt(ctx, "hello", PromptOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if disposition != DispositionStarted {
		t.Fatalf("disposition %q", disposition)
	}
	aborted, err := c.WaitSettled(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if aborted {
		t.Fatal("aborted")
	}
	types := col.types()
	want := []string{"agent_start", "turn_start", "message_start", "message_update", "message_end", "tool_execution_start", "tool_execution_end", "message_end", "turn_end", "turn_start", "message_start", "message_update", "message_end", "turn_end", "agent_end", "agent_settled"}
	if strings.Join(types, ",") != strings.Join(want, ",") {
		t.Fatalf("events\n got %v\nwant %v", types, want)
	}
}

func TestResponsesAreNotEvents(t *testing.T) {
	c := startFake(t)
	col := collect(c)
	if _, err := c.GetState(testContext(t)); err != nil {
		t.Fatal(err)
	}
	if types := col.types(); len(types) != 0 {
		t.Fatalf("a matched response was dispatched as an event: %v", types)
	}
}

func TestCancellationCleansPendingSlot(t *testing.T) {
	c := startFake(t)
	col := collect(c)
	ctx, cancel := context.WithTimeout(testContext(t), 100*time.Millisecond)
	defer cancel()
	_, err := c.Call(ctx, map[string]any{"type": "echo", "delayMs": 700, "payload": "late"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err %v, want the context's", err)
	}
	c.mu.Lock()
	pending := len(c.pending)
	c.mu.Unlock()
	if pending != 0 {
		t.Fatalf("%d pending slots after cancellation", pending)
	}
	// The client stays usable, and the late answer surfaces as an event.
	if _, err := c.GetState(testContext(t)); err != nil {
		t.Fatalf("call after cancellation: %v", err)
	}
	col.waitFor(t, EventResponse)
	col.mu.Lock()
	last := col.events[len(col.events)-1]
	col.mu.Unlock()
	response, err := last.Response()
	if err != nil {
		t.Fatal(err)
	}
	if response.Command != "echo" || !strings.Contains(string(response.Data), "late") {
		t.Fatalf("late response event %+v", response)
	}
}

func TestUnmatchedResponseIsEvent(t *testing.T) {
	c := startFake(t)
	col := collect(c)
	if err := c.write([]byte("not json\n")); err != nil {
		t.Fatal(err)
	}
	col.waitFor(t, EventResponse)
	col.mu.Lock()
	event := col.events[0]
	col.mu.Unlock()
	response, err := event.Response()
	if err != nil {
		t.Fatal(err)
	}
	if response.Command != "parse" || response.Success || response.ID != "" {
		t.Fatalf("parse failure record %+v", response)
	}
}

func TestFailedResponseIsCommandError(t *testing.T) {
	c := startFake(t)
	err := callOnly(c, testContext(t), map[string]any{"type": "bogus"})
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("err %v, want *CommandError", err)
	}
	if cmdErr.Command != "bogus" || cmdErr.Message != "Unknown command: bogus" {
		t.Fatalf("%+v", cmdErr)
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("message %q", err.Error())
	}
}

func TestCloseWaitsForVoluntaryExit(t *testing.T) {
	c := startFake(t)
	if _, err := c.GetState(testContext(t)); err != nil {
		t.Fatal(err)
	}
	c.Close()
	select {
	case <-c.Done():
	default:
		t.Fatal("Done is open after Close")
	}
	if err := c.Wait(); err != nil {
		t.Fatalf("a child that honored the closed input was killed: %v", err)
	}
	if _, err := c.GetState(testContext(t)); !errors.Is(err, ErrClosed) {
		t.Fatalf("call after Close: %v, want ErrClosed", err)
	}
}

func TestCloseKillsChildThatIgnoresStdin(t *testing.T) {
	c := startFake(t, fakeIgnoreEOFEnv+"=1")
	if _, err := c.GetState(testContext(t)); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	c.Close()
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("Close took %v", elapsed)
	}
	var exitErr *exec.ExitError
	if err := c.Wait(); !errors.As(err, &exitErr) {
		t.Fatalf("Wait: %v, want an exit error from the kill", err)
	}
	c.Close() // idempotent
}

func TestChildExitFailsPendingCalls(t *testing.T) {
	c := startFake(t)
	_, err := c.Call(testContext(t), map[string]any{"type": "die"})
	if !errors.Is(err, ErrExited) {
		t.Fatalf("err %v, want ErrExited", err)
	}
	select {
	case <-c.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("Done did not close after the child exited")
	}
	var exitErr *exec.ExitError
	if err := c.Wait(); !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Fatalf("Wait: %v, want exit status 3", err)
	}
}

func TestStartFailsWhenChildCannotRun(t *testing.T) {
	_, err := Start(context.Background(), Options{Pig: "/nonexistent/pig"})
	if err == nil || !strings.Contains(err.Error(), "override") {
		t.Fatalf("err %v", err)
	}
}

func TestStartContextEndsChild(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c, err := Start(ctx, Options{Pig: os.Args[0], Env: fakeEnv(), CloseGrace: 300 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	cancel()
	select {
	case <-c.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("the client outlived its context")
	}
}

func TestStderrIsDrained(t *testing.T) {
	var buf bytes.Buffer
	c, err := Start(context.Background(), Options{Pig: os.Args[0], Env: fakeEnv(fakeStderrEnv + "=diagnostic line"), Stderr: &buf, CloseGrace: 300 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if !strings.Contains(buf.String(), "diagnostic line") {
		t.Fatalf("stderr %q", buf.String())
	}
}

func TestSubscribeUnsubscribe(t *testing.T) {
	c := startFake(t)
	var count int
	var mu sync.Mutex
	unsubscribe := c.Subscribe(func(Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})
	unsubscribe()
	unsubscribe()
	if _, err := c.Run(testContext(t), "hello", PromptOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if count != 0 {
		t.Fatalf("%d events after unsubscribe", count)
	}
}

func TestPID(t *testing.T) {
	c := startFake(t)
	if c.PID() <= 0 {
		t.Fatalf("pid %d", c.PID())
	}
}
