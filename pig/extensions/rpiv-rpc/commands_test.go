package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// newLoopback builds a Client over two pipes and no process: the test reads
// what the client writes from the returned reader and feeds records to the
// client through the returned writer. The exit watcher is not started, so
// Close must not be called on it.
func newLoopback(t *testing.T) (*Client, *bufio.Reader, *os.File) {
	t.Helper()
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		stdin:      stdinW,
		stdout:     stdoutR,
		pending:    make(map[string]chan Response),
		exitedCh:   make(chan struct{}),
		readerDone: make(chan struct{}),
		stderrDone: make(chan struct{}),
		done:       make(chan struct{}),
		ctx:        ctx,
		cancel:     cancel,
	}
	go c.readLoop()
	t.Cleanup(func() {
		_ = stdoutW.Close()
		<-c.readerDone
		cancel()
		_ = stdinW.Close()
		_ = stdinR.Close()
		_ = stdoutR.Close()
	})
	return c, bufio.NewReader(stdinR), stdoutW
}

// wireCase describes one typed wrapper: the command it must write (without
// the id), the data the child answers with, and the value the wrapper must
// return.
type wireCase struct {
	name string
	send func(ctx context.Context, c *Client) (any, error)
	want map[string]any
	data string
	out  any
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

func TestTypedWrappersWire(t *testing.T) {
	cases := []wireCase{
		{"prompt", func(ctx context.Context, c *Client) (any, error) {
			return c.Prompt(ctx, "Find the tests", PromptOptions{})
		}, map[string]any{"type": "prompt", "message": "Find the tests"}, `{"disposition":"started"}`, "started"},
		{"prompt with options", func(ctx context.Context, c *Client) (any, error) {
			return c.Prompt(ctx, "Review", PromptOptions{StreamingBehavior: StreamingSteer, Metadata: map[string]any{"task_id": "task-42"}, Images: []Image{{Data: "AA", MimeType: "image/png"}}})
		}, map[string]any{"type": "prompt", "message": "Review", "streamingBehavior": "steer", "metadata": map[string]any{"task_id": "task-42"}, "images": []any{map[string]any{"type": "image", "data": "AA", "mimeType": "image/png"}}}, `{"disposition":"queued"}`, "queued"},
		{"steer", func(ctx context.Context, c *Client) (any, error) {
			return c.Steer(ctx, "Stop", nil)
		}, map[string]any{"type": "steer", "message": "Stop"}, `{"disposition":"queued"}`, "queued"},
		{"follow_up", func(ctx context.Context, c *Client) (any, error) {
			return c.FollowUp(ctx, "Then docs", []Image{})
		}, map[string]any{"type": "follow_up", "message": "Then docs", "images": []any{}}, `{"disposition":"handled"}`, "handled"},
		{"abort", func(ctx context.Context, c *Client) (any, error) {
			return nil, c.Abort(ctx)
		}, map[string]any{"type": "abort"}, ``, nil},
		{"clear_queue", func(ctx context.Context, c *Client) (any, error) {
			return c.ClearQueue(ctx)
		}, map[string]any{"type": "clear_queue"}, `{"steering":["a"],"followUp":["b"]}`, ClearQueueResult{Steering: []string{"a"}, FollowUp: []string{"b"}}},
		{"new_session", func(ctx context.Context, c *Client) (any, error) {
			return c.NewSession(ctx, "")
		}, map[string]any{"type": "new_session"}, `{"cancelled":false}`, false},
		{"new_session with parent", func(ctx context.Context, c *Client) (any, error) {
			return c.NewSession(ctx, "/p.jsonl")
		}, map[string]any{"type": "new_session", "parentSession": "/p.jsonl"}, `{"cancelled":true}`, true},
		{"switch_session", func(ctx context.Context, c *Client) (any, error) {
			return c.SwitchSession(ctx, "/s.jsonl")
		}, map[string]any{"type": "switch_session", "sessionPath": "/s.jsonl"}, `{"cancelled":false}`, false},
		{"fork", func(ctx context.Context, c *Client) (any, error) {
			return c.Fork(ctx, "a1b2")
		}, map[string]any{"type": "fork", "entryId": "a1b2"}, `{"text":"hello","cancelled":false}`, ForkResult{Text: "hello"}},
		{"clone", func(ctx context.Context, c *Client) (any, error) {
			return c.Clone(ctx)
		}, map[string]any{"type": "clone"}, `{"cancelled":false}`, false},
		{"get_fork_messages", func(ctx context.Context, c *Client) (any, error) {
			return c.GetForkMessages(ctx)
		}, map[string]any{"type": "get_fork_messages"}, `{"messages":[{"entryId":"e1","text":"hi"}]}`, []ForkMessage{{EntryID: "e1", Text: "hi"}}},
		{"get_state", func(ctx context.Context, c *Client) (any, error) {
			state, err := c.GetState(ctx)
			return state.SessionID + "/" + state.ThinkingLevel + "/" + *state.SessionName, err
		}, map[string]any{"type": "get_state"}, `{"sessionId":"s1","sessionName":"n","thinkingLevel":"medium","isStreaming":false}`, "s1/medium/n"},
		{"set_model", func(ctx context.Context, c *Client) (any, error) {
			model, err := c.SetModel(ctx, "openai", "gpt-5")
			return model.Qualified(), err
		}, map[string]any{"type": "set_model", "provider": "openai", "modelId": "gpt-5"}, `{"id":"gpt-5","provider":"openai","name":"GPT-5"}`, "openai/gpt-5"},
		{"cycle_model", func(ctx context.Context, c *Client) (any, error) {
			return c.CycleModel(ctx)
		}, map[string]any{"type": "cycle_model"}, `{"model":{"provider":"p","id":"m"},"thinkingLevel":"low","isScoped":true}`, &CycleModelResult{Model: ModelRef{Provider: "p", ID: "m"}, ThinkingLevel: "low", IsScoped: true}},
		{"cycle_model nothing", func(ctx context.Context, c *Client) (any, error) {
			return c.CycleModel(ctx)
		}, map[string]any{"type": "cycle_model"}, `null`, (*CycleModelResult)(nil)},
		{"get_available_models", func(ctx context.Context, c *Client) (any, error) {
			models, err := c.GetAvailableModels(ctx)
			return len(models), err
		}, map[string]any{"type": "get_available_models"}, `{"models":[{"id":"a","provider":"p"},{"id":"b","provider":"p"}]}`, 2},
		{"set_thinking_level", func(ctx context.Context, c *Client) (any, error) {
			return nil, c.SetThinkingLevel(ctx, "high")
		}, map[string]any{"type": "set_thinking_level", "level": "high"}, ``, nil},
		{"cycle_thinking_level", func(ctx context.Context, c *Client) (any, error) {
			return c.CycleThinkingLevel(ctx)
		}, map[string]any{"type": "cycle_thinking_level"}, `{"level":"xhigh"}`, &CycleThinkingLevelResult{Level: "xhigh"}},
		{"get_available_thinking_levels", func(ctx context.Context, c *Client) (any, error) {
			return c.GetAvailableThinkingLevels(ctx)
		}, map[string]any{"type": "get_available_thinking_levels"}, `{"levels":["off","low"]}`, []string{"off", "low"}},
		{"set_steering_mode", func(ctx context.Context, c *Client) (any, error) {
			return nil, c.SetSteeringMode(ctx, QueueModeAll)
		}, map[string]any{"type": "set_steering_mode", "mode": "all"}, ``, nil},
		{"set_follow_up_mode", func(ctx context.Context, c *Client) (any, error) {
			return nil, c.SetFollowUpMode(ctx, QueueModeOneAtATime)
		}, map[string]any{"type": "set_follow_up_mode", "mode": "one-at-a-time"}, ``, nil},
		{"compact", func(ctx context.Context, c *Client) (any, error) {
			return c.Compact(ctx, "Keep file names.")
		}, map[string]any{"type": "compact", "customInstructions": "Keep file names."}, `{"summary":"s","firstKeptEntryId":"e","tokensBefore":50,"estimatedTokensAfter":10,"usage":{"input":1},"details":{"readFiles":[]}}`, CompactionResult{Summary: "s", FirstKeptEntryID: "e", TokensBefore: 50, EstimatedTokensAfter: 10, Usage: json.RawMessage(`{"input":1}`), Details: json.RawMessage(`{"readFiles":[]}`)}},
		{"compact plain", func(ctx context.Context, c *Client) (any, error) {
			r, err := c.Compact(ctx, "")
			return r.Summary, err
		}, map[string]any{"type": "compact"}, `{"summary":"s","firstKeptEntryId":"e","tokensBefore":1,"estimatedTokensAfter":1}`, "s"},
		{"set_auto_compaction", func(ctx context.Context, c *Client) (any, error) {
			return nil, c.SetAutoCompaction(ctx, false)
		}, map[string]any{"type": "set_auto_compaction", "enabled": false}, ``, nil},
		{"set_auto_retry", func(ctx context.Context, c *Client) (any, error) {
			return nil, c.SetAutoRetry(ctx, true)
		}, map[string]any{"type": "set_auto_retry", "enabled": true}, ``, nil},
		{"abort_retry", func(ctx context.Context, c *Client) (any, error) {
			return nil, c.AbortRetry(ctx)
		}, map[string]any{"type": "abort_retry"}, ``, nil},
		{"bash", func(ctx context.Context, c *Client) (any, error) {
			return c.Bash(ctx, "go test ./...", BashOptions{ExcludeFromContext: true})
		}, map[string]any{"type": "bash", "command": "go test ./...", "excludeFromContext": true}, `{"output":"ok","exitCode":0,"cancelled":false,"truncated":false,"fullOutputPath":"/tmp/o"}`, BashResult{Output: "ok", ExitCode: intPtr(0), FullOutputPath: "/tmp/o"}},
		{"bash killed", func(ctx context.Context, c *Client) (any, error) {
			return c.Bash(ctx, "sleep 9", BashOptions{})
		}, map[string]any{"type": "bash", "command": "sleep 9"}, `{"output":"","cancelled":true,"truncated":false}`, BashResult{Cancelled: true}},
		{"abort_bash", func(ctx context.Context, c *Client) (any, error) {
			return nil, c.AbortBash(ctx)
		}, map[string]any{"type": "abort_bash"}, ``, nil},
		{"get_session_stats", func(ctx context.Context, c *Client) (any, error) {
			stats, err := c.GetSessionStats(ctx)
			return stats.TotalMessages, err
		}, map[string]any{"type": "get_session_stats"}, `{"sessionId":"s","totalMessages":3,"tokens":{"total":1}}`, 3},
		{"export_html", func(ctx context.Context, c *Client) (any, error) {
			return c.ExportHTML(ctx, "/tmp/s.html")
		}, map[string]any{"type": "export_html", "outputPath": "/tmp/s.html"}, `{"path":"/tmp/s.html"}`, "/tmp/s.html"},
		{"export_html default", func(ctx context.Context, c *Client) (any, error) {
			return c.ExportHTML(ctx, "")
		}, map[string]any{"type": "export_html"}, `{"path":"/d.html"}`, "/d.html"},
		{"get_entries", func(ctx context.Context, c *Client) (any, error) {
			r, err := c.GetEntries(ctx, "a1")
			return len(r.Entries), err
		}, map[string]any{"type": "get_entries", "since": "a1"}, `{"entries":[{"type":"message","id":"b2","parentId":"a1","timestamp":"t"}],"leafId":"b2"}`, 1},
		{"get_entries all", func(ctx context.Context, c *Client) (any, error) {
			r, err := c.GetEntries(ctx, "")
			return r.LeafID, err
		}, map[string]any{"type": "get_entries"}, `{"entries":[],"leafId":null}`, (*string)(nil)},
		{"get_tree", func(ctx context.Context, c *Client) (any, error) {
			r, err := c.GetTree(ctx)
			return r.Tree[0].Children[0].Entry.ID + "/" + *r.LeafID, err
		}, map[string]any{"type": "get_tree"}, `{"tree":[{"entry":{"type":"message","id":"a","parentId":null,"timestamp":"t"},"children":[{"entry":{"type":"message","id":"b","parentId":"a","timestamp":"t"},"children":[],"label":"x"}]}],"leafId":"b"}`, "b/b"},
		{"get_last_assistant_text", func(ctx context.Context, c *Client) (any, error) {
			text, ok, err := c.GetLastAssistantText(ctx)
			return []any{text, ok}, err
		}, map[string]any{"type": "get_last_assistant_text"}, `{"text":"Hello"}`, []any{"Hello", true}},
		{"get_last_assistant_text none", func(ctx context.Context, c *Client) (any, error) {
			text, ok, err := c.GetLastAssistantText(ctx)
			return []any{text, ok}, err
		}, map[string]any{"type": "get_last_assistant_text"}, `{}`, []any{"", false}},
		{"set_session_name", func(ctx context.Context, c *Client) (any, error) {
			return nil, c.SetSessionName(ctx, "Review auth")
		}, map[string]any{"type": "set_session_name", "name": "Review auth"}, ``, nil},
		{"get_messages", func(ctx context.Context, c *Client) (any, error) {
			messages, err := c.GetMessages(ctx)
			return messages[1].Role + ":" + messages[1].Text(), err
		}, map[string]any{"type": "get_messages"}, `{"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]},{"role":"assistant","content":[{"type":"text","text":"yo"}]}]}`, "assistant:yo"},
		{"get_commands", func(ctx context.Context, c *Client) (any, error) {
			return c.GetCommands(ctx)
		}, map[string]any{"type": "get_commands"}, `{"commands":[{"name":"review","description":"d","source":"extension","sourceInfo":{"path":"/e","source":"extension","scope":"user","origin":"local"}}]}`, []SlashCommand{{Name: "review", Description: "d", Source: "extension", SourceInfo: SourceInfo{Path: "/e", Source: "extension", Scope: "user", Origin: "local"}}}},
		{"get_extensions", func(ctx context.Context, c *Client) (any, error) {
			return c.GetExtensions(ctx)
		}, map[string]any{"type": "get_extensions"}, `{"paths":["builtin:plan-mode","/x/ext"]}`, []string{"builtin:plan-mode", "/x/ext"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, from, to := newLoopback(t)
			type outcome struct {
				value any
				err   error
			}
			finished := make(chan outcome, 1)
			go func() {
				value, err := tc.send(testContext(t), c)
				finished <- outcome{value, err}
			}()
			line, err := from.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			var sent map[string]any
			if err := json.Unmarshal([]byte(line), &sent); err != nil {
				t.Fatalf("not a JSON record: %q", line)
			}
			id, _ := sent["id"].(string)
			if id == "" {
				t.Fatalf("no id in %q", line)
			}
			delete(sent, "id")
			if !reflect.DeepEqual(sent, tc.want) {
				t.Fatalf("sent %v\nwant %v", sent, tc.want)
			}
			response := `{"id":"` + id + `","type":"response","command":"` + tc.want["type"].(string) + `","success":true`
			if tc.data != "" {
				response += `,"data":` + tc.data
			}
			if _, err := to.Write([]byte(response + "}\n")); err != nil {
				t.Fatal(err)
			}
			o := <-finished
			if o.err != nil {
				t.Fatal(o.err)
			}
			if !reflect.DeepEqual(o.value, tc.out) {
				t.Fatalf("returned %#v\nwant     %#v", o.value, tc.out)
			}
		})
	}
}

func TestTypedWrapperFailure(t *testing.T) {
	c, from, to := newLoopback(t)
	finished := make(chan error, 1)
	go func() {
		_, err := c.GetEntries(testContext(t), "zzz")
		finished <- err
	}()
	line, err := from.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	var sent map[string]any
	_ = json.Unmarshal([]byte(line), &sent)
	if _, err := to.Write([]byte(`{"id":"` + sent["id"].(string) + `","type":"response","command":"get_entries","success":false,"error":"Entry not found: zzz"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	err = <-finished
	cmdErr, ok := err.(*CommandError)
	if !ok || cmdErr.Message != "Entry not found: zzz" || cmdErr.Command != "get_entries" {
		t.Fatalf("err %v", err)
	}
}

func TestCallDoesNotEscapeHTML(t *testing.T) {
	c, from, to := newLoopback(t)
	go func() {
		_, _ = c.Prompt(testContext(t), "a < b && c > d", PromptOptions{})
	}()
	line, err := from.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if want := `"message":"a < b && c > d"`; !contains(line, want) {
		t.Fatalf("wrote %q, want %s unescaped", line, want)
	}
	_, _ = to.Write([]byte(`{"id":"rpiv-1","type":"response","command":"prompt","success":true,"data":{"disposition":"started"}}` + "\n"))
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

var _ = strPtr
