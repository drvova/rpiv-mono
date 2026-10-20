package rpc

import (
	"context"
	"strings"
	"sync"
)

// RunResult is what a prompt produced by the time its run settled.
type RunResult struct {
	// Disposition is the prompt's disposition. For DispositionHandled no
	// run happened and the other fields are zero.
	Disposition string
	// LastAssistantText is the trimmed text of the last assistant message
	// of the run whose text is not blank, concatenated as
	// get_last_assistant_text concatenates it; empty when none had text.
	LastAssistantText string
	// Aborted is agent_settled's aborted flag.
	Aborted bool
	// Stats is get_session_stats read after the run settled, nil when that
	// command failed.
	Stats *SessionStats
	// ToolCalls counts the tool_execution_start events of the run.
	ToolCalls int
	// Usage sums the usage of the run's assistant message_end records.
	Usage Usage
}

// WaitSettled blocks until the next agent_settled event and reports whether
// the run was aborted. It returns ctx.Err() when ctx ends first, and ErrClosed
// or ErrExited when the child is gone.
func (c *Client) WaitSettled(ctx context.Context) (aborted bool, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	settled := make(chan AgentSettledEvent, 1)
	var once sync.Once
	unsubscribe := c.Subscribe(func(event Event) {
		if event.Type != EventAgentSettled {
			return
		}
		decoded, _ := event.AgentSettled()
		once.Do(func() { settled <- decoded })
	})
	defer unsubscribe()
	return c.awaitSettled(ctx, settled)
}

// awaitSettled waits for one settle delivery, the context or the child's end.
func (c *Client) awaitSettled(ctx context.Context, settled <-chan AgentSettledEvent) (bool, error) {
	select {
	case event := <-settled:
		return event.Aborted, nil
	case <-ctx.Done():
		return false, ctx.Err()
	case <-c.readerDone:
		select {
		case event := <-settled:
			return event.Aborted, nil
		default:
		}
		return false, c.terminalError()
	}
}

// Run sends message as a prompt and follows the run to its agent_settled
// event. Every event of the run goes to onEvent (when not nil) on the reader
// goroutine, before Run accounts it. The subscription is in place before the
// prompt is sent, so no event is missed.
//
// A prompt the child answers with DispositionHandled returns at once with
// only Disposition set. A DispositionQueued prompt waits for the next
// agent_settled, which belongs to the run ahead of it when queue delivery is
// one-at-a-time.
func (c *Client) Run(ctx context.Context, message string, opts PromptOptions, onEvent func(Event)) (RunResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var (
		mu      sync.Mutex
		result  RunResult
		once    sync.Once
		settled = make(chan AgentSettledEvent, 1)
	)
	unsubscribe := c.Subscribe(func(event Event) {
		if onEvent != nil {
			onEvent(event)
		}
		mu.Lock()
		defer mu.Unlock()
		switch event.Type {
		case EventMessageEnd:
			message, err := event.Message()
			if err != nil || message.Role != "assistant" {
				return
			}
			if message.Usage != nil {
				result.Usage.Add(*message.Usage)
			}
			if text := strings.TrimSpace(message.Text()); text != "" {
				result.LastAssistantText = text
			}
		case EventToolExecutionStart:
			result.ToolCalls++
		case EventAgentSettled:
			decoded, _ := event.AgentSettled()
			once.Do(func() { settled <- decoded })
		}
	})
	defer unsubscribe()

	disposition, err := c.Prompt(ctx, message, opts)
	if err != nil {
		return RunResult{}, err
	}
	if disposition == DispositionHandled {
		return RunResult{Disposition: disposition}, nil
	}
	aborted, err := c.awaitSettled(ctx, settled)
	if err != nil {
		return RunResult{}, err
	}
	unsubscribe()
	mu.Lock()
	out := result
	mu.Unlock()
	out.Disposition = disposition
	out.Aborted = aborted
	if stats, err := c.GetSessionStats(ctx); err == nil {
		out.Stats = &stats
	}
	return out, nil
}
