package testkit

import (
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	host "github.com/juicesharp/rpiv-mono/pig/extensions/rpiv-host"
)

// EmittedEvent is one EmitEvent call, with the payload as it was emitted
// (before the JSON round trip the handlers see).
type EmittedEvent struct {
	Channel string
	Data    any
}

// HandlerError is an error an event-bus handler returned during EmitEvent.
type HandlerError struct {
	Channel string
	Err     error
}

type subscription struct {
	id      uint64
	channel string
	handler func(data any) error
}

// SubscribeEvent registers handler on channel and returns the idempotent
// function that removes it. A nil handler is rejected with
// host.ErrNilHandler and a no-op unsubscribe, as the adapter rejects it.
func (f *Fake) SubscribeEvent(channel string, handler func(data any) error) (func(), error) {
	if handler == nil {
		return func() {}, host.ErrNilHandler
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.fail("SubscribeEvent"); err != nil {
		return func() {}, err
	}
	f.nextID++
	sub := &subscription{id: f.nextID, channel: channel, handler: handler}
	f.subscriptions = append(f.subscriptions, sub)
	return sync.OnceFunc(func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.subscriptions = slices.DeleteFunc(f.subscriptions, func(s *subscription) bool { return s == sub })
	}), nil
}

// EmitEvent records the event in Emitted and runs every handler subscribed
// to channel in registration order, outside the mutex, with the JSON round
// trip of data (objects as map[string]any, arrays as []any, numbers as
// float64, nil for nil), as the host delivers a payload. A handler's error
// goes to HandlerErrors and does not stop the handlers after it; EmitEvent
// fails only for a payload that cannot be encoded or a scripted failure.
func (f *Fake) EmitEvent(channel string, data any) error {
	f.mu.Lock()
	f.Emitted = append(f.Emitted, EmittedEvent{Channel: channel, Data: data})
	err := f.fail("EmitEvent")
	var handlers []func(data any) error
	for _, sub := range f.subscriptions {
		if sub.channel == channel {
			handlers = append(handlers, sub.handler)
		}
	}
	f.mu.Unlock()
	if err != nil {
		return err
	}
	payload, err := jsonRoundTrip(data)
	if err != nil {
		return fmt.Errorf("pi.events.emit %q: %w", channel, err)
	}
	for _, handler := range handlers {
		if herr := handler(payload); herr != nil {
			f.mu.Lock()
			f.HandlerErrors = append(f.HandlerErrors, HandlerError{Channel: channel, Err: herr})
			f.mu.Unlock()
		}
	}
	return nil
}

// Subscriptions is the number of live handlers on channel.
func (f *Fake) Subscriptions(channel string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, sub := range f.subscriptions {
		if sub.channel == channel {
			n++
		}
	}
	return n
}

// jsonRoundTrip is the value as a JSON consumer decodes it: nil for nil.
func jsonRoundTrip(data any) (any, error) {
	if data == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}
