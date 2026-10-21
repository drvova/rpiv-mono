package workflow

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWithTimeout(t *testing.T) {
	type timeoutErr struct{ error }
	marker := &timeoutErr{errors.New("boom")}
	block := func(ctx context.Context) (int, error) { <-ctx.Done(); return 0, ctx.Err() }
	t.Run("returns the given error instance on timeout", func(t *testing.T) {
		_, err := WithTimeout(context.Background(), block, time.Millisecond, marker)
		if !errors.Is(err, marker) || err.Error() != "boom" {
			t.Fatalf("%v", err)
		}
	})
	t.Run("plain error form", func(t *testing.T) {
		_, err := WithTimeout(context.Background(), block, time.Millisecond, errors.New("timeout string"))
		if err == nil || err.Error() != "timeout string" {
			t.Fatalf("%v", err)
		}
	})
	t.Run("resolves when fn settles first", func(t *testing.T) {
		v, err := WithTimeout(context.Background(), func(context.Context) (int, error) { return 42, nil }, time.Second, errors.New("should not fire"))
		if err != nil || v != 42 {
			t.Fatalf("%v %v", v, err)
		}
	})
	t.Run("cancels fn's context when the deadline passes", func(t *testing.T) {
		observed := make(chan error, 1)
		_, err := WithTimeout(context.Background(), func(ctx context.Context) (int, error) {
			<-ctx.Done()
			observed <- ctx.Err()
			return 0, ctx.Err()
		}, time.Millisecond, marker)
		if !errors.Is(err, marker) {
			t.Fatalf("%v", err)
		}
		select {
		case got := <-observed:
			if !errors.Is(got, context.DeadlineExceeded) {
				t.Fatalf("fn saw %v", got)
			}
		case <-time.After(time.Second):
			t.Fatal("fn was never cancelled")
		}
	})
	t.Run("a cancelled parent context returns its error, not the timeout", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := WithTimeout(ctx, block, time.Second, marker)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("%v", err)
		}
	})
}

func TestDeepEqual(t *testing.T) {
	cases := []struct {
		name string
		a, b any
		want bool
	}{
		{"identical primitives", 1, 1, true},
		{"identical strings", "a", "a", true},
		{"differing primitives", 1, 2, false},
		{"int vs float by value", 1, 1.0, true},
		{"object vs nil", map[string]any{"a": 1}, nil, false},
		{"nil vs object", nil, map[string]any{"a": 1}, false},
		{"array vs string", []any{1}, "x", false},
		{"nil vs empty object", nil, map[string]any{}, false},
		{"array vs object", []any{1}, map[string]any{"0": 1}, false},
		{"object vs array", map[string]any{"length": 1}, []any{}, false},
		{"arrays equal by element", []any{1, 2, 3}, []any{1, 2, 3}, true},
		{"nested arrays", []any{1, []any{2, 3}}, []any{1, []any{2, 3}}, true},
		{"unequal length", []any{1, 2}, []any{1, 2, 3}, false},
		{"unequal element", []any{1, 2}, []any{1, 3}, false},
		{"objects regardless of key order", map[string]any{"a": 1, "b": 2}, map[string]any{"b": 2, "a": 1}, true},
		{"nested objects", map[string]any{"a": map[string]any{"c": 3}}, map[string]any{"a": map[string]any{"c": 3}}, true},
		{"missing key", map[string]any{"a": 1}, map[string]any{"a": 1, "b": 2}, false},
		{"extra key", map[string]any{"a": 1, "b": 2}, map[string]any{"a": 1}, false},
		{"differing value", map[string]any{"a": 1}, map[string]any{"a": 2}, false},
		{"Schema vs map[string]any", Schema{"type": "string"}, map[string]any{"type": "string"}, true},
		{"functions never compare equal and never panic", func() {}, func() {}, false},
		{"channels never compare equal", make(chan int), make(chan int), false},
		{"structs holding slices never compare equal", struct{ S []int }{[]int{1}}, struct{ S []int }{[]int{1}}, false},
		{"function vs scalar", func() {}, 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if DeepEqual(c.a, c.b) != c.want {
				t.Fatalf("DeepEqual(%v, %v) want %v", c.a, c.b, c.want)
			}
		})
	}
}

