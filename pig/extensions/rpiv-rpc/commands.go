package rpc

import (
	"context"
	"encoding/json"
	"fmt"
)

// call sends a command and decodes a successful response's data into T. A
// failed response becomes a *CommandError; an absent or null payload leaves
// the zero value.
func call[T any](c *Client, ctx context.Context, cmd map[string]any) (T, error) {
	var out T
	response, err := c.Call(ctx, cmd)
	if err != nil {
		return out, err
	}
	if !response.Success {
		return out, &CommandError{Command: response.Command, Message: response.Error}
	}
	if len(response.Data) == 0 || string(response.Data) == "null" {
		return out, nil
	}
	if err := json.Unmarshal(response.Data, &out); err != nil {
		return out, fmt.Errorf("rpc: decode %s response: %w", response.Command, err)
	}
	return out, nil
}

// callOnly sends a command whose success payload carries nothing.
func callOnly(c *Client, ctx context.Context, cmd map[string]any) error {
	_, err := call[json.RawMessage](c, ctx, cmd)
	return err
}

type dispositionData struct {
	Disposition string `json:"disposition"`
}

// Prompt sends a user prompt and returns its disposition once the child
// accepted it: DispositionStarted when a run began, DispositionQueued when it
// waits behind the active run, DispositionHandled when an extension command
// or input handler consumed it and no run follows. The answer arrives as
// events; see Run and WaitSettled.
func (c *Client) Prompt(ctx context.Context, message string, opts PromptOptions) (string, error) {
	cmd := map[string]any{"type": "prompt", "message": message}
	if opts.StreamingBehavior != "" {
		cmd["streamingBehavior"] = opts.StreamingBehavior
	}
	if opts.Metadata != nil {
		cmd["metadata"] = opts.Metadata
	}
	if opts.Images != nil {
		cmd["images"] = opts.Images
	}
	data, err := call[dispositionData](c, ctx, cmd)
	return data.Disposition, err
}

// Steer queues a message that interrupts the active run after its current
// tool batch. The disposition is queued or handled.
func (c *Client) Steer(ctx context.Context, message string, images []Image) (string, error) {
	cmd := map[string]any{"type": "steer", "message": message}
	if images != nil {
		cmd["images"] = images
	}
	data, err := call[dispositionData](c, ctx, cmd)
	return data.Disposition, err
}

// FollowUp queues a message for after the active run settles. The
// disposition is queued or handled.
func (c *Client) FollowUp(ctx context.Context, message string, images []Image) (string, error) {
	cmd := map[string]any{"type": "follow_up", "message": message}
	if images != nil {
		cmd["images"] = images
	}
	data, err := call[dispositionData](c, ctx, cmd)
	return data.Disposition, err
}

// Abort cancels the current run and returns once the child acknowledged it;
// the run's agent_settled event reports aborted true.
func (c *Client) Abort(ctx context.Context) error {
	return callOnly(c, ctx, map[string]any{"type": "abort"})
}

// ClearQueue removes every queued steering and follow-up message and returns
// their text.
func (c *Client) ClearQueue(ctx context.Context) (ClearQueueResult, error) {
	return call[ClearQueueResult](c, ctx, map[string]any{"type": "clear_queue"})
}

type cancelledData struct {
	Cancelled bool `json:"cancelled"`
}

// NewSession replaces the session with a fresh one, recording parentSession
// when not empty. It returns whether an extension cancelled the replacement.
func (c *Client) NewSession(ctx context.Context, parentSession string) (cancelled bool, err error) {
	cmd := map[string]any{"type": "new_session"}
	if parentSession != "" {
		cmd["parentSession"] = parentSession
	}
	data, err := call[cancelledData](c, ctx, cmd)
	return data.Cancelled, err
}

// SwitchSession replaces the session with the one in sessionPath. It returns
// whether an extension cancelled the switch.
func (c *Client) SwitchSession(ctx context.Context, sessionPath string) (cancelled bool, err error) {
	data, err := call[cancelledData](c, ctx, map[string]any{"type": "switch_session", "sessionPath": sessionPath})
	return data.Cancelled, err
}

// Fork starts a new session branched before the user message entryID and
// returns that message's text.
func (c *Client) Fork(ctx context.Context, entryID string) (ForkResult, error) {
	return call[ForkResult](c, ctx, map[string]any{"type": "fork", "entryId": entryID})
}

