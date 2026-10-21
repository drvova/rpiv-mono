package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Errors a command can fail with once the child is gone.
var (
	// ErrClosed reports a command sent, or still unanswered, after Close.
	ErrClosed = errors.New("rpc: client closed")
	// ErrExited reports a command the child did not answer before it exited
	// or closed its output. The wrapped text carries the exit status once
	// the child has been reaped, and "output closed" when the child closed
	// its output but keeps running.
	ErrExited = errors.New("rpc: pig exited")
)

// drainGrace is how long the exit watcher lets the readers drain output the
// child wrote before it exited; after it the pipes are closed so a grandchild
// holding them cannot keep the client alive.
const drainGrace = 2 * time.Second

// exitGrace is how long a command that lost the child's output waits for the
// child to be reaped before it fails: the output closes a moment before Wait
// returns, and the exit status says why the child died.
const exitGrace = 2 * time.Second

// Client drives one `pig --mode rpc` child over its standard streams. Every
// method is safe for concurrent use. Commands are correlated by id, so
// several may be in flight at once and answers may arrive in any order.
//
// Subscribers, event callbacks and the UI observer run on the reader
// goroutine in subscription order; one that blocks stalls every later record,
// the answers to commands included. A callback may call Close; it must not
// wait for a command, for Done or for Wait, since those need the reader. UI
// dialog handlers run on their own goroutines.
type Client struct {
	opts   Options
	cmd    *exec.Cmd
	stdin  *os.File
	stdout *os.File
	stderr *os.File

	stdinClosed atomic.Bool
	stdinOnce   sync.Once
	writeMu     sync.Mutex

	// closing is set first thing by Close: from then on the reader drains the
	// child's output without delivering events, and an unanswered command
	// fails with ErrClosed.
	closing atomic.Bool

	mu         sync.Mutex
	pending    map[string]chan Response
	subs       []subscriber
	nextSub    uint64
	uiHandler  UIHandler
	uiObserver func(UIRequest)
	exited     bool
	exitErr    error

	nextID     atomic.Uint64
	closeOnce  sync.Once
	exitedCh   chan struct{}
	readerDone chan struct{}
	stderrDone chan struct{}
	done       chan struct{}

	// dialogs counts the goroutines answering dialogs; the exit watcher joins
	// them before it closes done.
	dialogs sync.WaitGroup

	// ctx is the client's lifetime: it ends when the child exits or Close
	// runs, which releases UI dialog handlers.
	ctx    context.Context
	cancel context.CancelFunc
}

type subscriber struct {
	id uint64
	fn func(Event)
}

// Start launches `pig --mode rpc` with opts and returns once the process is
// running; it does not wait for the child to be ready, since the child
// answers its first command when it is. ctx bounds the launch and the
// process's lifetime: when ctx ends, the client is closed as by Close.
//
// The executable is ResolvePig(opts.Pig). The child is the leader of its own
// process group on Unix, so Close can reach the processes it spawned.
func Start(ctx context.Context, opts Options) (*Client, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pig, err := ResolvePig(opts.Pig)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(pig, opts.CommandArgs()...)
	cmd.Dir = opts.Cwd
	cmd.Env = append(os.Environ(), opts.Env...)
	setProcessGroup(cmd)

	// os.Pipe keeps the read ends ours, so Wait reaps the child as soon as it
	// exits instead of waiting for its output streams to close, and the
	// readers can be released on a deadline.
	var owned []*os.File
	closeOwned := func() {
		for _, file := range owned {
			_ = file.Close()
		}
	}
	pipe := func() (*os.File, *os.File, error) {
		r, w, err := os.Pipe()
		if err != nil {
			closeOwned()
			return nil, nil, fmt.Errorf("rpc: pipe: %w", err)
		}
		owned = append(owned, r, w)
		return r, w, nil
	}
	stdinR, stdinW, err := pipe()
	if err != nil {
		return nil, err
	}
	stdoutR, stdoutW, err := pipe()
	if err != nil {
		return nil, err
	}
	stderrR, stderrW, err := pipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdinR, stdoutW, stderrW
	if err := cmd.Start(); err != nil {
		closeOwned()
		return nil, fmt.Errorf("rpc: start %s: %w", pig, err)
	}
	_ = stdinR.Close()
	_ = stdoutW.Close()
	_ = stderrW.Close()

	lifetime, cancel := context.WithCancel(context.Background())
	c := &Client{
		opts:       opts,
		cmd:        cmd,
		stdin:      stdinW,
		stdout:     stdoutR,
		stderr:     stderrR,
		pending:    make(map[string]chan Response),
		exitedCh:   make(chan struct{}),
		readerDone: make(chan struct{}),
		stderrDone: make(chan struct{}),
		done:       make(chan struct{}),
		ctx:        lifetime,
		cancel:     cancel,
	}
	go c.drainStderr()
	go c.readLoop()
	go c.waitLoop()
	go c.watchContext(ctx)
	return c, nil
}