func TestResolveUnderCwd(t *testing.T) {
	if ResolveUnderCwd("/cwd", "/abs/path.md") != "/abs/path.md" {
		t.Fatal("absolute")
	}
	if ResolveUnderCwd("/cwd", "rel/path.md") != filepath.Join("/cwd", "rel/path.md") {
		t.Fatal("relative")
	}
}

func TestReadMarkSymbol(t *testing.T) {
	sym := SymbolFor("test:internal-utils-rw")
	obj := &SymbolMarks{}
	if _, ok := ReadSymbol[int](obj, sym); ok {
		t.Fatal("unset should read false")
	}
	MarkSymbol(obj, sym, 42)
	if v, ok := ReadSymbol[int](obj, sym); !ok || v != 42 {
		t.Fatalf("%v %v", v, ok)
	}
	if _, ok := ReadSymbol[string](obj, sym); ok {
		t.Fatal("type mismatch should read false")
	}
	MarkSymbol(obj, sym, nil)
	if _, ok := ReadSymbol[int](obj, sym); ok {
		t.Fatal("nil should clear")
	}
}

func TestGlobalSlot(t *testing.T) {
	t.Run("initialises lazily on first access", func(t *testing.T) {
		key := SymbolFor("test:globalSlot-lazy-init")
		ResetGlobalSlot(key)
		initCount := 0
		get := GlobalSlot(key, func() map[string]int { initCount++; return map[string]int{} })
		if initCount != 0 {
			t.Fatal("eager")
		}
		if m := get(); m == nil || initCount != 1 {
			t.Fatal("init")
		}
	})
	t.Run("returns the same value on subsequent accesses", func(t *testing.T) {
		key := SymbolFor("test:globalSlot-same-value")
		ResetGlobalSlot(key)
		get := GlobalSlot(key, func() *[]string { return &[]string{} })
		if get() != get() {
			t.Fatal("not shared")
		}
	})
	t.Run("different keys get independent slots", func(t *testing.T) {
		ResetGlobalSlot(SymbolFor("test:a"))
		ResetGlobalSlot(SymbolFor("test:b"))
		getA := GlobalSlot(SymbolFor("test:a"), func() *int { v := 1; return &v })
		getB := GlobalSlot(SymbolFor("test:b"), func() *int { v := 2; return &v })
		if getA() == getB() || *getA() != 1 || *getB() != 2 {
			t.Fatal("slots collide")
		}
	})
	t.Run("reset clears the slot so the next access re-initialises", func(t *testing.T) {
		key := SymbolFor("test:globalSlot-reset")
		ResetGlobalSlot(key)
		initCount := 0
		get := GlobalSlot(key, func() *int { initCount++; v := initCount; return &v })
		first := get()
		ResetGlobalSlot(key)
		if after := get(); after == first || initCount != 2 {
			t.Fatal("not re-initialised")
		}
	})
	t.Run("concurrent getters share one initialisation", func(t *testing.T) {
		key := SymbolFor("test:globalSlot-concurrent")
		ResetGlobalSlot(key)
		var mu sync.Mutex
		inits := 0
		get := GlobalSlot(key, func() *int { mu.Lock(); inits++; mu.Unlock(); v := 0; return &v })
		var wg sync.WaitGroup
		for range 16 {
			wg.Add(1)
			go func() { defer wg.Done(); get() }()
		}
		wg.Wait()
		if inits != 1 {
			t.Fatalf("inits %d", inits)
		}
	})
}

