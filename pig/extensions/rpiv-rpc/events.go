package rpc

import (
	"encoding/json"
	"strconv"
)

// Event is one stdout record that is not the response to a pending command:
// an agent session event, an extension UI request, or a response nobody waits
// for (one whose id was cancelled, or the child's `parse` failure record,
// which has no id). Type is the record's "type" member; Raw is the record.
type Event struct {
	Type string
	Raw  json.RawMessage
}

// Event types the child writes.
const (
	EventAgentStart          = "agent_start"
	EventAgentEnd            = "agent_end"
	EventAgentSettled        = "agent_settled"
	EventTurnStart           = "turn_start"
	EventTurnEnd             = "turn_end"
	EventMessageStart        = "message_start"
	EventMessageUpdate       = "message_update"
	EventMessageEnd          = "message_end"
	EventToolExecutionStart  = "tool_execution_start"
	EventToolExecutionUpdate = "tool_execution_update"
	EventToolExecutionEnd    = "tool_execution_end"
	EventBashExecutionUpdate = "bash_execution_update"
	EventQueueUpdate         = "queue_update"
	EventThinkingLevel       = "thinking_level_changed"
	EventCompactionStart     = "compaction_start"
	EventCompactionEnd       = "compaction_end"
	EventAutoRetryStart      = "auto_retry_start"
	EventAutoRetryEnd        = "auto_retry_end"
	EventEntryAppended       = "entry_appended"
	EventSessionInfoChanged  = "session_info_changed"
	EventExtensionUIRequest  = "extension_ui_request"
	EventExtensionError      = "extension_error"
	EventError               = "error"
	EventResponse            = "response"
)

// Decode unmarshals the record into v.
func (e Event) Decode(v any) error { return json.Unmarshal(e.Raw, v) }

// AgentSettledEvent is agent_settled: Aborted is true when the run ended
// because it was aborted.
type AgentSettledEvent struct {
	Aborted bool `json:"aborted"`
}

// AgentEndEvent is agent_end: the run's messages and whether a retry follows.
type AgentEndEvent struct {
	Messages  []Message `json:"messages"`
	WillRetry bool      `json:"willRetry"`
}

// MessageEvent is message_start, message_end or turn_end: the message they
// carry.
type MessageEvent struct {
	Message Message `json:"message"`
}

// TurnEndEvent is turn_end: the assistant message and the tool results of
// the turn.
type TurnEndEvent struct {
	Message     Message   `json:"message"`
	ToolResults []Message `json:"toolResults"`
}

// AssistantMessageEvent is the streamed piece a message_update carries. Type
// is start, text_start, text_delta, text_end, thinking_start, thinking_delta,
// thinking_end, toolcall_start, toolcall_delta, toolcall_end, done or error.
type AssistantMessageEvent struct {
	Type         string          `json:"type"`
	ContentIndex int             `json:"contentIndex"`
	Delta        string          `json:"delta"`
	Content      string          `json:"content"`
	ID           string          `json:"id"`
	ToolName     string          `json:"toolName"`
	ToolCall     *ContentBlock   `json:"toolCall"`
	Reason       string          `json:"reason"`
	Error        json.RawMessage `json:"error"`
}

// MessageUpdateEvent is message_update: the usage so far and the new piece.
type MessageUpdateEvent struct {
	Usage                 *MessageUsage         `json:"usage"`
	AssistantMessageEvent AssistantMessageEvent `json:"assistantMessageEvent"`
}

// ToolExecutionEvent is tool_execution_start, tool_execution_update or
// tool_execution_end. Result is set on end, PartialResult on update, and
// DurationMs on an end whose tool ran.
type ToolExecutionEvent struct {
	ToolCallID    string          `json:"toolCallId"`
	ToolName      string          `json:"toolName"`
	Args          json.RawMessage `json:"args"`
	PartialResult json.RawMessage `json:"partialResult"`
	Result        json.RawMessage `json:"result"`
	IsError       bool            `json:"isError"`
	DurationMs    *float64        `json:"durationMs"`
}

// BashExecutionUpdateEvent is bash_execution_update: one output chunk of the
// bash command whose request id is ID.
type BashExecutionUpdateEvent struct {
	ID    RequestID `json:"id"`
	Delta string    `json:"delta"`
}

// RequestID is a command id as the child echoes it. The child repeats the id's
// JSON value, so a string id decodes to its text and any other JSON value to
// its source text.
type RequestID string

// UnmarshalJSON accepts a JSON string or any other JSON value.
func (id *RequestID) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		text, err := strconv.Unquote(string(data))
		if err != nil {
			var decoded string
			if err := json.Unmarshal(data, &decoded); err != nil {
				return err
			}
			text = decoded
		}
		*id = RequestID(text)
		return nil
	}
	*id = RequestID(data)
	return nil
}