// Clone forks at the active leaf and switches to the clone. It returns
// whether an extension cancelled the clone.
func (c *Client) Clone(ctx context.Context) (cancelled bool, err error) {
	data, err := call[cancelledData](c, ctx, map[string]any{"type": "clone"})
	return data.Cancelled, err
}

// GetForkMessages lists the user messages a fork can start before.
func (c *Client) GetForkMessages(ctx context.Context) ([]ForkMessage, error) {
	data, err := call[struct {
		Messages []ForkMessage `json:"messages"`
	}](c, ctx, map[string]any{"type": "get_fork_messages"})
	return data.Messages, err
}

// GetState reads the session, queue and model state.
func (c *Client) GetState(ctx context.Context) (State, error) {
	return call[State](c, ctx, map[string]any{"type": "get_state"})
}

// SetModel selects a model and returns its full record.
func (c *Client) SetModel(ctx context.Context, provider, modelID string) (Model, error) {
	return call[Model](c, ctx, map[string]any{"type": "set_model", "provider": provider, "modelId": modelID})
}

// CycleModel selects the next available or scoped model; nil means there was
// nothing to cycle to.
func (c *Client) CycleModel(ctx context.Context) (*CycleModelResult, error) {
	return call[*CycleModelResult](c, ctx, map[string]any{"type": "cycle_model"})
}

// GetAvailableModels lists the full records of the available models.
func (c *Client) GetAvailableModels(ctx context.Context) ([]Model, error) {
	data, err := call[struct {
		Models []Model `json:"models"`
	}](c, ctx, map[string]any{"type": "get_available_models"})
	return data.Models, err
}

// SetThinkingLevel sets the thinking level (off, minimal, low, medium, high,
// xhigh or max); the model's capabilities may clamp it.
func (c *Client) SetThinkingLevel(ctx context.Context, level string) error {
	return callOnly(c, ctx, map[string]any{"type": "set_thinking_level", "level": level})
}

// CycleThinkingLevel selects the next level the model supports; nil means
// the model has only one.
func (c *Client) CycleThinkingLevel(ctx context.Context) (*CycleThinkingLevelResult, error) {
	return call[*CycleThinkingLevelResult](c, ctx, map[string]any{"type": "cycle_thinking_level"})
}

// GetAvailableThinkingLevels lists the levels the current model supports.
func (c *Client) GetAvailableThinkingLevels(ctx context.Context) ([]string, error) {
	data, err := call[struct {
		Levels []string `json:"levels"`
	}](c, ctx, map[string]any{"type": "get_available_thinking_levels"})
	return data.Levels, err
}

// SetSteeringMode sets and persists how steering messages are delivered:
// QueueModeAll or QueueModeOneAtATime.
func (c *Client) SetSteeringMode(ctx context.Context, mode string) error {
	return callOnly(c, ctx, map[string]any{"type": "set_steering_mode", "mode": mode})
}

// SetFollowUpMode sets and persists how follow-up messages are delivered:
// QueueModeAll or QueueModeOneAtATime.
func (c *Client) SetFollowUpMode(ctx context.Context, mode string) error {
	return callOnly(c, ctx, map[string]any{"type": "set_follow_up_mode", "mode": mode})
}

// Compact runs manual compaction and returns its result once it finished.
// customInstructions, when not empty, steer the summary.
func (c *Client) Compact(ctx context.Context, customInstructions string) (CompactionResult, error) {
	cmd := map[string]any{"type": "compact"}
	if customInstructions != "" {
		cmd["customInstructions"] = customInstructions
	}
	return call[CompactionResult](c, ctx, cmd)
}

// SetAutoCompaction persists whether the child compacts automatically.
func (c *Client) SetAutoCompaction(ctx context.Context, enabled bool) error {
	return callOnly(c, ctx, map[string]any{"type": "set_auto_compaction", "enabled": enabled})
}

// SetAutoRetry sets whether failed provider requests are retried in this
// session.
func (c *Client) SetAutoRetry(ctx context.Context, enabled bool) error {
	return callOnly(c, ctx, map[string]any{"type": "set_auto_retry", "enabled": enabled})
}

