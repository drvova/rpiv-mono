package testkit

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/MichaelKinsy/PiG/extensions/sdk"
	host "github.com/juicesharp/rpiv-mono/pig/extensions/rpiv-host"
)

// defaultModelAuth is the GetModelAuth and APIKeyAndHeaders answer the
// TypeScript mock registry gave: a configured key with no extra headers.
const defaultModelAuth = `{"ok":true,"apiKey":"test-key","headers":{}}`

// Model is ModelID.
func (f *Fake) Model() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ModelID
}

// ModelProvider is Provider.
func (f *Fake) ModelProvider() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Provider
}

// ModelQualified is "<Provider>/<ModelID>", or ModelID alone without a
// provider.
func (f *Fake) ModelQualified() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Provider == "" {
		return f.ModelID
	}
	return f.Provider + "/" + f.ModelID
}

// GetModelInfo is a copy of ModelInfo; nil is no model.
func (f *Fake) GetModelInfo() (*sdk.ModelInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("GetModelInfo"); err != nil {
		return nil, err
	}
	if f.ModelInfo == nil {
		return nil, nil
	}
	info := *f.ModelInfo
	return &info, nil
}

// SetModel records the model in ModelsSet and answers with SetModelFn, or
// splits "provider/model" into Provider and ModelID and succeeds.
func (f *Fake) SetModel(model string) (bool, error) {
	f.mu.Lock()
	f.ModelsSet = append(f.ModelsSet, model)
	err, fn := f.fail("SetModel"), f.SetModelFn
	if err == nil && fn == nil {
		if provider, id, ok := strings.Cut(model, "/"); ok {
			f.Provider, f.ModelID = provider, id
		} else {
			f.ModelID = model
		}
	}
	f.mu.Unlock()
	if err != nil {
		return false, err
	}
	if fn != nil {
		return fn(model)
	}
	return true, nil
}

// GetThinkingLevel is ThinkingLevel.
func (f *Fake) GetThinkingLevel() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("GetThinkingLevel"); err != nil {
		return "", err
	}
	return f.ThinkingLevel, nil
}

// SetThinkingLevel sets ThinkingLevel and records the level.
func (f *Fake) SetThinkingLevel(level string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ThinkingLevelsSet = append(f.ThinkingLevelsSet, level)
	f.ThinkingLevel = level
}

// GetContextUsage is a copy of ContextUsage; nil is unknown.
func (f *Fake) GetContextUsage() (*sdk.ContextUsage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("GetContextUsage"); err != nil {
		return nil, err
	}
	if f.ContextUsage == nil {
		return nil, nil
	}
	usage := *f.ContextUsage
	return &usage, nil
}

// GetModelAuth records the call in ModelAuthCalls and answers with
// ModelAuth, or a configured test key.
func (f *Fake) GetModelAuth(providerID, modelID string) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ModelAuthCalls = append(f.ModelAuthCalls, ModelAuthCall{Provider: providerID, ModelID: modelID})
	if err := f.fail("GetModelAuth"); err != nil {
		return nil, err
	}
	if f.ModelAuth == nil {
		return json.RawMessage(defaultModelAuth), nil
	}
	return slices.Clone(f.ModelAuth), nil
}

// Complete records the call in Completions and answers with CompleteFn, or
// an empty JSON object.
func (f *Fake) Complete(model, request, auth map[string]any) (json.RawMessage, error) {
	f.mu.Lock()
	f.Completions = append(f.Completions, CompleteCall{Model: model, Request: request, Auth: auth})
	err, fn := f.fail("Complete"), f.CompleteFn
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if fn != nil {
		return fn(model, request, auth)
	}
	return json.RawMessage("{}"), nil
}

// Exec records the call in Execs and answers with ExecFn, or a zero result.
func (f *Fake) Exec(command string, args []string) (*sdk.ExecResult, error) {
	return f.exec(ExecCall{Command: command, Args: slices.Clone(args)})
}

// ExecWithOptions records the call in Execs and answers with ExecFn, or a
// zero result.
func (f *Fake) ExecWithOptions(command string, args []string, opts sdk.ExecOptions) (*sdk.ExecResult, error) {
	return f.exec(ExecCall{Command: command, Args: slices.Clone(args), Options: opts, WithOptions: true})
}

func (f *Fake) exec(call ExecCall) (*sdk.ExecResult, error) {
	f.mu.Lock()
	f.Execs = append(f.Execs, call)
	method := "Exec"
	if call.WithOptions {
		method = "ExecWithOptions"
	}
	err, fn := f.fail(method), f.ExecFn
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if fn != nil {
		return fn(call.Command, call.Args, call.Options)
	}
	return &sdk.ExecResult{}, nil
}