// QueueUpdateEvent is queue_update: the queued steering and follow-up texts.
type QueueUpdateEvent struct {
	Steering []string `json:"steering"`
	FollowUp []string `json:"followUp"`
}

// SessionInfoChangedEvent is session_info_changed; Name is nil when the name
// was cleared.
type SessionInfoChangedEvent struct {
	Name *string `json:"name"`
}

// ThinkingLevelChangedEvent is thinking_level_changed.
type ThinkingLevelChangedEvent struct {
	Level string `json:"level"`
}

// CompactionEvent is compaction_start or compaction_end. Result is set on a
// successful end, and ErrorMessage on a failed one.
type CompactionEvent struct {
	Reason       string            `json:"reason"`
	Result       *CompactionResult `json:"result"`
	Aborted      bool              `json:"aborted"`
	WillRetry    bool              `json:"willRetry"`
	ErrorMessage string            `json:"errorMessage"`
}

// EntryAppendedEvent is entry_appended: the custom entry an extension
// persisted.
type EntryAppendedEvent struct {
	Entry Entry `json:"entry"`
}

// ErrorEvent is error or extension_error: a failure reported outside a
// response. Fields the child does not set stay empty.
type ErrorEvent struct {
	Message   string `json:"message"`
	Error     string `json:"error"`
	Extension string `json:"extension"`
	Event     string `json:"event"`
}

// AgentSettled decodes an agent_settled record.
func (e Event) AgentSettled() (AgentSettledEvent, error) {
	var out AgentSettledEvent
	return out, e.Decode(&out)
}

// AgentEnd decodes an agent_end record.
func (e Event) AgentEnd() (AgentEndEvent, error) {
	var out AgentEndEvent
	return out, e.Decode(&out)
}

// Message decodes the message of a message_start or message_end record.
func (e Event) Message() (Message, error) {
	var out MessageEvent
	return out.Message, e.Decode(&out)
}

// TurnEnd decodes a turn_end record.
func (e Event) TurnEnd() (TurnEndEvent, error) {
	var out TurnEndEvent
	return out, e.Decode(&out)
}

// MessageUpdate decodes a message_update record.
func (e Event) MessageUpdate() (MessageUpdateEvent, error) {
	var out MessageUpdateEvent
	return out, e.Decode(&out)
}

// ToolExecution decodes a tool_execution_start, tool_execution_update or
// tool_execution_end record.
func (e Event) ToolExecution() (ToolExecutionEvent, error) {
	var out ToolExecutionEvent
	return out, e.Decode(&out)
}

// BashExecutionUpdate decodes a bash_execution_update record.
func (e Event) BashExecutionUpdate() (BashExecutionUpdateEvent, error) {
	var out BashExecutionUpdateEvent
	return out, e.Decode(&out)
}

// QueueUpdate decodes a queue_update record.
func (e Event) QueueUpdate() (QueueUpdateEvent, error) {
	var out QueueUpdateEvent
	return out, e.Decode(&out)
}

// SessionInfoChanged decodes a session_info_changed record.
func (e Event) SessionInfoChanged() (SessionInfoChangedEvent, error) {
	var out SessionInfoChangedEvent
	return out, e.Decode(&out)
}

// ThinkingLevelChanged decodes a thinking_level_changed record.
func (e Event) ThinkingLevelChanged() (ThinkingLevelChangedEvent, error) {
	var out ThinkingLevelChangedEvent
	return out, e.Decode(&out)
}

// Compaction decodes a compaction_start or compaction_end record.
func (e Event) Compaction() (CompactionEvent, error) {
	var out CompactionEvent
	return out, e.Decode(&out)
}

// EntryAppended decodes an entry_appended record.
func (e Event) EntryAppended() (EntryAppendedEvent, error) {
	var out EntryAppendedEvent
	return out, e.Decode(&out)
}

// ErrorEvent decodes an error or extension_error record.
func (e Event) ErrorEvent() (ErrorEvent, error) {
	var out ErrorEvent
	return out, e.Decode(&out)
}

// UIRequest decodes an extension_ui_request record.
func (e Event) UIRequest() (UIRequest, error) {
	var out UIRequest
	if err := e.Decode(&out); err != nil {
		return UIRequest{}, err
	}
	out.Raw = e.Raw
	return out, nil
}

// Response decodes a response record that no command waited for.
func (e Event) Response() (Response, error) {
	var out Response
	return out, e.Decode(&out)
}
