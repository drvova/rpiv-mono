package rpc

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Response is one `{"type":"response"}` record: the child's answer to a
// command. Data is the success payload as written and Error the failure text.
type Response struct {
	ID      string          `json:"id,omitempty"`
	Command string          `json:"command"`
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// CommandError is a response with success false, returned by the typed
// wrappers. Message is the child's error text, free text rather than a code.
type CommandError struct {
	Command string
	Message string
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("rpc: %s: %s", e.Command, e.Message)
}

// Image is an image block a prompt carries; it marshals as pi-ai's
// {type:"image", data, mimeType} with base64 bytes in Data.
type Image struct {
	Data     string
	MimeType string
}

// MarshalJSON writes the wire shape of an image content block.
func (i Image) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type     string `json:"type"`
		Data     string `json:"data"`
		MimeType string `json:"mimeType"`
	}{"image", i.Data, i.MimeType})
}

// PromptOptions are the optional members of the prompt command.
type PromptOptions struct {
	// StreamingBehavior is "steer" or "followUp": how a prompt sent while
	// the agent runs joins that run. The child rejects a second prompt
	// without it.
	StreamingBehavior string
	// Metadata is an arbitrary object the child stores with the prompt.
	Metadata map[string]any
	// Images are attached to the prompt.
	Images []Image
}

// Prompt dispositions.
const (
	DispositionStarted = "started"
	DispositionQueued  = "queued"
	DispositionHandled = "handled"
)

// Streaming behaviors of a prompt sent during a run.
const (
	StreamingSteer    = "steer"
	StreamingFollowUp = "followUp"
)

// Queue modes of set_steering_mode and set_follow_up_mode.
const (
	QueueModeAll        = "all"
	QueueModeOneAtATime = "one-at-a-time"
)

// BashOptions are the options of the bash command.
type BashOptions struct {
	// ExcludeFromContext keeps the command and its output out of the model
	// context, as `!!` does in the terminal.
	ExcludeFromContext bool
	// OnOutput receives each output chunk as the command runs. It is called
	// on the reader goroutine, so it must return promptly.
	OnOutput func(delta string)
}

// BashResult is the final result of a bash command. ExitCode is nil when the
// command was cancelled or killed.
type BashResult struct {
	Output         string `json:"output"`
	ExitCode       *int   `json:"exitCode"`
	Cancelled      bool   `json:"cancelled"`
	Truncated      bool   `json:"truncated"`
	FullOutputPath string `json:"fullOutputPath,omitempty"`
}

// Model is a full model record. The common fields are decoded; Raw keeps the
// whole object for fields the port does not name (cost, capabilities, ...).
type Model struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	API           string          `json:"api"`
	Provider      string          `json:"provider"`
	BaseURL       string          `json:"baseUrl"`
	Reasoning     bool            `json:"reasoning"`
	Input         []string        `json:"input"`
	ContextWindow int             `json:"contextWindow"`
	MaxTokens     int             `json:"maxTokens"`
	Raw           json.RawMessage `json:"-"`
}

// UnmarshalJSON decodes the named fields and retains the raw object.
func (m *Model) UnmarshalJSON(data []byte) error {
	type plain Model
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*m = Model(decoded)
	m.Raw = append(json.RawMessage(nil), data...)
	return nil
}

// Qualified returns "provider/id".
func (m Model) Qualified() string { return m.Provider + "/" + m.ID }

// State is the get_state payload. SessionFile and SessionName are nil when the
// child has no session file or no name.
type State struct {
	SessionID             string          `json:"sessionId"`
	SessionFile           *string         `json:"sessionFile"`
	SessionName           *string         `json:"sessionName"`
	Model                 *Model          `json:"model"`
	ThinkingLevel         string          `json:"thinkingLevel"`
	SteeringMode          string          `json:"steeringMode"`
	FollowUpMode          string          `json:"followUpMode"`
	IsStreaming           bool            `json:"isStreaming"`
	IsCompacting          bool            `json:"isCompacting"`
	AutoCompactionEnabled bool            `json:"autoCompactionEnabled"`
	MessageCount          int             `json:"messageCount"`
	PendingMessageCount   int             `json:"pendingMessageCount"`
	Raw                   json.RawMessage `json:"-"`
}

