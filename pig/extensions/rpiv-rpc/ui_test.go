package rpc

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

// ask makes the fake open a dialog and returns the extension_ui_response the
// fake received.
func ask(t *testing.T, c *Client, members map[string]any) map[string]any {
	t.Helper()
	cmd := map[string]any{"type": "ask"}
	for key, value := range members {
		cmd[key] = value
	}
	data, err := call[struct {
		Response map[string]any `json:"response"`
	}](c, testContext(t), cmd)
	if err != nil {
		t.Fatal(err)
	}
	return data.Response
}

func TestDialogRelayRoundTrip(t *testing.T) {
	c := startFake(t)
	var seen []UIRequest
	var mu sync.Mutex
	c.SetUIHandler(func(ctx context.Context, req UIRequest) UIResponse {
		mu.Lock()
		seen = append(seen, req)
		mu.Unlock()
		switch req.Method {
		case UIMethodSelect:
			return UIValue(req.Options[1])
		case UIMethodConfirm:
			return UIConfirmed(true)
		case UIMethodInput:
			return UIValue("typed " + req.Placeholder)
		case UIMethodEditor:
			return UIValue(req.Prefill + "\nmore")
		}
		return UICancelled()
	})

	cases := []struct {
		name    string
		members map[string]any
		want    map[string]any
	}{
		{"select", map[string]any{"method": "select", "title": "Allow?", "options": []string{"Allow", "Block"}}, map[string]any{"value": "Block"}},
		{"confirm", map[string]any{"method": "confirm", "title": "Clear?", "message": "All lost."}, map[string]any{"confirmed": true}},
		{"input", map[string]any{"method": "input", "title": "Name", "placeholder": "here"}, map[string]any{"value": "typed here"}},
		{"editor", map[string]any{"method": "editor", "title": "Edit", "prefill": "line 1"}, map[string]any{"value": "line 1\nmore"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := ask(t, c, tc.members)
			if response["type"] != "extension_ui_response" || !strings.HasPrefix(response["id"].(string), "ui-") {
				t.Fatalf("response envelope %v", response)
			}
			for key, want := range tc.want {
				if response[key] != want {
					t.Fatalf("%s: %v, want %v (response %v)", key, response[key], want, response)
				}
			}
			if _, cancelled := response["cancelled"]; cancelled {
				t.Fatalf("cancelled: %v", response)
			}
		})
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 4 {
		t.Fatalf("handler saw %d requests", len(seen))
	}
	if seen[0].Title != "Allow?" || len(seen[0].Options) != 2 || seen[1].Message != "All lost." || seen[2].Placeholder != "here" || seen[3].Prefill != "line 1" {
		t.Fatalf("decoded requests %+v", seen)
	}
	if len(seen[0].Raw) == 0 {
		t.Fatal("Raw not retained")
	}
}

func TestDialogWithoutHandlerIsCancelled(t *testing.T) {
	c := startFake(t)
	response := ask(t, c, map[string]any{"method": "select", "title": "x", "options": []string{"a"}})
	if response["cancelled"] != true {
		t.Fatalf("response %v", response)
	}
	if _, ok := response["value"]; ok {
		t.Fatalf("a value was sent with the cancellation: %v", response)
	}
}

func TestDialogHandlerCancellation(t *testing.T) {
	c := startFake(t)
	c.SetUIHandler(func(context.Context, UIRequest) UIResponse { return UICancelled() })
	response := ask(t, c, map[string]any{"method": "confirm", "title": "x"})
	if response["cancelled"] != true {
		t.Fatalf("response %v", response)
	}
	if _, ok := response["confirmed"]; ok {
		t.Fatalf("confirmed was sent with the cancellation: %v", response)
	}
}

func TestDialogNilValueCancels(t *testing.T) {
	c := startFake(t)
	c.SetUIHandler(func(context.Context, UIRequest) UIResponse { return UIResponse{} })
	response := ask(t, c, map[string]any{"method": "input", "title": "x"})
	if response["cancelled"] != true {
		t.Fatalf("response %v", response)
	}
}

func TestDialogTimeoutBoundsHandlerContext(t *testing.T) {
	c := startFake(t)
	var hadDeadline bool
	var mu sync.Mutex
	c.SetUIHandler(func(ctx context.Context, req UIRequest) UIResponse {
		_, ok := ctx.Deadline()
		mu.Lock()
		hadDeadline = ok
		mu.Unlock()
		<-ctx.Done()
		return UICancelled()
	})
	started := time.Now()
	response := ask(t, c, map[string]any{"method": "select", "title": "x", "options": []string{"a"}, "timeout": 100})
	if response["cancelled"] != true {
		t.Fatalf("response %v", response)
	}
	if time.Since(started) > 5*time.Second {
		t.Fatal("the handler was not released by the timeout")
	}
	mu.Lock()
	defer mu.Unlock()
	if !hadDeadline {
		t.Fatal("handler context has no deadline for a timed dialog")
	}
}

func TestDialogHandlerReleasedOnClose(t *testing.T) {
	c := startFake(t)
	released := make(chan struct{})
	c.SetUIHandler(func(ctx context.Context, req UIRequest) UIResponse {
		<-ctx.Done()
		close(released)
		return UICancelled()
	})
	go func() {
		_, _ = c.Call(context.Background(), map[string]any{"type": "ask", "method": "select", "title": "x", "options": []string{"a"}})
	}()
	col := collect(c)
	col.waitFor(t, EventExtensionUIRequest)
	c.Close()
	select {
	case <-released:
	case <-time.After(10 * time.Second):
		t.Fatal("handler not released by Close")
	}
}

func TestDialogInsideRun(t *testing.T) {
	c := startFake(t)
	c.SetUIHandler(func(ctx context.Context, req UIRequest) UIResponse {
		if req.Method != UIMethodSelect || req.Title != "Pick one" {
			return UICancelled()
		}
		return UIValue(req.Options[0])
	})
	result, err := c.Run(testContext(t), "dialog please", PromptOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.LastAssistantText != "You chose Allow" {
		t.Fatalf("text %q", result.LastAssistantText)
	}
}

func TestFireAndForgetGoesToObserver(t *testing.T) {
	c := startFake(t)
	var observed []UIRequest
	var mu sync.Mutex
	c.SetUIObserver(func(req UIRequest) {
		mu.Lock()
		observed = append(observed, req)
		mu.Unlock()
	})
	col := collect(c)
	if err := callOnly(c, testContext(t), map[string]any{"type": "notify_me"}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(observed) != 4 {
		t.Fatalf("observed %d requests", len(observed))
	}
	notify, status, widget, cleared := observed[0], observed[1], observed[2], observed[3]
	if notify.Method != UIMethodNotify || notify.Message != "hello" || notify.NotifyType != "warning" {
		t.Fatalf("notify %+v", notify)
	}
	if status.Method != UIMethodSetStatus || status.StatusKey != "k" || status.StatusText == nil || *status.StatusText != "busy" {
		t.Fatalf("setStatus %+v", status)
	}
	if widget.Method != UIMethodSetWidget || widget.WidgetKey != "w" || widget.WidgetLines == nil || len(*widget.WidgetLines) != 2 || widget.WidgetPlacement != "aboveEditor" {
		t.Fatalf("setWidget %+v", widget)
	}
	if cleared.WidgetLines != nil {
		t.Fatalf("a widget clear carries lines: %+v", cleared)
	}
	// The requests are events too.
	count := 0
	for _, typ := range col.types() {
		if typ == EventExtensionUIRequest {
			count++
		}
	}
	if count != 4 {
		t.Fatalf("%d extension_ui_request events", count)
	}
}

func TestRespondUIShapes(t *testing.T) {
	cases := []struct {
		name string
		req  UIRequest
		resp UIResponse
		want string
	}{
		{"value", UIRequest{ID: "1", Method: UIMethodSelect}, UIValue("Allow"), `{"id":"1","type":"extension_ui_response","value":"Allow"}`},
		{"confirmed", UIRequest{ID: "2", Method: UIMethodConfirm}, UIConfirmed(false), `{"confirmed":false,"id":"2","type":"extension_ui_response"}`},
		{"cancelled", UIRequest{ID: "3", Method: UIMethodInput}, UICancelled(), `{"cancelled":true,"id":"3","type":"extension_ui_response"}`},
		{"html not escaped", UIRequest{ID: "4", Method: UIMethodEditor}, UIValue("<b>&</b>"), `{"id":"4","type":"extension_ui_response","value":"<b>&</b>"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, from, _ := newLoopback(t)
			done := make(chan error, 1)
			go func() { done <- c.RespondUI(tc.req, tc.resp) }()
			line, err := from.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(line) != tc.want {
				t.Fatalf("wrote %s\nwant  %s", strings.TrimSpace(line), tc.want)
			}
		})
	}
}

func TestUIRequestDecode(t *testing.T) {
	raw := `{"type":"extension_ui_request","id":"uuid-6","method":"setStatus","statusKey":"my-ext"}`
	req, err := Event{Type: EventExtensionUIRequest, Raw: json.RawMessage(raw)}.UIRequest()
	if err != nil {
		t.Fatal(err)
	}
	if req.IsDialog() || req.StatusText != nil || req.StatusKey != "my-ext" || string(req.Raw) != raw {
		t.Fatalf("%+v", req)
	}
	for _, method := range []string{"select", "confirm", "input", "editor"} {
		if !(UIRequest{Method: method}).IsDialog() {
			t.Fatalf("%s is a dialog", method)
		}
	}
}
