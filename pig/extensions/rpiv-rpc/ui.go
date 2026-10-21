package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// UI request methods.
const (
	UIMethodSelect        = "select"
	UIMethodConfirm       = "confirm"
	UIMethodInput         = "input"
	UIMethodEditor        = "editor"
	UIMethodNotify        = "notify"
	UIMethodSetStatus     = "setStatus"
	UIMethodSetWidget     = "setWidget"
	UIMethodSetTitle      = "setTitle"
	UIMethodSetEditorText = "set_editor_text"
)

// UIRequest is one extension_ui_request record: a dialog the child's
// extensions wait on (select, confirm, input, editor) or a fire-and-forget
// notification (notify, setStatus, setWidget, setTitle, set_editor_text).
// Only the fields of the request's method are set; Raw keeps the record.
type UIRequest struct {
	ID     string `json:"id"`
	Method string `json:"method"`

	// Dialog fields.
	Title       string   `json:"title"`
	Message     string   `json:"message"`
	Options     []string `json:"options"`
	Placeholder string   `json:"placeholder"`
	Prefill     string   `json:"prefill"`
	// Timeout, in milliseconds, is set when the dialog resolves by itself
	// (no value, or false for confirm) once it elapses.
	Timeout *float64 `json:"timeout"`

	// Fire-and-forget fields.
	NotifyType string `json:"notifyType"`
	StatusKey  string `json:"statusKey"`
	// StatusText is nil when the status entry is cleared.
	StatusText *string `json:"statusText"`
	WidgetKey  string  `json:"widgetKey"`
	// WidgetLines is nil when the widget is cleared.
	WidgetLines     *[]string `json:"widgetLines"`
	WidgetPlacement string    `json:"widgetPlacement"`
	Text            string    `json:"text"`

	Raw json.RawMessage `json:"-"`
}

// IsDialog reports whether the child waits for a response to this request.
func (r UIRequest) IsDialog() bool {
	switch r.Method {
	case UIMethodSelect, UIMethodConfirm, UIMethodInput, UIMethodEditor:
		return true
	}
	return false
}

// UIResponse answers a dialog. Cancelled wins over the other fields; for
// confirm Confirmed is sent; for the other dialogs Value is sent, and a nil
// Value cancels.
type UIResponse struct {
	Value     *string
	Confirmed bool
	Cancelled bool
}

// UIValue answers a select, input or editor dialog with value.
func UIValue(value string) UIResponse { return UIResponse{Value: &value} }

// UIConfirmed answers a confirm dialog.
func UIConfirmed(confirmed bool) UIResponse { return UIResponse{Confirmed: confirmed} }

// UICancelled cancels any dialog: the extension gets no value, or false.
func UICancelled() UIResponse { return UIResponse{Cancelled: true} }

// UIHandler answers one dialog request. ctx ends when the client closes, and
// carries the request's timeout as a deadline when it has one; a handler that
// outlives it should return UICancelled, since the child no longer listens.
type UIHandler func(ctx context.Context, req UIRequest) UIResponse

// SetUIHandler installs the dialog handler. Each dialog runs the handler on
// its own goroutine and writes the matching extension_ui_response. With no
// handler every dialog is cancelled at once.
func (c *Client) SetUIHandler(handler UIHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.uiHandler = handler
}

// SetUIObserver installs the receiver of fire-and-forget requests (notify,
// setStatus, setWidget, setTitle, set_editor_text). It runs on the reader
// goroutine. Dialogs never reach it.
func (c *Client) SetUIObserver(observer func(UIRequest)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.uiObserver = observer
}

// RespondUI writes an extension_ui_response for req. The handler path does
// this itself; RespondUI serves a consumer that answers a dialog it saw as an
// event. The child ignores a response for an id it no longer waits on.
func (c *Client) RespondUI(req UIRequest, resp UIResponse) error {
	record := map[string]any{"type": "extension_ui_response", "id": req.ID}
	switch {
	case resp.Cancelled:
		record["cancelled"] = true
	case req.Method == UIMethodConfirm:
		record["confirmed"] = resp.Confirmed
	case resp.Value != nil:
		record["value"] = *resp.Value
	default:
		record["cancelled"] = true
	}
	payload, err := encodeRecord(record)
	if err != nil {
		return err
	}
	return c.write(payload)
}

// handleUIRequest relays one extension_ui_request: dialogs to the handler on
// a new goroutine, the rest to the observer on the reader goroutine.
func (c *Client) handleUIRequest(event Event) {
	req, err := event.UIRequest()
	if err != nil || req.ID == "" {
		return
	}
	c.mu.Lock()
	handler, observer := c.uiHandler, c.uiObserver
	c.mu.Unlock()
	if !req.IsDialog() {
		if observer != nil {
			observer(req)
		}
		return
	}
	c.dialogs.Add(1)
	go c.runDialog(req, handler)
}

// runDialog answers one dialog on its own goroutine: with no handler it is
// cancelled at once; otherwise the handler runs under the client's lifetime
// context, bounded by the request's timeout, and its answer is written. The
// exit watcher joins every dialog goroutine before Done closes.
//
// An answer that cannot be written is reported to Options.OnError, unless the
// child is closed or has exited: then it no longer waits for the answer, and
// the failure is expected.
func (c *Client) runDialog(req UIRequest, handler UIHandler) {
	defer c.dialogs.Done()
	resp := UICancelled()
	if handler != nil {
		ctx := c.ctx
		if req.Timeout != nil && *req.Timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, time.Duration(*req.Timeout*float64(time.Millisecond)))
			defer cancel()
		}
		resp = handler(ctx, req)
	}
	err := c.RespondUI(req, resp)
	if err == nil || errors.Is(err, ErrClosed) || c.childGone() {
		return
	}
	if c.opts.OnError != nil {
		c.opts.OnError(fmt.Errorf("rpc: answer dialog %s: %w", req.ID, err))
	}
}