// PID returns the child's process id.
func (c *Client) PID() int { return c.cmd.Process.Pid }

// Done returns a channel that closes once the child has exited, the client's
// readers have stopped and every dialog handler has returned: no callback
// runs after it closes, and every pending command has failed. The reader
// runs the callbacks, so waiting on Done from one deadlocks; call Close there
// and wait for Done elsewhere.
func (c *Client) Done() <-chan struct{} { return c.done }

// Wait blocks until Done and returns the child's exit error: nil for exit
// status 0, an *exec.ExitError otherwise, including a kill by Close. Like
// Done, it must not be called from a callback.
func (c *Client) Wait() error {
	<-c.done
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.exitErr
}

// Close shuts the child down: it stops delivering events, closes standard
// input, which tells the child to finish, waits up to Options.CloseGrace for
// it to exit, then kills its process group. Close returns once the child is
// gone. It does not join the readers or the dialog handlers, so it is safe
// to call from a subscriber, Run's onEvent, the UI observer or a dialog
// handler; Done and Wait join them. Close is idempotent.
//
// A command still in flight is answered if its answer arrives before the
// child exits and fails with ErrClosed otherwise. Events the child writes
// after Close are not delivered; its output is still drained so it can
// finish.
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		c.closing.Store(true)
		c.cancel()
		c.closeStdin()
		timer := time.NewTimer(c.opts.closeGrace())
		defer timer.Stop()
		select {
		case <-c.exitedCh:
		case <-timer.C:
			killProcessGroup(c.cmd.Process)
		}
	})
	<-c.exitedCh
}

// Subscribe registers fn for every event (each record that is not the answer
// to a pending command) and returns the function that unregisters it. fn runs
// on the reader goroutine; a subscription made during dispatch sees later
// events only. fn may call Close; a command fn sends is not answered until fn
// returns, so fn must not wait for one.
func (c *Client) Subscribe(fn func(Event)) (unsubscribe func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextSub
	c.nextSub++
	c.subs = append(c.subs, subscriber{id: id, fn: fn})
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.subs = slices.DeleteFunc(c.subs, func(s subscriber) bool { return s.id == id })
	}
}

// Call sends one command and returns its response. cmd must carry a "type";
// its "id" is used when it is a non-empty string and assigned otherwise. A
// response with success false is returned as is, since the error text is
// part of the response; the typed wrappers turn it into a *CommandError.
//
// When ctx ends first, Call returns ctx.Err() and forgets the command; an
// answer that arrives later is delivered to subscribers as a response event.
// Once the child is gone, Call fails with ErrClosed or ErrExited.
func (c *Client) Call(ctx context.Context, cmd map[string]any) (Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	typ, _ := cmd["type"].(string)
	if typ == "" {
		return Response{}, errors.New("rpc: command has no \"type\"")
	}
	id, _ := cmd["id"].(string)
	if id == "" {
		id = c.newRequestID()
	}
	record := make(map[string]any, len(cmd)+1)
	maps.Copy(record, cmd)
	record["id"] = id
	payload, err := encodeRecord(record)
	if err != nil {
		return Response{}, fmt.Errorf("rpc: encode %s: %w", typ, err)
	}

	answer := make(chan Response, 1)
	c.mu.Lock()
	c.pending[id] = answer
	c.mu.Unlock()
	defer c.forget(id)

	if err := c.write(payload); err != nil {
		// A write fails once input is closed, or when the child died with
		// input still open; both mean no answer can arrive.
		if c.stdinClosed.Load() || c.childGone() {
			return Response{}, c.terminalError()
		}
		return Response{}, fmt.Errorf("rpc: write %s: %w", typ, err)
	}
	select {
	case response := <-answer:
		return response, nil
	case <-ctx.Done():
		return Response{}, ctx.Err()
	case <-c.readerDone:
		// An answer delivered just before the reader stopped is still queued.
		select {
		case response := <-answer:
			return response, nil
		default:
		}
		return Response{}, c.terminalError()
	}
}

// newRequestID returns the next command id. Ids are unique per client.
func (c *Client) newRequestID() string {
	return "rpiv-" + strconv.FormatUint(c.nextID.Add(1), 10)
}

// forget drops the pending slot of id, if it is still registered.
func (c *Client) forget(id string) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