// AbortRetry cancels the active retry delay.
func (c *Client) AbortRetry(ctx context.Context) error {
	return callOnly(c, ctx, map[string]any{"type": "abort_retry"})
}

// Bash runs a shell command in the child and returns its final result.
// Output chunks stream to opts.OnOutput as bash_execution_update events
// carrying this command's id; the final output is also in the result.
func (c *Client) Bash(ctx context.Context, command string, opts BashOptions) (BashResult, error) {
	id := c.newRequestID()
	if opts.OnOutput != nil {
		unsubscribe := c.Subscribe(func(event Event) {
			if event.Type != EventBashExecutionUpdate {
				return
			}
			update, err := event.BashExecutionUpdate()
			if err == nil && string(update.ID) == id {
				opts.OnOutput(update.Delta)
			}
		})
		defer unsubscribe()
	}
	cmd := map[string]any{"type": "bash", "id": id, "command": command}
	if opts.ExcludeFromContext {
		cmd["excludeFromContext"] = true
	}
	return call[BashResult](c, ctx, cmd)
}

// AbortBash cancels the active bash commands.
func (c *Client) AbortBash(ctx context.Context) error {
	return callOnly(c, ctx, map[string]any{"type": "abort_bash"})
}

// GetSessionStats reads the session's message, token and cost totals.
func (c *Client) GetSessionStats(ctx context.Context) (SessionStats, error) {
	return call[SessionStats](c, ctx, map[string]any{"type": "get_session_stats"})
}

// ExportHTML exports the session to HTML at outputPath, or at the child's
// default path when empty, and returns the written path.
func (c *Client) ExportHTML(ctx context.Context, outputPath string) (string, error) {
	cmd := map[string]any{"type": "export_html"}
	if outputPath != "" {
		cmd["outputPath"] = outputPath
	}
	data, err := call[struct {
		Path string `json:"path"`
	}](c, ctx, cmd)
	return data.Path, err
}

// GetEntries reads the session entries in append order, all of them when
// since is empty and those after the entry since otherwise. An unknown since
// is a *CommandError.
func (c *Client) GetEntries(ctx context.Context, since string) (EntriesResult, error) {
	cmd := map[string]any{"type": "get_entries"}
	if since != "" {
		cmd["since"] = since
	}
	return call[EntriesResult](c, ctx, cmd)
}

// GetTree reads the session entry tree and the active leaf.
func (c *Client) GetTree(ctx context.Context) (TreeResult, error) {
	return call[TreeResult](c, ctx, map[string]any{"type": "get_tree"})
}

// GetLastAssistantText reads the text of the last assistant message. ok is
// false when the session has none.
func (c *Client) GetLastAssistantText(ctx context.Context) (text string, ok bool, err error) {
	data, err := call[struct {
		Text *string `json:"text"`
	}](c, ctx, map[string]any{"type": "get_last_assistant_text"})
	if err != nil || data.Text == nil {
		return "", false, err
	}
	return *data.Text, true, nil
}

// SetSessionName sets the session's display name; the child writes
// session_info_changed before it answers.
func (c *Client) SetSessionName(ctx context.Context, name string) error {
	return callOnly(c, ctx, map[string]any{"type": "set_session_name", "name": name})
}

// GetMessages reads the agent messages of the active branch.
func (c *Client) GetMessages(ctx context.Context) ([]Message, error) {
	data, err := call[struct {
		Messages []Message `json:"messages"`
	}](c, ctx, map[string]any{"type": "get_messages"})
	return data.Messages, err
}

// GetCommands lists the extension commands, prompt templates and skills a
// prompt can invoke as /name.
func (c *Client) GetCommands(ctx context.Context) ([]SlashCommand, error) {
	data, err := call[struct {
		Commands []SlashCommand `json:"commands"`
	}](c, ctx, map[string]any{"type": "get_commands"})
	return data.Commands, err
}

// GetExtensions lists the resolved paths of the loaded extensions in load
// order; PiG's built-in ones appear as builtin:<name>.
func (c *Client) GetExtensions(ctx context.Context) ([]string, error) {
	data, err := call[struct {
		Paths []string `json:"paths"`
	}](c, ctx, map[string]any{"type": "get_extensions"})
	return data.Paths, err
}