// AllModels is a copy of Models; never nil.
func (f *Fake) AllModels() ([]map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("AllModels"); err != nil {
		return nil, err
	}
	return append([]map[string]any{}, f.Models...), nil
}

// AvailableModels is a copy of Models, as the TypeScript mock registry's
// getAvailable listed every model; never nil.
func (f *Fake) AvailableModels() ([]map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("AvailableModels"); err != nil {
		return nil, err
	}
	return append([]map[string]any{}, f.Models...), nil
}

// FindModel is the entry of Models whose "provider" and "id" (or "modelId")
// match; nil when there is none.
func (f *Fake) FindModel(providerID, modelID string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, model := range f.Models {
		provider, _ := model["provider"].(string)
		id, _ := model["id"].(string)
		if id == "" {
			id, _ = model["modelId"].(string)
		}
		if provider == providerID && id == modelID {
			return model
		}
	}
	return nil
}

// ProviderAuthStatus is ProviderAuth[provider]; an absent provider is not
// configured.
func (f *Fake) ProviderAuthStatus(provider string) (sdk.ProviderAuthStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("ProviderAuthStatus"); err != nil {
		return sdk.ProviderAuthStatus{}, err
	}
	return f.ProviderAuth[provider], nil
}

// ProviderDisplayName is ProviderNames[provider], or the provider id.
func (f *Fake) ProviderDisplayName(provider string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("ProviderDisplayName"); err != nil {
		return "", err
	}
	if name, ok := f.ProviderNames[provider]; ok {
		return name, nil
	}
	return provider, nil
}

// APIKeyForProvider is APIKeys[provider]; an absent provider is nil.
func (f *Fake) APIKeyForProvider(provider string) *string {
	f.mu.Lock()
	defer f.mu.Unlock()
	key, ok := f.APIKeys[provider]
	if !ok {
		return nil
	}
	return &key
}

// APIKeyAndHeaders answers with APIKeyAndHeadersFn, or a configured test
// key: {"ok":true,"apiKey":"test-key","headers":{}}.
func (f *Fake) APIKeyAndHeaders(model map[string]any) (map[string]any, error) {
	f.mu.Lock()
	err, fn := f.fail("APIKeyAndHeaders"), f.APIKeyAndHeadersFn
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if fn != nil {
		return fn(model)
	}
	var auth map[string]any
	_ = json.Unmarshal([]byte(defaultModelAuth), &auth)
	return auth, nil
}

// StreamSimple records the call in Streams and answers with StreamFn, or an
// empty ScriptedStream.
func (f *Fake) StreamSimple(model, request, options map[string]any) host.ModelStream {
	return f.stream(model, request, options, true)
}

// Stream records the call in Streams and answers with StreamFn, or an empty
// ScriptedStream.
func (f *Fake) Stream(model, request, options map[string]any) host.ModelStream {
	return f.stream(model, request, options, false)
}

func (f *Fake) stream(model, request, options map[string]any, simple bool) host.ModelStream {
	f.mu.Lock()
	f.Streams = append(f.Streams, StreamCall{Model: model, Request: request, Options: options, Simple: simple})
	fn := f.StreamFn
	f.mu.Unlock()
	if fn != nil {
		return fn(model, request, options, simple)
	}
	return NewScriptedStream(nil, nil)
}

// ScriptedStream is a host.ModelStream that yields scripted events and ends
// with a scripted result.
type ScriptedStream struct {
	events []map[string]any
	result map[string]any
}

var _ host.ModelStream = (*ScriptedStream)(nil)

// NewScriptedStream returns a stream of events (copied) that ends with
// result, the final assistant message.
func NewScriptedStream(events []map[string]any, result map[string]any) *ScriptedStream {
	copied := make([]map[string]any, 0, len(events))
	for _, event := range events {
		copied = append(copied, maps.Clone(event))
	}
	return &ScriptedStream{events: copied, result: result}
}

// Events yields the scripted events in order until they end or ctx is done,
// then closes the channel.
func (s *ScriptedStream) Events(ctx context.Context) <-chan map[string]any {
	out := make(chan map[string]any)
	go func() {
		defer close(out)
		for _, event := range s.events {
			select {
			case out <- maps.Clone(event):
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// Result is the scripted final message.
func (s *ScriptedStream) Result() map[string]any { return s.result }