// encodeRecord serializes one JSON Lines record: the object followed by LF,
// without HTML escaping.
func encodeRecord(record any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(record); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// write sends one record to the child's standard input. Writes are
// serialized so records never interleave.
func (c *Client) write(payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.stdinClosed.Load() {
		return ErrClosed
	}
	_, err := c.stdin.Write(payload)
	return err
}

// closeStdin closes the child's standard input once. It does not take
// writeMu, so a write blocked on a full pipe is released by the close.
func (c *Client) closeStdin() {
	c.stdinOnce.Do(func() {
		c.stdinClosed.Store(true)
		_ = c.stdin.Close()
	})
}

// awaitExit waits up to exitGrace for the child to be reaped, unless the
// client is closing. The child's output closes a moment before Wait returns,
// and a failure reported after that moment can name the exit status.
func (c *Client) awaitExit() {
	if c.closing.Load() {
		return
	}
	select {
	case <-c.exitedCh:
		return
	default:
	}
	timer := time.NewTimer(exitGrace)
	defer timer.Stop()
	select {
	case <-c.exitedCh:
	case <-timer.C:
	}
}

// childGone reports whether the child no longer reads commands: the client
// is closing, or the child has been reaped (waiting as awaitExit does).
func (c *Client) childGone() bool {
	c.awaitExit()
	if c.closing.Load() {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.exited
}

// terminalError is the error a command fails with once no answer can arrive.
func (c *Client) terminalError() error {
	c.awaitExit()
	if c.closing.Load() {
		return ErrClosed
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.exited {
		if c.exitErr != nil {
			return fmt.Errorf("%w: %v", ErrExited, c.exitErr)
		}
		return fmt.Errorf("%w: exit status 0", ErrExited)
	}
	return fmt.Errorf("%w: output closed", ErrExited)
}

// drainStderr copies the child's diagnostics to Options.Stderr, or discards
// them, so a full stderr pipe never blocks the child.
func (c *Client) drainStderr() {
	defer close(c.stderrDone)
	dst := c.opts.Stderr
	if dst == nil {
		dst = io.Discard
	}
	_, _ = io.Copy(dst, c.stderr)
}

// readLoop parses the child's standard output until it closes.
func (c *Client) readLoop() {
	defer close(c.readerDone)
	_ = ReadLines(c.stdout, func(line []byte) error {
		c.handleLine(line)
		return nil
	})
}

// handleLine routes one record: a response whose id is pending answers that
// command; everything else, a response nobody waits for included, is an
// event, dropped once Close has run. A line that is not a JSON object with a
// "type" is ignored, as PiG's own client ignores stray output.
func (c *Client) handleLine(line []byte) {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(line, &head); err != nil || head.Type == "" {
		return
	}
	if head.Type == EventResponse {
		var response Response
		if json.Unmarshal(line, &response) == nil && response.ID != "" {
			c.mu.Lock()
			answer, ok := c.pending[response.ID]
			if ok {
				delete(c.pending, response.ID)
			}
			c.mu.Unlock()
			if ok {
				answer <- response
				return
			}
		}
	}
	if c.closing.Load() {
		return
	}
	event := Event{Type: head.Type, Raw: line}
	if head.Type == EventExtensionUIRequest {
		c.handleUIRequest(event)
	}
	c.dispatch(event)
}

// dispatch calls a snapshot of the subscribers in order. The record reaches
// every subscriber of the snapshot, even when an earlier one called Close.
func (c *Client) dispatch(event Event) {
	c.mu.Lock()
	subs := slices.Clone(c.subs)
	c.mu.Unlock()
	for _, sub := range subs {
		sub.fn(event)
	}
}

// waitLoop reaps the child, lets the readers drain, releases the pipes,
// joins the dialog goroutines and closes done.
func (c *Client) waitLoop() {
	err := c.cmd.Wait()
	c.mu.Lock()
	c.exited = true
	c.exitErr = err
	c.mu.Unlock()
	close(c.exitedCh)
	c.cancel()
	c.closeStdin()

	timer := time.NewTimer(drainGrace)
	defer timer.Stop()
	select {
	case <-c.readerDone:
	case <-timer.C:
	}
	_ = c.stdout.Close()
	_ = c.stderr.Close()
	<-c.readerDone
	<-c.stderrDone
	// No dialog starts after the reader stopped, so the count is final.
	c.dialogs.Wait()
	close(c.done)
}

// watchContext closes the client when the Start context ends first.
func (c *Client) watchContext(ctx context.Context) {
	select {
	case <-ctx.Done():
		c.Close()
	case <-c.done:
	}
}
