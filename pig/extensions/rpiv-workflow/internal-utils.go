package workflow

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"
)

// Generic helpers shared across the package: error rendering, timeouts,
// process-global slots, symbol-keyed marks on function objects, structural
// equality and the construction-time error helpers. Workflow-domain helpers
// live in their domain files.

// AssertNever is the exhaustiveness guard for discriminated-union switches:
// it returns the error a reached-by-mistake default arm should surface.
func AssertNever(value any) error {
	return fmt.Errorf("assertNever: unreachable value %s", jsString(value))
}

// InvalidError builds the construction-time error whose message follows the
// authoring-surface convention `<origin>: <message>` — the ONE spelling of
// that prefix every construction-time refusal uses (DefineRoute / Gate / Match
// / Fanin / UnionCollectors / …). A plain error, no named type, so error
// posture and propagation match the inline refusals it replaces.
func InvalidError(origin, message string) error {
	return fmt.Errorf("%s: %s", origin, message)
}

// RequireNonEmptyString returns InvalidError(origin, requirement) unless value
// is a non-empty string. `trim` switches the empty check from zero-length to
// trimmed-zero-length — the lone trim site is depArtifactFlag, where a
// whitespace-only flag would inject a bare ` <path>` with no marker.
func RequireNonEmptyString(value any, origin, requirement string, trim bool) error {
	s, ok := value.(string)
	empty := !ok
	if ok {
		if trim {
			empty = strings.TrimSpace(s) == ""
		} else {
			empty = s == ""
		}
	}
	if empty {
		return InvalidError(origin, requirement)
	}
	return nil
}

