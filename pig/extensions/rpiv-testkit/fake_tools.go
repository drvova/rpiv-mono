package testkit

import (
	"fmt"
	"maps"
	"slices"

	"github.com/MichaelKinsy/PiG/extensions/sdk"
)

// GetActiveTools is a copy of ActiveTools; never nil.
func (f *Fake) GetActiveTools() ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("GetActiveTools"); err != nil {
		return nil, err
	}
	return append([]string{}, f.ActiveTools...), nil
}

// SetActiveTools records the list in ActiveToolCalls and replaces
// ActiveTools with a copy.
func (f *Fake) SetActiveTools(tools []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ActiveToolCalls = append(f.ActiveToolCalls, slices.Clone(tools))
	f.ActiveTools = append([]string{}, tools...)
}

// GetAllTools is a copy of AllTools; never nil.
func (f *Fake) GetAllTools() ([]sdk.ToolInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("GetAllTools"); err != nil {
		return nil, err
	}
	return append([]sdk.ToolInfo{}, f.AllTools...), nil
}

// GetCommands is Commands followed by one "skill:<name>" command with source
// "skill" per entry of Skills; never nil.
func (f *Fake) GetCommands() ([]sdk.CommandInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("GetCommands"); err != nil {
		return nil, err
	}
	commands := append([]sdk.CommandInfo{}, f.Commands...)
	for _, name := range f.Skills {
		commands = append(commands, SkillCommand(name))
	}
	return commands, nil
}

// SkillCommand is the sdk.CommandInfo Pi lists for a skill: "skill:<name>"
// with source "skill" and a SKILL.md path under /mock/skills.
func SkillCommand(name string) sdk.CommandInfo {
	return sdk.CommandInfo{
		Name:   "skill:" + name,
		Source: "skill",
		SourceInfo: sdk.SourceInfo{
			Path:    "/mock/skills/" + name + "/SKILL.md",
			Source:  "skill",
			BaseDir: "/mock/skills/" + name,
		},
	}
}

// RegisterTool records the definition in RegisteredTools, adds its name to
// ActiveTools when absent, and calls RegisterToolFn.
func (f *Fake) RegisterTool(definition sdk.ToolDefinition) {
	f.mu.Lock()
	f.RegisteredTools = append(f.RegisteredTools, definition)
	if !slices.Contains(f.ActiveTools, definition.Name) {
		f.ActiveTools = append(f.ActiveTools, definition.Name)
	}
	fn := f.RegisterToolFn
	f.mu.Unlock()
	if fn != nil {
		fn(definition)
	}
}

// ExecuteTool records the call in ExecutedTools and answers with
// ExecuteToolFn, or with an outcome whose ToolCall names the tool (id
// "<CallID>/<n>") and whose result is empty.
func (f *Fake) ExecuteTool(name string, args any, options *sdk.ExecuteToolOptions) (sdk.AgentToolCallOutcome, error) {
	f.mu.Lock()
	f.ExecutedTools = append(f.ExecutedTools, ExecuteToolCall{Name: name, Args: args, Options: options})
	n := len(f.ExecutedTools)
	err, fn, callID := f.fail("ExecuteTool"), f.ExecuteToolFn, f.CallID
	f.mu.Unlock()
	if err != nil {
		return sdk.AgentToolCallOutcome{}, err
	}
	if fn != nil {
		return fn(name, args, options)
	}
	arguments, _ := args.(map[string]any)
	return sdk.AgentToolCallOutcome{
		ToolCall: sdk.ToolCall{Type: "toolCall", ID: fmt.Sprintf("%s/%d", callID, n), Name: name, Arguments: arguments},
		Result:   sdk.AgentToolResult{Content: []sdk.ToolResultContent{}},
	}, nil
}

// OnUpdate records the partial result in Updates.
func (f *Fake) OnUpdate(partial any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Updates = append(f.Updates, partial)
	return f.fail("OnUpdate")
}

// GetFlag is Flags[name]; an absent name is nil.
func (f *Fake) GetFlag(name string) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("GetFlag"); err != nil {
		return nil, err
	}
	return f.Flags[name], nil
}

// GetSettings is a copy of Settings; nil is an empty object.
func (f *Fake) GetSettings() (sdk.Settings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("GetSettings"); err != nil {
		return nil, err
	}
	if f.Settings == nil {
		return sdk.Settings{}, nil
	}
	return maps.Clone(f.Settings), nil
}

// GetSystemPrompt is SystemPrompt.
func (f *Fake) GetSystemPrompt() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("GetSystemPrompt"); err != nil {
		return "", err
	}
	return f.SystemPrompt, nil
}

// GetSystemPromptOptions is PromptOptions.
func (f *Fake) GetSystemPromptOptions() (sdk.SystemPromptOptions, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("GetSystemPromptOptions"); err != nil {
		return sdk.SystemPromptOptions{}, err
	}
	return f.PromptOptions, nil
}