// UnmarshalJSON decodes the named fields and retains the raw object.
func (s *State) UnmarshalJSON(data []byte) error {
	type plain State
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*s = State(decoded)
	s.Raw = append(json.RawMessage(nil), data...)
	return nil
}

// Usage is a token total: the tokens member of get_session_stats and the sum
// Run accumulates over a run's assistant messages.
type Usage struct {
	Input      int `json:"input"`
	Output     int `json:"output"`
	CacheRead  int `json:"cacheRead"`
	CacheWrite int `json:"cacheWrite"`
	Total      int `json:"total"`
}

// Add accumulates one message's usage. Total grows by the message's
// totalTokens, which providers may compute differently from the sum of the
// splits.
func (u *Usage) Add(m MessageUsage) {
	u.Input += m.Input
	u.Output += m.Output
	u.CacheRead += m.CacheRead
	u.CacheWrite += m.CacheWrite
	u.Total += m.TotalTokens
}

// MessageUsage is the usage object of one assistant message or
// message_update record.
type MessageUsage struct {
	Input       int        `json:"input"`
	Output      int        `json:"output"`
	CacheRead   int        `json:"cacheRead"`
	CacheWrite  int        `json:"cacheWrite"`
	TotalTokens int        `json:"totalTokens"`
	Cost        *UsageCost `json:"cost"`
}

// UsageCost is the cost object of a usage record, in dollars.
type UsageCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
	Total      float64 `json:"total"`
}

// ContextUsage is the contextUsage member of get_session_stats. Tokens and
// Percent are nil when the child cannot compute them.
type ContextUsage struct {
	Tokens        *int     `json:"tokens"`
	ContextWindow int      `json:"contextWindow"`
	Percent       *float64 `json:"percent"`
}

// SessionStats is the get_session_stats payload.
type SessionStats struct {
	SessionFile       *string       `json:"sessionFile"`
	SessionID         string        `json:"sessionId"`
	UserMessages      int           `json:"userMessages"`
	AssistantMessages int           `json:"assistantMessages"`
	ToolCalls         int           `json:"toolCalls"`
	ToolResults       int           `json:"toolResults"`
	TotalMessages     int           `json:"totalMessages"`
	Tokens            Usage         `json:"tokens"`
	Cost              float64       `json:"cost"`
	ContextUsage      *ContextUsage `json:"contextUsage"`
}

// CompactionResult is the compact payload. Usage and Details stay raw.
type CompactionResult struct {
	Summary              string          `json:"summary"`
	FirstKeptEntryID     string          `json:"firstKeptEntryId"`
	TokensBefore         int             `json:"tokensBefore"`
	EstimatedTokensAfter int             `json:"estimatedTokensAfter"`
	Usage                json.RawMessage `json:"usage,omitempty"`
	Details              json.RawMessage `json:"details,omitempty"`
}

// ClearQueueResult holds the text of the removed queued messages.
type ClearQueueResult struct {
	Steering []string `json:"steering"`
	FollowUp []string `json:"followUp"`
}

// ForkResult is the fork payload: the selected user text, and whether an
// extension cancelled the fork.
type ForkResult struct {
	Text      string `json:"text"`
	Cancelled bool   `json:"cancelled"`
}

// ForkMessage is one user message a fork can start before.
type ForkMessage struct {
	EntryID string `json:"entryId"`
	Text    string `json:"text"`
}

// ModelRef identifies a model by provider and id.
type ModelRef struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
}

// CycleModelResult is the cycle_model payload when a model was selected.
type CycleModelResult struct {
	Model         ModelRef `json:"model"`
	ThinkingLevel string   `json:"thinkingLevel"`
	IsScoped      bool     `json:"isScoped"`
}

