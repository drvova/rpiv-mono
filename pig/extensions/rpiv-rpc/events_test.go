package rpc

import (
	"encoding/json"
	"testing"
)

func event(typ, raw string) Event { return Event{Type: typ, Raw: json.RawMessage(raw)} }

func TestRequestIDDecoding(t *testing.T) {
	cases := map[string]string{
		`"rpiv-3"`: "rpiv-3",
		`7`:        "7",
		`null`:     "null",
		`"a\"b"`:   `a"b`,
	}
	for raw, want := range cases {
		var id RequestID
		if err := json.Unmarshal([]byte(raw), &id); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if string(id) != want {
			t.Errorf("%s: %q, want %q", raw, id, want)
		}
	}
}

func TestMessageDecoding(t *testing.T) {
	raw := `{"role":"assistant","content":[{"type":"thinking","thinking":"hm"},{"type":"text","text":"Hello"},{"type":"toolCall","id":"call_1","name":"read","arguments":{"path":"a.txt"}},{"type":"text","text":" world"}],"api":"openai-completions","provider":"mock","model":"mock-1","usage":{"input":100,"output":20,"cacheRead":0,"cacheWrite":0,"totalTokens":120,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"toolUse","timestamp":1}`
	var message Message
	if err := json.Unmarshal([]byte(raw), &message); err != nil {
		t.Fatal(err)
	}
	if message.Text() != "Hello world" {
		t.Fatalf("text %q", message.Text())
	}
	calls := message.ToolCalls()
	if len(calls) != 1 || calls[0].Name != "read" || string(calls[0].Arguments) != `{"path":"a.txt"}` {
		t.Fatalf("tool calls %+v", calls)
	}
	if message.Usage == nil || message.Usage.TotalTokens != 120 || message.Usage.Cost == nil || message.StopReason != "toolUse" || message.Provider != "mock" {
		t.Fatalf("%+v", message)
	}
	if string(message.Raw) != raw || string(message.Content[0].Raw) != `{"type":"thinking","thinking":"hm"}` {
		t.Fatal("raw not retained")
	}

	var bash Message
	if err := json.Unmarshal([]byte(`{"role":"bashExecution","command":"ls","output":"ok","exitCode":0}`), &bash); err != nil {
		t.Fatal(err)
	}
	if bash.Role != "bashExecution" || bash.Content != nil || bash.Text() != "" {
		t.Fatalf("%+v", bash)
	}

	var custom Message
	if err := json.Unmarshal([]byte(`{"role":"custom","content":"plain text"}`), &custom); err != nil {
		t.Fatalf("string content: %v", err)
	}
	if custom.Content != nil {
		t.Fatalf("%+v", custom)
	}
}

