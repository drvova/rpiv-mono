package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// The test binary doubles as a fake pig: when fakePigEnv is set, TestMain runs
// the protocol from standard input instead of the tests. The knobs below
// select scripted behaviors.
const (
	fakePigEnv       = "RPIV_RPC_FAKE_PIG"
	fakeIgnoreEOFEnv = "RPIV_RPC_FAKE_IGNORE_EOF"
	fakeStderrEnv    = "RPIV_RPC_FAKE_STDERR"
	fakeExitEnv      = "RPIV_RPC_FAKE_EXIT_ON_START"
	fakeHolderEnv    = "RPIV_RPC_FAKE_HOLDER"
)

func TestMain(m *testing.M) {
	if os.Getenv(fakePigEnv) != "" {
		os.Exit(runFakePig())
	}
	os.Exit(m.Run())
}

// fakeEnv is the environment that turns the test binary into the fake pig.
// The race runtime's one-second sleep at exit is disabled so that a child
// honoring a closed input exits within the tests' short close grace.
func fakeEnv(extra ...string) []string {
	return append([]string{fakePigEnv + "=1", "GORACE=atexit_sleep_ms=0"}, extra...)
}

// startFake starts the test binary as a fake pig with a short close grace.
func startFake(t *testing.T, extraEnv ...string) *Client {
	t.Helper()
	opts := Options{
		Pig:        os.Args[0],
		Env:        fakeEnv(extraEnv...),
		CloseGrace: 300 * time.Millisecond,
	}
	c, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

// testContext returns a context that fails the test on a hang.
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// fakePig speaks the RPC protocol with scripted answers. Every command runs on
// its own goroutine so answers can arrive out of send order.
type fakePig struct {
	out sync.Mutex

	mu        sync.Mutex
	uiPending map[string]chan map[string]any
	uiSeq     int

	abort     chan struct{}
	abortOnce sync.Once
	closed    chan struct{}
}

func runFakePig() int {
	if code := os.Getenv(fakeExitEnv); code != "" {
		n, _ := strconv.Atoi(code)
		return n
	}
	if text := os.Getenv(fakeStderrEnv); text != "" {
		fmt.Fprintln(os.Stderr, text)
	}
	f := &fakePig{
		uiPending: make(map[string]chan map[string]any),
		abort:     make(chan struct{}),
		closed:    make(chan struct{}),
	}
	if os.Getenv(fakeHolderEnv) == "1" {
		holder := exec.Command("sleep", "60")
		if err := holder.Start(); err != nil {
			fmt.Fprintln(os.Stderr, "holder:", err)
			return 2
		}
		f.emit(map[string]any{"type": "holder", "pid": holder.Process.Pid})
	}
	var handlers sync.WaitGroup
	_ = ReadLines(os.Stdin, func(line []byte) error {
		var cmd map[string]any
		if err := json.Unmarshal(line, &cmd); err != nil || cmd == nil {
			f.emit(map[string]any{"type": "response", "command": "parse", "success": false, "error": "Failed to parse command: " + string(line)})
			return nil
		}
		switch cmd["type"] {
		case "extension_ui_response":
			f.resolveUI(cmd)
		case "die":
			os.Exit(3)
		default:
			handlers.Add(1)
			go func() {
				defer handlers.Done()
				f.handle(cmd)
			}()
		}
		return nil
	})
	if os.Getenv(fakeIgnoreEOFEnv) == "1" {
		time.Sleep(60 * time.Second)
	}
	close(f.closed)
	handlers.Wait()
	return 0
}

func (f *fakePig) emit(record map[string]any) {
	payload, err := encodeRecord(record)
	if err != nil {
		panic(err)
	}
	f.out.Lock()
	defer f.out.Unlock()
	_, _ = os.Stdout.Write(payload)
}

func (f *fakePig) resolveUI(response map[string]any) {
	id, _ := response["id"].(string)
	f.mu.Lock()
	ch, ok := f.uiPending[id]
	delete(f.uiPending, id)
	f.mu.Unlock()
	if ok {
		ch <- response
	}
}

// dialog writes an extension_ui_request with the given members and waits for
// its response. A closed input cancels it.
func (f *fakePig) dialog(members map[string]any) map[string]any {
	f.mu.Lock()
	f.uiSeq++
	id := fmt.Sprintf("ui-%d", f.uiSeq)
	ch := make(chan map[string]any, 1)
	f.uiPending[id] = ch
	f.mu.Unlock()
	request := map[string]any{"type": "extension_ui_request", "id": id}
	for key, value := range members {
		if value != nil {
			request[key] = value
		}
	}
	f.emit(request)
	select {
	case response := <-ch:
		return response
	case <-f.closed:
		return map[string]any{"cancelled": true, "inputClosed": true}
	}
}

var (
	fakeUsage1 = map[string]any{"input": 10, "output": 5, "cacheRead": 1, "cacheWrite": 2, "totalTokens": 18, "cost": map[string]any{"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0, "total": 0}}
	fakeUsage2 = map[string]any{"input": 20, "output": 7, "cacheRead": 3, "cacheWrite": 4, "totalTokens": 34, "cost": map[string]any{"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0, "total": 0}}
	fakeState  = map[string]any{
		"model":                 map[string]any{"id": "mock-1", "name": "Mock", "api": "openai-completions", "provider": "mock", "reasoning": false, "input": []string{"text"}, "contextWindow": 128000, "maxTokens": 4096},
		"thinkingLevel":         "off",
		"isStreaming":           false,
		"isCompacting":          false,
		"steeringMode":          "one-at-a-time",
		"followUpMode":          "one-at-a-time",
		"sessionId":             "fake-session",
		"autoCompactionEnabled": true,
		"messageCount":          0,
		"pendingMessageCount":   0,
	}
	fakeStats = map[string]any{
		"sessionId": "fake-session", "userMessages": 1, "assistantMessages": 2, "toolCalls": 1, "toolResults": 1, "totalMessages": 5,
		"tokens":       map[string]any{"input": 30, "output": 12, "cacheRead": 4, "cacheWrite": 6, "total": 52},
		"cost":         0.001,
		"contextUsage": map[string]any{"tokens": 52, "contextWindow": 128000, "percent": 0.04},
	}
)

func (f *fakePig) handle(cmd map[string]any) {
	id := cmd["id"]
	typ, _ := cmd["type"].(string)
	respond := func(success bool, data any, errText string) {
		record := map[string]any{"type": "response", "command": typ, "success": success}
		if id != nil {
			record["id"] = id
		}
		if data != nil {
			record["data"] = data
		}
		if errText != "" {
			record["error"] = errText
		}
		f.emit(record)
	}
	switch typ {
	case "get_state":
		respond(true, fakeState, "")
	case "get_session_stats":
		respond(true, fakeStats, "")
	case "get_last_assistant_text":
		respond(true, map[string]any{"text": "Hello from fake"}, "")
	case "echo":
		delay, _ := cmd["delayMs"].(float64)
		time.Sleep(time.Duration(delay) * time.Millisecond)
		respond(true, map[string]any{"payload": cmd["payload"]}, "")
	case "abort":
		f.abortOnce.Do(func() { close(f.abort) })
		respond(true, nil, "")
	case "bash":
		command, _ := cmd["command"].(string)
		var output strings.Builder
		for i := 1; i <= 3; i++ {
			delta := fmt.Sprintf("%s:%d\n", command, i)
			output.WriteString(delta)
			f.emit(map[string]any{"type": "bash_execution_update", "id": id, "delta": delta})
			time.Sleep(2 * time.Millisecond)
		}
		respond(true, map[string]any{"output": output.String(), "exitCode": 0, "cancelled": false, "truncated": false}, "")
	case "ask":
		members := map[string]any{}
		for _, key := range []string{"method", "title", "message", "options", "placeholder", "prefill", "timeout"} {
			members[key] = cmd[key]
		}
		response := f.dialog(members)
		respond(true, map[string]any{"response": response}, "")
	case "notify_me":
		f.emit(map[string]any{"type": "extension_ui_request", "id": "n-1", "method": "notify", "message": "hello", "notifyType": "warning"})
		f.emit(map[string]any{"type": "extension_ui_request", "id": "n-2", "method": "setStatus", "statusKey": "k", "statusText": "busy"})
		f.emit(map[string]any{"type": "extension_ui_request", "id": "n-3", "method": "setWidget", "widgetKey": "w", "widgetLines": []string{"a", "b"}, "widgetPlacement": "aboveEditor"})
		f.emit(map[string]any{"type": "extension_ui_request", "id": "n-4", "method": "setWidget", "widgetKey": "w"})
		respond(true, nil, "")
	case "prompt":
		f.prompt(cmd, respond)
	default:
		respond(false, nil, "Unknown command: "+typ)
	}
}

func (f *fakePig) prompt(cmd map[string]any, respond func(bool, any, string)) {
	message, _ := cmd["message"].(string)
	switch {
	case message == "/handled":
		respond(true, map[string]any{"disposition": "handled"}, "")
		return
	case cmd["streamingBehavior"] != nil:
		respond(true, map[string]any{"disposition": "queued"}, "")
		return
	}
	respond(true, map[string]any{"disposition": "started"}, "")
	f.emit(map[string]any{"type": "agent_start"})
	if message == "slow" {
		select {
		case <-f.abort:
		case <-f.closed:
		}
		f.emit(map[string]any{"type": "agent_end", "messages": []any{}, "willRetry": false})
		f.emit(map[string]any{"type": "agent_settled", "aborted": true})
		return
	}
	toolCall := map[string]any{"type": "toolCall", "id": "call_1", "name": "bash", "arguments": map[string]any{"command": "true"}}
	assistant1 := map[string]any{"role": "assistant", "content": []any{toolCall}, "usage": fakeUsage1, "stopReason": "toolUse"}
	toolResult := map[string]any{"role": "toolResult", "toolCallId": "call_1", "toolName": "bash", "content": []any{map[string]any{"type": "text", "text": ""}}, "isError": false}
	f.emit(map[string]any{"type": "turn_start"})
	f.emit(map[string]any{"type": "message_start", "message": map[string]any{"role": "assistant", "content": []any{}}})
	f.emit(map[string]any{"type": "message_update", "usage": fakeUsage1, "assistantMessageEvent": map[string]any{"type": "toolcall_start", "contentIndex": 0, "id": "call_1", "toolName": "bash"}})
	f.emit(map[string]any{"type": "message_end", "message": assistant1})
	f.emit(map[string]any{"type": "tool_execution_start", "toolCallId": "call_1", "toolName": "bash", "args": map[string]any{"command": "true"}})
	f.emit(map[string]any{"type": "tool_execution_end", "toolCallId": "call_1", "toolName": "bash", "result": map[string]any{"content": []any{}, "details": map[string]any{}}, "isError": false, "durationMs": 3})
	f.emit(map[string]any{"type": "message_end", "message": toolResult})
	f.emit(map[string]any{"type": "turn_end", "message": assistant1, "toolResults": []any{toolResult}})
	text := "Hello from fake"
	if strings.HasPrefix(message, "dialog") {
		response := f.dialog(map[string]any{"method": "select", "title": "Pick one", "options": []string{"Allow", "Block"}})
		value, _ := response["value"].(string)
		if response["cancelled"] == true {
			value = "nothing"
		}
		text = "You chose " + value
	}
	assistant2 := map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": "  " + text + "\n"}}, "usage": fakeUsage2, "stopReason": "stop"}
	f.emit(map[string]any{"type": "turn_start"})
	f.emit(map[string]any{"type": "message_start", "message": map[string]any{"role": "assistant", "content": []any{}}})
	f.emit(map[string]any{"type": "message_update", "usage": fakeUsage2, "assistantMessageEvent": map[string]any{"type": "text_delta", "contentIndex": 0, "delta": text}})
	f.emit(map[string]any{"type": "message_end", "message": assistant2})
	f.emit(map[string]any{"type": "turn_end", "message": assistant2, "toolResults": []any{}})
	f.emit(map[string]any{"type": "agent_end", "messages": []any{assistant1, toolResult, assistant2}, "willRetry": false})
	f.emit(map[string]any{"type": "agent_settled", "aborted": false})
}