// CycleThinkingLevelResult is the cycle_thinking_level payload when the level
// changed.
type CycleThinkingLevelResult struct {
	Level string `json:"level"`
}

// Entry is one session entry. The identity fields are decoded; Raw keeps the
// whole entry, as the session format asks (unknown fields survive).
type Entry struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	ParentID  *string         `json:"parentId"`
	Timestamp string          `json:"timestamp"`
	Raw       json.RawMessage `json:"-"`
}

// UnmarshalJSON decodes the identity fields and retains the raw entry.
func (e *Entry) UnmarshalJSON(data []byte) error {
	type plain Entry
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*e = Entry(decoded)
	e.Raw = append(json.RawMessage(nil), data...)
	return nil
}

// EntriesResult is the get_entries payload.
type EntriesResult struct {
	Entries []Entry `json:"entries"`
	LeafID  *string `json:"leafId"`
}

// TreeNode is one node of the get_tree payload.
type TreeNode struct {
	Entry          Entry      `json:"entry"`
	Children       []TreeNode `json:"children"`
	Label          *string    `json:"label,omitempty"`
	LabelTimestamp *string    `json:"labelTimestamp,omitempty"`
}

// TreeResult is the get_tree payload.
type TreeResult struct {
	Tree   []TreeNode `json:"tree"`
	LeafID *string    `json:"leafId"`
}

// ContentBlock is one block of a message's content. The fields of the text,
// thinking, toolCall and image kinds are decoded; Raw keeps the block.
type ContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	MimeType  string          `json:"mimeType,omitempty"`
	Raw       json.RawMessage `json:"-"`
}

// UnmarshalJSON decodes the named fields and retains the raw block.
func (b *ContentBlock) UnmarshalJSON(data []byte) error {
	type plain ContentBlock
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*b = ContentBlock(decoded)
	b.Raw = append(json.RawMessage(nil), data...)
	return nil
}

// Message is one agent message as the child writes it in events and in
// get_messages. Content is nil for messages whose content is not an array (a
// bashExecution message carries output instead). Raw keeps the message.
type Message struct {
	Role         string          `json:"role"`
	Content      []ContentBlock  `json:"-"`
	Usage        *MessageUsage   `json:"usage"`
	StopReason   string          `json:"stopReason"`
	ErrorMessage string          `json:"errorMessage"`
	Provider     string          `json:"provider"`
	Model        string          `json:"model"`
	ToolCallID   string          `json:"toolCallId"`
	ToolName     string          `json:"toolName"`
	IsError      bool            `json:"isError"`
	Raw          json.RawMessage `json:"-"`
}

// UnmarshalJSON decodes the named fields, the content blocks when content is
// an array, and retains the raw message.
func (m *Message) UnmarshalJSON(data []byte) error {
	type plain Message
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var content struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(data, &content); err != nil {
		return err
	}
	*m = Message(decoded)
	m.Raw = append(json.RawMessage(nil), data...)
	trimmed := strings.TrimSpace(string(content.Content))
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal(content.Content, &m.Content); err != nil {
			return err
		}
	}
	return nil
}

// Text concatenates the message's text blocks, the rule of
// get_last_assistant_text (which also trims the result).
func (m Message) Text() string {
	var text strings.Builder
	for _, block := range m.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	return text.String()
}

// ToolCalls returns the message's toolCall blocks.
func (m Message) ToolCalls() []ContentBlock {
	var calls []ContentBlock
	for _, block := range m.Content {
		if block.Type == "toolCall" {
			calls = append(calls, block)
		}
	}
	return calls
}

// SourceInfo locates a slash command's definition.
type SourceInfo struct {
	Path    string `json:"path"`
	Source  string `json:"source"`
	Scope   string `json:"scope"`
	Origin  string `json:"origin"`
	BaseDir string `json:"baseDir,omitempty"`
}

// SlashCommand is one entry of get_commands: an extension command, a prompt
// template or a skill.
type SlashCommand struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Source      string     `json:"source"`
	SourceInfo  SourceInfo `json:"sourceInfo"`
}