// FormatError renders a caught value as a human-readable message: an error's
// message, a string verbatim, anything else through fmt — the ONE spelling of
// the error-to-string dance every failure path uses.
func FormatError(e any) string {
	switch v := e.(type) {
	case error:
		return v.Error()
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

// NowIso is the single source of ISO-8601 timestamps for audit rows and
// output meta, in the exact shape JavaScript's Date.toISOString emits
// (millisecond precision, UTC, trailing Z).
func NowIso() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

// WorkflowAbortError is the abort marker the detached-execution runtime
// returns when a child session is aborted (run-level cancellation or a
// fail-fast sibling halt). The runtime detects the aborted stop and returns
// THIS before any audit row is written, so the fold's IsAbortError check
// leaves the slot unfilled and resume re-dispatches the unit.
type WorkflowAbortError struct {
	Message string
}

// NewWorkflowAbortError builds an abort marker; an empty message reads "aborted".
func NewWorkflowAbortError(message string) *WorkflowAbortError {
	if message == "" {
		message = "aborted"
	}
	return &WorkflowAbortError{Message: message}
}

// Error implements error.
func (e *WorkflowAbortError) Error() string { return e.Message }

// IsAbortError reports whether err is (or wraps) a WorkflowAbortError. It is
// type-based, never message-based, so unrelated aborts never match.
func IsAbortError(err error) bool {
	var target *WorkflowAbortError
	return errors.As(err, &target)
}

// WorkflowConfigError is returned on a programmer-error misconfiguration of a
// runtime primitive (for example a Semaphore constructed with limit < 1) so a
// misconfiguration surfaces a typed error rather than a bare one.
type WorkflowConfigError struct {
	Message string
}

// Error implements error.
func (e *WorkflowConfigError) Error() string { return e.Message }

// Symbol is a process-wide registry key — the port of a `Symbol.for(key)`
// value. Two Symbols built from the same key are the same symbol.
type Symbol string

// SymbolFor builds the Symbol registered under key.
func SymbolFor(key string) Symbol { return Symbol(key) }

var globalSlots = struct {
	mu sync.Mutex
	m  map[Symbol]any
}{m: map[Symbol]any{}}

// GlobalSlot creates a lazily-initialised process-global slot getter anchored
// on a Symbol. The returned getter initialises the slot on first access and
// returns the same value afterwards; every getter built for the same Symbol
// shares one value. The slot table is guarded by a mutex; ResetGlobalSlot /
// ResetGlobalSlots clear it for tests.
func GlobalSlot[T any](key Symbol, init func() T) func() T {
	return func() T {
		globalSlots.mu.Lock()
		defer globalSlots.mu.Unlock()
		if value, ok := globalSlots.m[key]; ok {
			return value.(T)
		}
		value := init()
		globalSlots.m[key] = value
		return value
	}
}

// ResetGlobalSlot clears one global slot so its next access re-initialises.
func ResetGlobalSlot(key Symbol) {
	globalSlots.mu.Lock()
	defer globalSlots.mu.Unlock()
	delete(globalSlots.m, key)
}

// ResetGlobalSlots clears every global slot (the test-setup reset).
func ResetGlobalSlots() {
	globalSlots.mu.Lock()
	defer globalSlots.mu.Unlock()
	globalSlots.m = map[Symbol]any{}
}

// SymbolMarks is the symbol-keyed property bag a function object carries —
// the port of the symbol-keyed properties TypeScript attaches to plain
// function objects (the ReadsData / RouteNote / CanonicalFold family). Embed
// it (by pointer identity) in any value that must carry marks.
type SymbolMarks struct {
	mu sync.Mutex
	m  map[Symbol]any
}

// Markable is implemented by every value that carries SymbolMarks.
type Markable interface {
	symbolMarks() *SymbolMarks
}

func (s *SymbolMarks) symbolMarks() *SymbolMarks { return s }

// ReadSymbol reads a symbol-keyed mark off a function object. The second
// result is false when the symbol is unset (or set to nil).
func ReadSymbol[T any](obj Markable, sym Symbol) (T, bool) {
	var zero T
	if obj == nil {
		return zero, false
	}
	s := obj.symbolMarks()
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, ok := s.m[sym]
	if !ok || raw == nil {
		return zero, false
	}
	value, ok := raw.(T)
	return value, ok
}

// MarkSymbol writes a symbol-keyed mark onto a function object — the write
// twin of ReadSymbol. A nil value clears the mark.
func MarkSymbol(obj Markable, sym Symbol, value any) {
	s := obj.symbolMarks()
	s.mu.Lock()
	defer s.mu.Unlock()
	if value == nil {
		delete(s.m, sym)
		return
	}
	if s.m == nil {
		s.m = map[Symbol]any{}
	}
	s.m[sym] = value
}

// LazyProviderRegistry is the register-providers / flush-on-demand lifecycle
// over global slots shared by the built-in workflow registry and its
// skill-contract twin. Each provider runs AT MOST ONCE, but the flush is not a
// one-shot latch: providers registered after a flush run on the NEXT flush,
// chained after everything that already ran — which is what lets a host
// reload re-register providers and have the next consumer read refresh them.
type LazyProviderRegistry interface {
	// Register queues a lazy thunk — it runs once, on the next Flush.
	Register(provider func() error)
	// Flush runs every not-yet-run provider, strictly after prior flushes
	// settle. With no onError callback the first provider error fails THIS
	// flush (later registrations still run on subsequent flushes); with an
	// onError callback every error is recorded through it and Flush returns nil.
	// A flush with nothing pending returns the previous flush's result.
	Flush() error
	// Reset clears pending providers and the flush chain (tests).
	Reset()
}

type lazyProviders struct {
	mu   sync.Mutex
	list []func() error
}

type flushBox struct {
	mu      sync.Mutex
	flushed error
	ran     bool
}

type lazyProviderRegistry struct {
	getProviders func() *lazyProviders
	getFlushBox  func() *flushBox
	onError      func(error)
}

// NewLazyProviderRegistry builds a LazyProviderRegistry whose state (provider
// list + flush chain) is anchored on global slots derived from key, so a
// duplicate module load shares one process-wide lifecycle. onError may be nil.
func NewLazyProviderRegistry(key string, onError func(error)) LazyProviderRegistry {
	return &lazyProviderRegistry{
		getProviders: GlobalSlot(SymbolFor(key+":providers"), func() *lazyProviders { return &lazyProviders{} }),
		getFlushBox:  GlobalSlot(SymbolFor(key+":flush"), func() *flushBox { return &flushBox{} }),
		onError:      onError,
	}
}

func (r *lazyProviderRegistry) Register(provider func() error) {
	p := r.getProviders()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.list = append(p.list, provider)
}

func (r *lazyProviderRegistry) Flush() error {
	box := r.getFlushBox()
	// Serialise flushes: a later generation runs strictly after the prior one
	// settles, preserving registration order across reload generations.
	box.mu.Lock()
	defer box.mu.Unlock()
	p := r.getProviders()
	p.mu.Lock()
	pending := p.list
	p.list = nil
	p.mu.Unlock()
	if len(pending) == 0 {
		if box.ran {
			return box.flushed
		}
		return nil
	}
	// Every pending provider runs (a failing one never cancels its siblings);
	// without onError the first error is the flush's result.
	var first error
	for _, provider := range pending {
		if err := provider(); err != nil {
			if r.onError != nil {
				r.onError(err)
			} else if first == nil {
				first = err
			}
		}
	}
	box.flushed = first
	box.ran = true
	return first
}

func (r *lazyProviderRegistry) Reset() {
	p := r.getProviders()
	p.mu.Lock()
	p.list = nil
	p.mu.Unlock()
	box := r.getFlushBox()
	box.mu.Lock()
	box.flushed = nil
	box.ran = false
	box.mu.Unlock()
}

// WithTimeout runs fn under a context that is cancelled when d elapses (or
// when ctx is cancelled first) and joins its result through a buffered
// channel, so the worker never blocks on a caller that has left. On timeout
// the given timeoutErr is returned verbatim, so a typed error
// (SchemaTimeoutError and friends) survives errors.As checks; a cancellation
// of ctx itself returns ctx.Err(). fn must honour its context for the
// cancellation to take effect: whatever it returns after the caller has
// given up is discarded.
func WithTimeout[T any](ctx context.Context, fn func(ctx context.Context) (T, error), d time.Duration, timeoutErr error) (T, error) {
	type result struct {
		value T
		err   error
	}
	bounded, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	done := make(chan result, 1)
	go func() {
		value, err := fn(bounded)
		done <- result{value, err}
	}()
	select {
	case r := <-done:
		return r.value, r.err
	case <-bounded.Done():
		var zero T
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		return zero, timeoutErr
	}
}

// ResolveUnderCwd resolves p against cwd: an absolute p is returned unchanged,
// otherwise cwd and p are joined with the platform separator.
func ResolveUnderCwd(cwd, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(cwd, p)
}

// DeepEqual is the structural (key-order-independent) deep equality used by
// the cross-owner contract collision check, so two semantically identical
// contracts built by different code paths never read as divergent. Contracts
// are plain JSON data; maps, slices and scalars compare recursively, and
// numbers compare by value across Go's integer and float types.
func DeepEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if fa, ok := asFloat(a); ok {
		fb, ok := asFloat(b)
		return ok && fa == fb
	}
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	switch va.Kind() {
	case reflect.Map:
		if vb.Kind() != reflect.Map || va.Len() != vb.Len() {
			return false
		}
		iter := va.MapRange()
		for iter.Next() {
			other := vb.MapIndex(iter.Key())
			if !other.IsValid() || !DeepEqual(iter.Value().Interface(), other.Interface()) {
				return false
			}
		}
		return true
	case reflect.Slice, reflect.Array:
		if vb.Kind() != reflect.Slice && vb.Kind() != reflect.Array {
			return false
		}
		if va.Len() != vb.Len() {
			return false
		}
		for i := range va.Len() {
			if !DeepEqual(va.Index(i).Interface(), vb.Index(i).Interface()) {
				return false
			}
		}
		return true
	default:
		if vb.Kind() == reflect.Map || vb.Kind() == reflect.Slice || vb.Kind() == reflect.Array {
			return false
		}
		// Values Go cannot compare (functions, channels, structs holding
		// them) never read as equal; plain contract data carries none.
		if !va.Comparable() || !vb.Comparable() {
			return false
		}
		return a == b
	}
}

// asFloat widens any Go numeric to float64 (false for non-numerics).
func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}