func TestEventDecoders(t *testing.T) {
	settled, err := event(EventAgentSettled, `{"type":"agent_settled","aborted":true}`).AgentSettled()
	if err != nil || !settled.Aborted {
		t.Fatalf("%+v %v", settled, err)
	}

	end, err := event(EventToolExecutionEnd, `{"type":"tool_execution_end","toolCallId":"call-1","toolName":"bash","result":{"content":[{"type":"text","text":"out"}],"isError":true},"isError":true,"durationMs":12}`).ToolExecution()
	if err != nil || end.ToolCallID != "call-1" || !end.IsError || end.DurationMs == nil || *end.DurationMs != 12 || len(end.Result) == 0 {
		t.Fatalf("%+v %v", end, err)
	}
	start, err := event(EventToolExecutionStart, `{"type":"tool_execution_start","toolCallId":"call-1","toolName":"bash","args":{"command":"ls"}}`).ToolExecution()
	if err != nil || start.DurationMs != nil || string(start.Args) != `{"command":"ls"}` {
		t.Fatalf("%+v %v", start, err)
	}

	update, err := event(EventMessageUpdate, `{"type":"message_update","usage":{"input":1,"output":2,"cacheRead":0,"cacheWrite":0,"totalTokens":3},"assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"Hello"}}`).MessageUpdate()
	if err != nil || update.AssistantMessageEvent.Delta != "Hello" || update.Usage == nil || update.Usage.TotalTokens != 3 {
		t.Fatalf("%+v %v", update, err)
	}
	toolcall, err := event(EventMessageUpdate, `{"type":"message_update","assistantMessageEvent":{"type":"toolcall_end","contentIndex":0,"toolCall":{"type":"toolCall","id":"c","name":"read","arguments":{}}}}`).MessageUpdate()
	if err != nil || toolcall.AssistantMessageEvent.ToolCall == nil || toolcall.AssistantMessageEvent.ToolCall.Name != "read" {
		t.Fatalf("%+v %v", toolcall, err)
	}

	msg, err := event(EventMessageEnd, `{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"Hello"}],"stopReason":"stop"}}`).Message()
	if err != nil || msg.Text() != "Hello" {
		t.Fatalf("%+v %v", msg, err)
	}

	turn, err := event(EventTurnEnd, `{"type":"turn_end","message":{"role":"assistant","content":[]},"toolResults":[{"role":"toolResult","toolCallId":"c","toolName":"bash","content":[],"isError":false}]}`).TurnEnd()
	if err != nil || len(turn.ToolResults) != 1 || turn.ToolResults[0].ToolCallID != "c" {
		t.Fatalf("%+v %v", turn, err)
	}

	agentEnd, err := event(EventAgentEnd, `{"type":"agent_end","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}],"willRetry":true}`).AgentEnd()
	if err != nil || !agentEnd.WillRetry || len(agentEnd.Messages) != 1 {
		t.Fatalf("%+v %v", agentEnd, err)
	}

	bash, err := event(EventBashExecutionUpdate, `{"type":"bash_execution_update","id":"rpiv-9","delta":"out\n"}`).BashExecutionUpdate()
	if err != nil || bash.ID != "rpiv-9" || bash.Delta != "out\n" {
		t.Fatalf("%+v %v", bash, err)
	}

	queue, err := event(EventQueueUpdate, `{"type":"queue_update","steering":["a"],"followUp":[]}`).QueueUpdate()
	if err != nil || len(queue.Steering) != 1 || len(queue.FollowUp) != 0 {
		t.Fatalf("%+v %v", queue, err)
	}

	info, err := event(EventSessionInfoChanged, `{"type":"session_info_changed"}`).SessionInfoChanged()
	if err != nil || info.Name != nil {
		t.Fatalf("%+v %v", info, err)
	}
	named, err := event(EventSessionInfoChanged, `{"type":"session_info_changed","name":"x"}`).SessionInfoChanged()
	if err != nil || named.Name == nil || *named.Name != "x" {
		t.Fatalf("%+v %v", named, err)
	}

	level, err := event(EventThinkingLevel, `{"type":"thinking_level_changed","level":"high"}`).ThinkingLevelChanged()
	if err != nil || level.Level != "high" {
		t.Fatalf("%+v %v", level, err)
	}

	compaction, err := event(EventCompactionEnd, `{"type":"compaction_end","reason":"manual","result":{"summary":"s","firstKeptEntryId":"e","tokensBefore":10,"estimatedTokensAfter":5},"aborted":false,"willRetry":false}`).Compaction()
	if err != nil || compaction.Result == nil || compaction.Result.Summary != "s" || compaction.Result.EstimatedTokensAfter != 5 {
		t.Fatalf("%+v %v", compaction, err)
	}

	appended, err := event(EventEntryAppended, `{"type":"entry_appended","entry":{"type":"custom","id":"a7","parentId":"f6","timestamp":"t","customType":"review-state","data":{"count":42}}}`).EntryAppended()
	if err != nil || appended.Entry.Type != "custom" || appended.Entry.ParentID == nil || *appended.Entry.ParentID != "f6" || len(appended.Entry.Raw) == 0 {
		t.Fatalf("%+v %v", appended, err)
	}

	failure, err := event(EventError, `{"type":"error","message":"boom"}`).ErrorEvent()
	if err != nil || failure.Message != "boom" {
		t.Fatalf("%+v %v", failure, err)
	}

	var generic map[string]any
	if err := event(EventAgentStart, `{"type":"agent_start"}`).Decode(&generic); err != nil || generic["type"] != "agent_start" {
		t.Fatalf("%v %v", generic, err)
	}
}

func TestStateAndModelDecoding(t *testing.T) {
	raw := `{"model":{"id":"mock-1","name":"Mock","api":"openai-completions","provider":"mock","baseUrl":"http://127.0.0.1/v1","reasoning":false,"input":["text"],"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0},"contextWindow":128000,"maxTokens":4096},"thinkingLevel":"off","isStreaming":false,"isCompacting":false,"steeringMode":"one-at-a-time","followUpMode":"one-at-a-time","sessionId":"01a1","autoCompactionEnabled":true,"messageCount":0,"pendingMessageCount":0}`
	var state State
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		t.Fatal(err)
	}
	if state.SessionID != "01a1" || state.SessionFile != nil || state.SessionName != nil || state.Model == nil || state.Model.Qualified() != "mock/mock-1" || state.Model.ContextWindow != 128000 {
		t.Fatalf("%+v", state)
	}
	if string(state.Raw) != raw || !json.Valid(state.Model.Raw) {
		t.Fatal("raw not retained")
	}

	var stats SessionStats
	if err := json.Unmarshal([]byte(`{"sessionId":"s","userMessages":1,"assistantMessages":1,"toolCalls":0,"toolResults":0,"totalMessages":3,"tokens":{"input":100,"output":20,"cacheRead":0,"cacheWrite":0,"total":120},"cost":0,"contextUsage":{"tokens":120,"contextWindow":128000,"percent":0.09375}}`), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.Tokens.Total != 120 || stats.ContextUsage == nil || *stats.ContextUsage.Percent != 0.09375 {
		t.Fatalf("%+v", stats)
	}
}

func TestUsageAdd(t *testing.T) {
	var total Usage
	total.Add(MessageUsage{Input: 1, Output: 2, CacheRead: 3, CacheWrite: 4, TotalTokens: 11})
	total.Add(MessageUsage{Input: 10, Output: 20, CacheRead: 30, CacheWrite: 40, TotalTokens: 99})
	if total != (Usage{Input: 11, Output: 22, CacheRead: 33, CacheWrite: 44, Total: 110}) {
		t.Fatalf("%+v", total)
	}
}

func TestImageMarshal(t *testing.T) {
	data, err := json.Marshal(Image{Data: "AAAA", MimeType: "image/png"})
	if err != nil || string(data) != `{"type":"image","data":"AAAA","mimeType":"image/png"}` {
		t.Fatalf("%s %v", data, err)
	}
}
