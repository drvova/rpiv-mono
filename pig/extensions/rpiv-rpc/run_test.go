package rpc

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRunAccumulates(t *testing.T) {
	c := startFake(t)
	var types []string
	var mu sync.Mutex
	result, err := c.Run(testContext(t), "hello", PromptOptions{Metadata: map[string]any{"task": "1"}}, func(event Event) {
		mu.Lock()
		types = append(types, event.Type)
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != DispositionStarted || result.Aborted {
		t.Fatalf("%+v", result)
	}
	if result.LastAssistantText != "Hello from fake" {
		t.Fatalf("text %q (trimmed concatenation expected)", result.LastAssistantText)
	}
	if result.ToolCalls != 1 {
		t.Fatalf("tool calls %d", result.ToolCalls)
	}
	if want := (Usage{Input: 30, Output: 12, CacheRead: 4, CacheWrite: 6, Total: 52}); result.Usage != want {
		t.Fatalf("usage %+v, want %+v", result.Usage, want)
	}
	if result.Stats == nil || result.Stats.SessionID != "fake-session" || result.Stats.Tokens.Total != 52 || result.Stats.ContextUsage == nil || *result.Stats.ContextUsage.Tokens != 52 {
		t.Fatalf("stats %+v", result.Stats)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(types) == 0 || types[0] != EventAgentStart || types[len(types)-1] != EventAgentSettled {
		t.Fatalf("onEvent saw %v", types)
	}
}

func TestRunHandledReturnsAtOnce(t *testing.T) {
	c := startFake(t)
	result, err := c.Run(testContext(t), "/handled", PromptOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != DispositionHandled || result.Stats != nil || result.LastAssistantText != "" {
		t.Fatalf("%+v", result)
	}
}

func TestRunAborted(t *testing.T) {
	c := startFake(t)
	col := collect(c)
	type outcome struct {
		result RunResult
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		result, err := c.Run(testContext(t), "slow", PromptOptions{}, nil)
		finished <- outcome{result, err}
	}()
	col.waitFor(t, EventAgentStart)
	if err := c.Abort(testContext(t)); err != nil {
		t.Fatal(err)
	}
	o := <-finished
	if o.err != nil {
		t.Fatal(o.err)
	}
	if !o.result.Aborted {
		t.Fatalf("%+v", o.result)
	}
}

func TestRunContextCancellation(t *testing.T) {
	c := startFake(t)
	ctx, cancel := context.WithTimeout(testContext(t), 200*time.Millisecond)
	defer cancel()
	_, err := c.Run(ctx, "slow", PromptOptions{}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err %v", err)
	}
}

func TestRunFailsWhenChildExits(t *testing.T) {
	c := startFake(t)
	col := collect(c)
	finished := make(chan error, 1)
	go func() {
		_, err := c.Run(testContext(t), "slow", PromptOptions{}, nil)
		finished <- err
	}()
	col.waitFor(t, EventAgentStart)
	_, _ = c.Call(testContext(t), map[string]any{"type": "die"})
	if err := <-finished; !errors.Is(err, ErrExited) {
		t.Fatalf("err %v, want ErrExited", err)
	}
}

func TestWaitSettledAfterClose(t *testing.T) {
	c := startFake(t)
	c.Close()
	if _, err := c.WaitSettled(testContext(t)); !errors.Is(err, ErrClosed) {
		t.Fatalf("err %v", err)
	}
}

func TestBashStreamsOutput(t *testing.T) {
	c := startFake(t)
	var deltas []string
	var mu sync.Mutex
	result, err := c.Bash(testContext(t), "echo hi", BashOptions{OnOutput: func(delta string) {
		mu.Lock()
		deltas = append(deltas, delta)
		mu.Unlock()
	}})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(deltas, "") != "echo hi:1\necho hi:2\necho hi:3\n" {
		t.Fatalf("deltas %q", deltas)
	}
	if result.Output != strings.Join(deltas, "") || result.ExitCode == nil || *result.ExitCode != 0 || result.Cancelled {
		t.Fatalf("result %+v", result)
	}
}

func TestBashStreamsOnlyItsOwnOutput(t *testing.T) {
	c := startFake(t)
	var wg sync.WaitGroup
	for _, command := range []string{"one", "two", "three"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var mine []string
			var mu sync.Mutex
			result, err := c.Bash(testContext(t), command, BashOptions{OnOutput: func(delta string) {
				mu.Lock()
				mine = append(mine, delta)
				mu.Unlock()
			}})
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, delta := range mine {
				if !strings.HasPrefix(delta, command+":") {
					t.Errorf("%s received %q", command, delta)
				}
			}
			if len(mine) != 3 || strings.Join(mine, "") != result.Output {
				t.Errorf("%s: deltas %q, output %q", command, mine, result.Output)
			}
		}()
	}
	wg.Wait()
}
