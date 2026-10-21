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
	case <-c.exitedCh:
	default:
		t.Fatal("Close returned before the child exited")
	}
	waitDone(t, c)
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
	if !errors.Is(err, ErrExited) || !strings.Contains(err.Error(), "exit status 3") {
		t.Fatalf("err %v, want ErrExited naming exit status 3", err)
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
	if err := c.Wait(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "diagnostic line") {
		t.Fatalf("stderr %q", buf.String())
	}
}

func TestExitStatusInCommandError(t *testing.T) {
	// The child exits before it answers, which is how a startup failure
	// (unknown flag, missing model) looks: the error names the status
	// whether the command was written before or after the exit.
	c := startFake(t, fakeExitEnv+"=7")
	_, err := c.Call(testContext(t), map[string]any{"type": "get_state"})
	if !errors.Is(err, ErrExited) || !strings.Contains(err.Error(), "exit status 7") {
		t.Fatalf("err %v, want ErrExited naming exit status 7", err)
	}
	waitDone(t, c)
	var exitErr *exec.ExitError
	if err := c.Wait(); !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("Wait: %v", err)
	}
}

func TestCloseFromSubscriber(t *testing.T) {
	c := startFake(t)
	var mu sync.Mutex
	var seen []string
	returned := make(chan struct{})
	c.Subscribe(func(event Event) {
		mu.Lock()
		seen = append(seen, event.Type)
		mu.Unlock()
		if event.Type == EventAgentStart {
			c.Close()
			close(returned)
		}
	})
	if _, err := c.Prompt(testContext(t), "hello", PromptOptions{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-returned:
	case <-time.After(10 * time.Second):
		t.Fatal("Close called from a subscriber did not return")
	}
	waitDone(t, c)
	if err := c.Wait(); err != nil {
		t.Fatalf("the child did not finish on its own after Close: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 {
		t.Fatalf("events delivered after Close: %v", seen)
	}
}

func TestCloseFromSubscriberAtSettle(t *testing.T) {
	// The record being dispatched still reaches every subscriber, so a
	// WaitSettled subscribed after the closing one sees its settle.
	c := startFake(t)
	c.Subscribe(func(event Event) {
		if event.Type == EventAgentSettled {
			c.Close()
		}
	})
	ctx := testContext(t)
	if _, err := c.Prompt(ctx, "hello", PromptOptions{}); err != nil {
		t.Fatal(err)
	}
	aborted, err := c.WaitSettled(ctx)
	if err != nil || aborted {
		t.Fatalf("WaitSettled: %v, aborted %v", err, aborted)
	}
	waitDone(t, c)
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
