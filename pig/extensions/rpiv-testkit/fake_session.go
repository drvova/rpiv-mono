package testkit

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/MichaelKinsy/PiG/extensions/sdk"
)

// SendMessage records the message in SentMessages.
func (f *Fake) SendMessage(customType, content string, display bool, opts sdk.SendMessageOptions) error {
	msg := SentMessage{CustomType: customType, Content: content, Display: display, Opts: opts}
	f.mu.Lock()
	f.SentMessages = append(f.SentMessages, msg)
	err, fn := f.fail("SendMessage"), f.SendMessageFn
	f.mu.Unlock()
	if err != nil {
		return err
	}
	if fn != nil {
		return fn(msg)
	}
	return nil
}

// SendCustomMessage records the message in SentMessages with its details.
func (f *Fake) SendCustomMessage(msg sdk.CustomMessage, opts sdk.SendMessageOptions) error {
	sent := SentMessage{CustomType: msg.CustomType, Content: msg.Content, Display: msg.Display, Details: msg.Details, Opts: opts}
	f.mu.Lock()
	f.SentMessages = append(f.SentMessages, sent)
	err, fn := f.fail("SendCustomMessage"), f.SendMessageFn
	f.mu.Unlock()
	if err != nil {
		return err
	}
	if fn != nil {
		return fn(sent)
	}
	return nil
}

// SendUserMessage records the message in UserMessages.
func (f *Fake) SendUserMessage(content any, deliverAs string) error {
	f.mu.Lock()
	f.UserMessages = append(f.UserMessages, UserMessage{Content: content, DeliverAs: deliverAs})
	err, fn := f.fail("SendUserMessage"), f.SendUserMessageFn
	f.mu.Unlock()
	if err != nil {
		return err
	}
	if fn != nil {
		return fn(content, deliverAs)
	}
	return nil
}

// AppendEntry records the entry in AppendedEntries. It does not extend
// Entries: replay tests build the log they want with BuildSessionEntries.
func (f *Fake) AppendEntry(customType string, data any) error {
	f.mu.Lock()
	f.AppendedEntries = append(f.AppendedEntries, AppendedEntry{CustomType: customType, Data: data})
	err, fn := f.fail("AppendEntry"), f.AppendEntryFn
	f.mu.Unlock()
	if err != nil {
		return err
	}
	if fn != nil {
		return fn(customType, data)
	}
	return nil
}

// GetEntries is a copy of Entries; nil when there are none.
func (f *Fake) GetEntries() ([]json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("GetEntries"); err != nil {
		return nil, err
	}
	if len(f.Entries) == 0 {
		return nil, nil
	}
	return slices.Clone(f.Entries), nil
}

// GetBranch is a copy of Branch when it is set, else the root-to-leaf walk of
// Entries from LeafID (or the last entry) decoded as sdk.BranchEntry; nil
// without a session.
func (f *Fake) GetBranch() ([]sdk.BranchEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("GetBranch"); err != nil {
		return nil, err
	}
	if f.Branch != nil {
		return slices.Clone(f.Branch), nil
	}
	raw := branchFrom(f.Entries, f.LeafID)
	if len(raw) == 0 {
		return nil, nil
	}
	branch := make([]sdk.BranchEntry, 0, len(raw))
	for _, entry := range raw {
		var decoded sdk.BranchEntry
		if json.Unmarshal(entry, &decoded) == nil {
			branch = append(branch, decoded)
		}
	}
	return branch, nil
}

// GetSessionID is SessionID.
func (f *Fake) GetSessionID() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("GetSessionID"); err != nil {
		return "", err
	}
	return f.SessionID, nil
}

// GetSessionFile is SessionFile; empty is nil.
func (f *Fake) GetSessionFile() (*string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("GetSessionFile"); err != nil {
		return nil, err
	}
	return optionalString(f.SessionFile), nil
}

// GetSessionName is SessionName; empty is nil.
func (f *Fake) GetSessionName() (*string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("GetSessionName"); err != nil {
		return nil, err
	}
	return optionalString(f.SessionName), nil
}

// SetSessionName sets SessionName and records the name.
func (f *Fake) SetSessionName(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SessionNamesSet = append(f.SessionNamesSet, name)
	if err := f.fail("SetSessionName"); err != nil {
		return err
	}
	f.SessionName = name
	return nil
}

// GetLeafID is LeafID, else the last entry's id, else nil.
func (f *Fake) GetLeafID() (*string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("GetLeafID"); err != nil {
		return nil, err
	}
	if f.LeafID != "" {
		return optionalString(f.LeafID), nil
	}
	return optionalString(lastEntryID(f.Entries)), nil
}

// IsIdle is Idle.
func (f *Fake) IsIdle() (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("IsIdle"); err != nil {
		return false, err
	}
	return f.Idle, nil
}

// WaitForIdle increments WaitedForIdle and answers with WaitForIdleFn.
func (f *Fake) WaitForIdle() error {
	f.mu.Lock()
	f.WaitedForIdle++
	err, fn := f.fail("WaitForIdle"), f.WaitForIdleFn
	f.mu.Unlock()
	if err != nil {
		return err
	}
	if fn != nil {
		return fn()
	}
	return nil
}

// HasPendingMessages is Pending.
func (f *Fake) HasPendingMessages() (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("HasPendingMessages"); err != nil {
		return false, err
	}
	return f.Pending, nil
}

// IsProjectTrusted is Trusted.
func (f *Fake) IsProjectTrusted() (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("IsProjectTrusted"); err != nil {
		return false, err
	}
	return f.Trusted, nil
}

// Abort sets Aborted and counts the call.
func (f *Fake) Abort() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Aborted = true
	f.AbortCount++
}

// Reload increments Reloaded and answers with ReloadFn.
func (f *Fake) Reload() error {
	f.mu.Lock()
	f.Reloaded++
	err, fn := f.fail("Reload"), f.ReloadFn
	f.mu.Unlock()
	if err != nil {
		return err
	}
	if fn != nil {
		return fn()
	}
	return nil
}

// Compact records the options (a copy; nil stays nil).
func (f *Fake) Compact(opts map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Compactions = append(f.Compactions, maps.Clone(opts))
}

// BuildSessionContext is SessionCtx when set, else the messages of the
// "message" entries on the branch with ThinkingLevel and a model map of
// ModelID and Provider.
func (f *Fake) BuildSessionContext() (sdk.SessionContext, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("BuildSessionContext"); err != nil {
		return sdk.SessionContext{}, err
	}
	if f.SessionCtx != nil {
		return *f.SessionCtx, nil
	}
	return sdk.SessionContext{
		Messages:      messagesOf(branchFrom(f.Entries, f.LeafID)),
		ThinkingLevel: f.ThinkingLevel,
		Model:         map[string]any{"id": f.ModelID, "provider": f.Provider},
	}, nil
}

// BuildContextEntries is Entries decoded as objects.
func (f *Fake) BuildContextEntries() ([]map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("BuildContextEntries"); err != nil {
		return nil, err
	}
	return decodeEntryMaps(f.Entries), nil
}

// SessionGetBranch is the root-to-fromID walk of Entries decoded as objects;
// a nil fromID starts at LeafID or the last entry.
func (f *Fake) SessionGetBranch(fromID *string) ([]map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("SessionGetBranch"); err != nil {
		return nil, err
	}
	from := f.LeafID
	if fromID != nil {
		from = *fromID
	}
	return decodeEntryMaps(branchFrom(f.Entries, from)), nil
}

// optionalString is Pi's `string | undefined`: nil for an empty string.
func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
