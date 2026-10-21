package testkit

import (
	"errors"
	"slices"
	"sync"

	"github.com/MichaelKinsy/PiG/extensions/sdk"
)

// Notify records a notification.
func (f *Fake) Notify(message, level string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Notifications = append(f.Notifications, Notification{Message: message, Level: level})
}

// SetStatus records the update and sets Statuses[key]; an empty text clears
// the key, as Pi's setStatus(key, undefined) does.
func (f *Fake) SetStatus(key, text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.StatusLog = append(f.StatusLog, StatusUpdate{Key: key, Text: text})
	if text == "" {
		delete(f.Statuses, key)
		return
	}
	f.Statuses[key] = text
}

// SetTitle records the title.
func (f *Fake) SetTitle(title string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Titles = append(f.Titles, title)
}

// SetWorkingMessage records the message.
func (f *Fake) SetWorkingMessage(message string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.WorkingMessages = append(f.WorkingMessages, message)
}

// ClearWorkingMessage records an empty working message.
func (f *Fake) ClearWorkingMessage() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.WorkingMessages = append(f.WorkingMessages, "")
}

// Select records the dialog and answers with SelectFn, or cancels.
func (f *Fake) Select(title string, options []string) (string, bool, error) {
	f.mu.Lock()
	f.Dialogs = append(f.Dialogs, Dialog{Kind: "select", Title: title, Options: slices.Clone(options)})
	err, fn := f.fail("Select"), f.SelectFn
	f.mu.Unlock()
	if err != nil {
		return "", false, err
	}
	if fn == nil {
		return "", false, nil
	}
	return fn(title, options)
}

// Confirm records the dialog and answers with ConfirmFn, or declines.
func (f *Fake) Confirm(title, message string) (bool, error) {
	f.mu.Lock()
	f.Dialogs = append(f.Dialogs, Dialog{Kind: "confirm", Title: title, Text: message})
	err, fn := f.fail("Confirm"), f.ConfirmFn
	f.mu.Unlock()
	if err != nil {
		return false, err
	}
	if fn == nil {
		return false, nil
	}
	return fn(title, message)
}

// Input records the dialog and answers with InputFn, or cancels.
func (f *Fake) Input(title, placeholder string) (string, bool, error) {
	f.mu.Lock()
	f.Dialogs = append(f.Dialogs, Dialog{Kind: "input", Title: title, Text: placeholder})
	err, fn := f.fail("Input"), f.InputFn
	f.mu.Unlock()
	if err != nil {
		return "", false, err
	}
	if fn == nil {
		return "", false, nil
	}
	return fn(title, placeholder)
}

// Editor records the dialog and answers with EditorFn, or cancels.
func (f *Fake) Editor(title, prefill string) (string, bool, error) {
	f.mu.Lock()
	f.Dialogs = append(f.Dialogs, Dialog{Kind: "editor", Title: title, Text: prefill})
	err, fn := f.fail("Editor"), f.EditorFn
	f.mu.Unlock()
	if err != nil {
		return "", false, err
	}
	if fn == nil {
		return "", false, nil
	}
	return fn(title, prefill)
}

// Custom records the component and options and answers with CustomFn, or
// closes the component with (nil, nil). Without a UI it returns (nil, nil)
// without recording, as the SDK does not run the component.
func (f *Fake) Custom(factory any, options any) (any, error) {
	f.mu.Lock()
	if !f.UI {
		f.mu.Unlock()
		return nil, nil
	}
	f.Customs = append(f.Customs, factory)
	f.CustomOptions = append(f.CustomOptions, options)
	err, fn := f.fail("Custom"), f.CustomFn
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if fn == nil {
		return nil, nil
	}
	return fn(factory, options)
}

// GetEditorText is EditorText.
func (f *Fake) GetEditorText() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("GetEditorText"); err != nil {
		return "", err
	}
	return f.EditorText, nil
}

// SetEditorText sets EditorText and records the text.
func (f *Fake) SetEditorText(text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.EditorText = text
	f.EditorTextsSet = append(f.EditorTextsSet, text)
}

// PasteToEditor records the text.
func (f *Fake) PasteToEditor(text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Pasted = append(f.Pasted, text)
}

// SetEditorComponent records the factory (nil restores the host's editor).
func (f *Fake) SetEditorComponent(factory sdk.EditorFactory) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.EditorComponents = append(f.EditorComponents, factory)
	return f.fail("SetEditorComponent")
}