func TestLazyProviderRegistry(t *testing.T) {
	seq := 0
	fresh := func(onError func(error)) LazyProviderRegistry {
		seq++
		r := NewLazyProviderRegistry("test:lazy-provider-"+string(rune('a'+seq)), onError)
		r.Reset()
		return r
	}
	t.Run("runs each provider exactly once across repeated flushes", func(t *testing.T) {
		r := fresh(nil)
		runs := 0
		r.Register(func() error { runs++; return nil })
		for range 3 {
			if err := r.Flush(); err != nil {
				t.Fatal(err)
			}
		}
		if runs != 1 {
			t.Fatalf("runs %d", runs)
		}
	})
	t.Run("providers registered after a flush run on the next flush", func(t *testing.T) {
		r := fresh(nil)
		var ran []string
		r.Register(func() error { ran = append(ran, "gen-1"); return nil })
		_ = r.Flush()
		r.Register(func() error { ran = append(ran, "gen-2"); return nil })
		_ = r.Flush()
		if len(ran) != 2 || ran[0] != "gen-1" || ran[1] != "gen-2" {
			t.Fatalf("%v", ran)
		}
	})
	t.Run("a later generation runs strictly after the prior flush settles", func(t *testing.T) {
		r := fresh(nil)
		var mu sync.Mutex
		var ran []string
		gate := make(chan struct{})
		r.Register(func() error { <-gate; mu.Lock(); ran = append(ran, "slow-first"); mu.Unlock(); return nil })
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done(); _ = r.Flush() }()
		time.Sleep(10 * time.Millisecond)
		r.Register(func() error { mu.Lock(); ran = append(ran, "second"); mu.Unlock(); return nil })
		wg.Add(1)
		go func() { defer wg.Done(); _ = r.Flush() }()
		time.Sleep(10 * time.Millisecond)
		close(gate)
		wg.Wait()
		if len(ran) != 2 || ran[0] != "slow-first" || ran[1] != "second" {
			t.Fatalf("%v", ran)
		}
	})
	t.Run("onError isolates a failing provider; later registrations still flush", func(t *testing.T) {
		var errs []error
		r := fresh(func(err error) { errs = append(errs, err) })
		r.Register(func() error { return errors.New("boom") })
		if err := r.Flush(); err != nil || len(errs) != 1 {
			t.Fatalf("%v %v", err, errs)
		}
		ran := false
		r.Register(func() error { ran = true; return nil })
		if err := r.Flush(); err != nil || !ran {
			t.Fatal("later provider did not run")
		}
	})
	t.Run("a provider error without onError fails that flush only — later registrations recover", func(t *testing.T) {
		r := fresh(nil)
		r.Register(func() error { return errors.New("boom") })
		if err := r.Flush(); err == nil || err.Error() != "boom" {
			t.Fatalf("%v", err)
		}
		if err := r.Flush(); err == nil {
			t.Fatal("an empty flush returns the prior flush's result")
		}
		ran := false
		r.Register(func() error { ran = true; return nil })
		if err := r.Flush(); err != nil || !ran {
			t.Fatal("chain poisoned")
		}
	})
	t.Run("flush with nothing pending and nothing flushed resolves immediately", func(t *testing.T) {
		if err := fresh(nil).Flush(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestErrorVocabulary(t *testing.T) {
	if !IsAbortError(NewWorkflowAbortError("")) || NewWorkflowAbortError("").Error() != "aborted" || IsAbortError(errors.New("aborted")) {
		t.Fatal("abort error")
	}
	wrapped := errors.Join(errors.New("ctx"), NewWorkflowAbortError("cancelled"))
	if !IsAbortError(wrapped) {
		t.Fatal("wrapped abort error")
	}
	if (&WorkflowConfigError{Message: "limit"}).Error() != "limit" {
		t.Fatal("config error")
	}
	if InvalidError("gate", "bad").Error() != "gate: bad" {
		t.Fatal("invalid error")
	}
	if err := RequireNonEmptyString(123, "o", "req", false); err == nil || err.Error() != "o: req" {
		t.Fatal("non-string")
	}
	if err := RequireNonEmptyString(" ", "o", "req", true); err == nil {
		t.Fatal("trim")
	}
	if err := RequireNonEmptyString(" ", "o", "req", false); err != nil {
		t.Fatal("no trim")
	}
	if FormatError(errors.New("x")) != "x" || FormatError("s") != "s" || FormatError(3) != "3" {
		t.Fatal("formatError")
	}
	if AssertNever("v").Error() != "assertNever: unreachable value v" {
		t.Fatal("assertNever")
	}
	if len(NowIso()) != 24 || NowIso()[23] != 'Z' {
		t.Fatalf("nowIso %q", NowIso())
	}
	if ClampRange(nil, 1, 1, 3) != 1 || ClampRange(Int(9), 1, 1, 3) != 3 || ClampRange(Int(0), 1, 1, 3) != 1 || ClampRange(Int(2), 1, 1, 3) != 2 {
		t.Fatal("clampRange")
	}
}