// GetToolsExpanded is ToolsExpanded.
func (f *Fake) GetToolsExpanded() (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("GetToolsExpanded"); err != nil {
		return false, err
	}
	return f.ToolsExpanded, nil
}

// SetWidget records the call; a nil content removes the key from Widgets,
// any other content sets it.
func (f *Fake) SetWidget(key string, content any, options ...sdk.WidgetOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := WidgetCall{Key: key, Content: content, Options: slices.Clone(options)}
	f.WidgetLog = append(f.WidgetLog, call)
	if err := f.fail("SetWidget"); err != nil {
		return err
	}
	if content == nil {
		delete(f.Widgets, key)
		return nil
	}
	f.Widgets[key] = call
	return nil
}

// SetFooter records the lines (nil clears) and sets Footer.
func (f *Fake) SetFooter(lines []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Footers = append(f.Footers, slices.Clone(lines))
	if err := f.fail("SetFooter"); err != nil {
		return err
	}
	f.Footer = slices.Clone(lines)
	return nil
}

// SetHeader records the lines (nil clears) and sets Header.
func (f *Fake) SetHeader(lines []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Headers = append(f.Headers, slices.Clone(lines))
	if err := f.fail("SetHeader"); err != nil {
		return err
	}
	f.Header = slices.Clone(lines)
	return nil
}

// OnTerminalInput keeps the handler in TerminalInputHandlers and returns the
// idempotent function that removes it. Without a UI nothing is kept; a nil
// handler is rejected as the SDK rejects it.
func (f *Fake) OnTerminalInput(handler sdk.TerminalInputHandler) (func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("OnTerminalInput"); err != nil {
		return func() {}, err
	}
	if !f.UI {
		return func() {}, nil
	}
	if handler == nil {
		return func() {}, errors.New("OnTerminalInput: handler must not be nil")
	}
	f.nextID++
	id := f.nextID
	f.TerminalInputHandlers = append(f.TerminalInputHandlers, handler)
	f.terminalIDs = append(f.terminalIDs, id)
	return sync.OnceFunc(func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		if i := slices.Index(f.terminalIDs, id); i >= 0 {
			f.terminalIDs = slices.Delete(f.terminalIDs, i, i+1)
			f.TerminalInputHandlers = slices.Delete(f.TerminalInputHandlers, i, i+1)
		}
	}), nil
}

// OnWidthChange keeps the handler in WidthHandlers and returns the
// idempotent function that removes it. A nil handler is rejected as the SDK
// rejects it.
func (f *Fake) OnWidthChange(handler sdk.WidthChangeHandler) (func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("OnWidthChange"); err != nil {
		return func() {}, err
	}
	if handler == nil {
		return func() {}, errors.New("OnWidthChange: handler must not be nil")
	}
	f.nextID++
	id := f.nextID
	f.WidthHandlers = append(f.WidthHandlers, handler)
	f.widthIDs = append(f.widthIDs, id)
	return sync.OnceFunc(func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		if i := slices.Index(f.widthIDs, id); i >= 0 {
			f.widthIDs = slices.Delete(f.widthIDs, i, i+1)
			f.WidthHandlers = slices.Delete(f.WidthHandlers, i, i+1)
		}
	}), nil
}

// FeedTerminalInput delivers one chunk to the terminal input handlers in
// subscription order, as the host does: a handler's Data replaces the chunk
// for the handlers after it, and the first handler that consumes it ends the
// delivery. The result is the final verdict: consumed, and the chunk as the
// handlers left it when one of them replaced it.
func (f *Fake) FeedTerminalInput(data string) sdk.TerminalInputResult {
	f.mu.Lock()
	handlers := slices.Clone(f.TerminalInputHandlers)
	f.mu.Unlock()
	var result sdk.TerminalInputResult
	for _, handler := range handlers {
		verdict := handler(data)
		if verdict.Data != nil {
			data = *verdict.Data
			replaced := data
			result.Data = &replaced
		}
		if verdict.Consume {
			result.Consume = true
			return result
		}
	}
	return result
}

// Resize sets TermWidth and runs the width handlers in subscription order,
// one after the other on the calling goroutine, with a zero sdk.Context: a
// handler that needs the host must close over the host value it was built
// with.
func (f *Fake) Resize(width int) {
	f.mu.Lock()
	f.TermWidth = width
	handlers := slices.Clone(f.WidthHandlers)
	f.mu.Unlock()
	for _, handler := range handlers {
		handler(sdk.Context{}, width)
	}
}
