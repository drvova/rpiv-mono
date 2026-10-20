package config

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// setHome points HOME at a fresh temporary directory and clears XDG_CONFIG_HOME so every path the package
// resolves lands under the test's own "~/.config".
func setHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	return home
}

// unsetEnv removes key for the duration of the test (t.Setenv registers the restore).
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// captureStderr runs fn with os.Stderr redirected to a pipe and returns what fn wrote.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	func() {
		defer func() {
			os.Stderr = orig
			_ = w.Close()
		}()
		fn()
	}()
	out := <-done
	_ = r.Close()
	return out
}

var warnRE = regexp.MustCompile(`invalid JSON at .*using default`)

func expectWarnedOnce(t *testing.T, stderr string) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
	if stderr == "" || len(lines) != 1 {
		t.Fatalf("want exactly one warning line, got %q", stderr)
	}
	if !warnRE.MatchString(lines[0]) {
		t.Fatalf("warning %q does not match %v", lines[0], warnRE)
	}
	if !strings.HasPrefix(lines[0], "rpiv-config: invalid JSON at ") {
		t.Fatalf("warning %q lacks the rpiv-config prefix", lines[0])
	}
}

func expectEmpty(t *testing.T, got map[string]any) {
	t.Helper()
	if got == nil {
		t.Fatal("want a non-nil empty map, got nil")
	}
	if len(got) != 0 {
		t.Fatalf("want {}, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// ConfigPath
// ---------------------------------------------------------------------------

func TestConfigPath(t *testing.T) {
	home := setHome(t)
	cases := []struct {
		name string
		pkg  string
		file []string
		want string
	}{
		{"resolves to ~/.config/<name>/config.json by default", "rpiv-todo", nil, filepath.Join(home, ".config", "rpiv-todo", "config.json")},
		{"supports custom filename", "rpiv-advisor", []string{"advisor.json"}, filepath.Join(home, ".config", "rpiv-advisor", "advisor.json")},
		{"supports voice.json", "rpiv-voice", []string{"voice.json"}, filepath.Join(home, ".config", "rpiv-voice", "voice.json")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ConfigPath(tc.pkg, tc.file...); got != tc.want {
				t.Fatalf("ConfigPath(%q, %v) = %q, want %q", tc.pkg, tc.file, got, tc.want)
			}
		})
	}
}

func TestConfigPathXDGConfigHome(t *testing.T) {
	home := setHome(t)
	xdgAbs := filepath.Join(t.TempDir(), "rpiv-xdg-abs")
	xdgCustom := filepath.Join(t.TempDir(), "rpiv-xdg-custom")
	unset := "\x00unset"
	cases := []struct {
		name string
		xdg  string
		pkg  string
		file []string
		want string
	}{
		{"falls back to ~/.config when XDG_CONFIG_HOME is unset", unset, "rpiv-todo", nil, filepath.Join(home, ".config", "rpiv-todo", "config.json")},
		{"honors an absolute XDG_CONFIG_HOME", xdgAbs, "rpiv-todo", nil, filepath.Join(xdgAbs, "rpiv-todo", "config.json")},
		{`expands a "~/"-prefixed XDG_CONFIG_HOME`, "~/xdg", "rpiv-todo", nil, filepath.Join(home, "xdg", "rpiv-todo", "config.json")},
		{`resolves a bare "~" XDG_CONFIG_HOME to the home directory`, "~", "rpiv-todo", nil, filepath.Join(home, "rpiv-todo", "config.json")},
		{`falls back to ~/.config when XDG_CONFIG_HOME is ""`, "", "rpiv-todo", nil, filepath.Join(home, ".config", "rpiv-todo", "config.json")},
		{"falls back to ~/.config when XDG_CONFIG_HOME is whitespace-only", "   ", "rpiv-todo", nil, filepath.Join(home, ".config", "rpiv-todo", "config.json")},
		{"falls back to ~/.config when XDG_CONFIG_HOME is relative", "relative", "rpiv-todo", nil, filepath.Join(home, ".config", "rpiv-todo", "config.json")},
		{`falls back to ~/.config for a "~user" XDG_CONFIG_HOME (only "~" and "~/" expand)`, "~user/xdg", "rpiv-todo", nil, filepath.Join(home, ".config", "rpiv-todo", "config.json")},
		{"honors a custom filename under a set XDG_CONFIG_HOME", xdgCustom, "rpiv-advisor", []string{"advisor.json"}, filepath.Join(xdgCustom, "rpiv-advisor", "advisor.json")},
		{"trims surrounding whitespace from an absolute XDG_CONFIG_HOME", "  " + xdgAbs + "\t", "rpiv-todo", nil, filepath.Join(xdgAbs, "rpiv-todo", "config.json")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.xdg == unset {
				unsetEnv(t, "XDG_CONFIG_HOME")
			} else {
				t.Setenv("XDG_CONFIG_HOME", tc.xdg)
			}
			if got := ConfigPath(tc.pkg, tc.file...); got != tc.want {
				t.Fatalf("ConfigPath(%q, %v) = %q, want %q", tc.pkg, tc.file, got, tc.want)
			}
		})
	}
}

func TestLegacyConfigPathIgnoresXDG(t *testing.T) {
	home := setHome(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))
	cases := []struct {
		name string
		pkg  string
		file []string
		want string
	}{
		{"default file", "rpiv-todo", nil, filepath.Join(home, ".config", "rpiv-todo", "config.json")},
		{"custom file", "rpiv-advisor", []string{"advisor.json"}, filepath.Join(home, ".config", "rpiv-advisor", "advisor.json")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LegacyConfigPath(tc.pkg, tc.file...); got != tc.want {
				t.Fatalf("LegacyConfigPath(%q, %v) = %q, want %q", tc.pkg, tc.file, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// LoadJSONConfig
// ---------------------------------------------------------------------------

func TestLoadJSONConfig(t *testing.T) {
	home := setHome(t)
	path := filepath.Join(home, ".config", "rpiv-test-load", "config.json")

	t.Run("returns {} for missing file", func(t *testing.T) {
		var got map[string]any
		stderr := captureStderr(t, func() { got = LoadJSONConfig(path) })
		expectEmpty(t, got)
		if stderr != "" {
			t.Fatalf("missing file must not warn, got %q", stderr)
		}
	})

	t.Run("parses valid JSON object", func(t *testing.T) {
		writeFile(t, path, "{\n  \"hello\": \"world\",\n  \"count\": 42\n}\n")
		got := LoadJSONConfig(path)
		want := map[string]any{"hello": "world", "count": float64(42)}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("returns {} and warns once for malformed JSON", func(t *testing.T) {
		writeFile(t, path, "not json")
		var got map[string]any
		stderr := captureStderr(t, func() { got = LoadJSONConfig(path) })
		expectEmpty(t, got)
		expectWarnedOnce(t, stderr)
		if !strings.Contains(stderr, path) {
			t.Fatalf("warning %q does not name the file", stderr)
		}
	})

	t.Run("returns {} and warns once for an unreadable file (a directory)", func(t *testing.T) {
		dir := filepath.Join(home, ".config", "rpiv-test-load", "dir.json")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		stderr := captureStderr(t, func() { got = LoadJSONConfig(dir) })
		expectEmpty(t, got)
		expectWarnedOnce(t, stderr)
	})

	nonObjects := []struct {
		name    string
		content string
	}{
		{"returns {} for null content", "null\n"},
		{"returns {} for string content", "\"hello\"\n"},
		{"returns {} for number content", "42\n"},
		{"returns {} for boolean content", "true\n"},
		{"returns {} for array content", "[\n  1,\n  2,\n  3\n]\n"},
	}
	for _, tc := range nonObjects {
		t.Run(tc.name, func(t *testing.T) {
			writeFile(t, path, tc.content)
			var got map[string]any
			stderr := captureStderr(t, func() { got = LoadJSONConfig(path) })
			expectEmpty(t, got)
			if stderr != "" {
				t.Fatalf("a valid non-object document must not warn, got %q", stderr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// LoadJSONConfigWithLegacyFallback
// ---------------------------------------------------------------------------

func TestLoadJSONConfigWithLegacyFallback(t *testing.T) {
	const name = "rpiv-test-fb"
	type fixture struct {
		xdg, legacy string // file contents; "" means absent
	}
	cases := []struct {
		name     string
		files    fixture
		want     map[string]any
		wantWarn bool
	}{
		{"reads the XDG file when present", fixture{xdg: `{"from":"xdg"}`}, map[string]any{"from": "xdg"}, false},
		{"reads the legacy ~/.config file when the XDG path is missing", fixture{legacy: `{"from":"legacy"}`}, map[string]any{"from": "legacy"}, false},
		{"returns {} when both XDG and legacy paths are missing", fixture{}, map[string]any{}, false},
		{"warns + returns {} for a malformed XDG file and does NOT fall back to legacy", fixture{xdg: "not json", legacy: `{"from":"legacy"}`}, map[string]any{}, true},
		{"prefers a valid XDG file over a legacy file", fixture{xdg: `{"from":"xdg"}`, legacy: `{"from":"legacy"}`}, map[string]any{"from": "xdg"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := setHome(t)
			xdgRoot := filepath.Join(t.TempDir(), "rpiv-xdg-fallback")
			t.Setenv("XDG_CONFIG_HOME", xdgRoot)
			if tc.files.xdg != "" {
				writeFile(t, filepath.Join(xdgRoot, name, "config.json"), tc.files.xdg)
			}
			if tc.files.legacy != "" {
				writeFile(t, filepath.Join(home, ".config", name, "config.json"), tc.files.legacy)
			}
			var got map[string]any
			stderr := captureStderr(t, func() { got = LoadJSONConfigWithLegacyFallback(name) })
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			if tc.wantWarn {
				expectWarnedOnce(t, stderr)
			} else if stderr != "" {
				t.Fatalf("unexpected warning %q", stderr)
			}
		})
	}

	t.Run("honors a custom filename on both paths", func(t *testing.T) {
		home := setHome(t)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))
		writeFile(t, filepath.Join(home, ".config", name, "voice.json"), `{"from":"legacy-voice"}`)
		got := LoadJSONConfigWithLegacyFallback(name, "voice.json")
		if !reflect.DeepEqual(got, map[string]any{"from": "legacy-voice"}) {
			t.Fatalf("got %v", got)
		}
	})
}

// ---------------------------------------------------------------------------
// SaveJSONConfig
// ---------------------------------------------------------------------------

func TestSaveJSONConfig(t *testing.T) {
	home := setHome(t)
	saveDir := filepath.Join(home, ".config", "rpiv-test-save")

	t.Run("returns true on success and writes formatted JSON with trailing newline", func(t *testing.T) {
		path := filepath.Join(saveDir, "config.json")
		if ok := SaveJSONConfig(path, map[string]any{"key": "value"}); !ok {
			t.Fatal("want true")
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if want := "{\n  \"key\": \"value\"\n}\n"; string(got) != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("creates parent directories recursively", func(t *testing.T) {
		path := filepath.Join(saveDir, "nested", "config.json")
		if ok := SaveJSONConfig(path, map[string]any{"nested": true}); !ok {
			t.Fatal("want true")
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("file not written: %v", err)
		}
	})

	t.Run("sets mode 0600 on the written file", func(t *testing.T) {
		path := filepath.Join(saveDir, "mode.json")
		if ok := SaveJSONConfig(path, map[string]any{}); !ok {
			t.Fatal("want true")
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("mode = %o, want 600", perm)
		}
	})

	t.Run("writes structs in field order and round-trips through LoadJSONConfig", func(t *testing.T) {
		type sample struct {
			Zebra string   `json:"zebra"`
			Apple []string `json:"apple"`
			Count int      `json:"count"`
		}
		path := filepath.Join(saveDir, "struct.json")
		if ok := SaveJSONConfig(path, sample{Zebra: "z", Apple: []string{"a"}, Count: 2}); !ok {
			t.Fatal("want true")
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if want := "{\n  \"zebra\": \"z\",\n  \"apple\": [\n    \"a\"\n  ],\n  \"count\": 2\n}\n"; string(got) != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		loaded := LoadJSONConfig(path)
		want := map[string]any{"zebra": "z", "apple": []any{"a"}, "count": float64(2)}
		if !reflect.DeepEqual(loaded, want) {
			t.Fatalf("round trip got %v, want %v", loaded, want)
		}
	})

	t.Run("does not HTML-escape <, > and & (JSON.stringify parity)", func(t *testing.T) {
		path := filepath.Join(saveDir, "html.json")
		if ok := SaveJSONConfig(path, map[string]any{"s": "<a href=\"x\">&</a>"}); !ok {
			t.Fatal("want true")
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if want := "{\n  \"s\": \"<a href=\\\"x\\\">&</a>\"\n}\n"; string(got) != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("overwrites an existing file", func(t *testing.T) {
		path := filepath.Join(saveDir, "overwrite.json")
		writeFile(t, path, "{\n  \"old\": true,\n  \"padding\": \"xxxxxxxxxxxxxxxxxxxxxxxxxxxx\"\n}\n")
		if ok := SaveJSONConfig(path, map[string]any{"new": true}); !ok {
			t.Fatal("want true")
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if want := "{\n  \"new\": true\n}\n"; string(got) != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("returns false when mkdir/write fails (unwritable path)", func(t *testing.T) {
		// /dev/null is a character device, not a directory: mkdir under it fails with ENOTDIR on every
		// POSIX platform.
		if ok := SaveJSONConfig("/dev/null/cannot-create/config.json", map[string]any{"x": 1}); ok {
			t.Fatal("want false")
		}
	})

	t.Run("does not panic on write failure (callers may ignore the bool)", func(t *testing.T) {
		_ = SaveJSONConfig("/dev/null/cannot-create/config.json", map[string]any{})
	})

	t.Run("returns false for a value encoding/json cannot encode", func(t *testing.T) {
		path := filepath.Join(saveDir, "bad.json")
		if ok := SaveJSONConfig(path, map[string]any{"ch": make(chan int)}); ok {
			t.Fatal("want false")
		}
		if _, err := os.Stat(path); err == nil {
			t.Fatal("no file must be written when encoding fails")
		}
	})
}

// ---------------------------------------------------------------------------
// ValidateGuidanceFields
// ---------------------------------------------------------------------------

func TestValidateGuidanceFields(t *testing.T) {
	cases := []struct {
		name  string
		input any
		want  GuidanceFields
	}{
		{"returns {} for nil input", nil, GuidanceFields{}},
		{"returns {} for a nil *GuidanceFields", (*GuidanceFields)(nil), GuidanceFields{}},
		{"returns {} for non-object input", "string", GuidanceFields{}},
		{"returns {} for array input", []any{"promptSnippet"}, GuidanceFields{}},
		{"extracts valid promptSnippet", map[string]any{"promptSnippet": "hello"}, GuidanceFields{PromptSnippet: "hello"}},
		{"ignores empty promptSnippet", map[string]any{"promptSnippet": ""}, GuidanceFields{}},
		{"ignores non-string promptSnippet", map[string]any{"promptSnippet": float64(42)}, GuidanceFields{}},
		{"extracts valid promptGuidelines", map[string]any{"promptGuidelines": []any{"a", "b"}}, GuidanceFields{PromptGuidelines: []string{"a", "b"}}},
		{"extracts valid promptGuidelines given as []string", map[string]any{"promptGuidelines": []string{"a", "b"}}, GuidanceFields{PromptGuidelines: []string{"a", "b"}}},
		{"ignores empty promptGuidelines array", map[string]any{"promptGuidelines": []any{}}, GuidanceFields{}},
		{"ignores promptGuidelines with empty strings", map[string]any{"promptGuidelines": []any{"", "a"}}, GuidanceFields{}},
		{"ignores promptGuidelines with non-string entries", map[string]any{"promptGuidelines": []any{"a", float64(1)}}, GuidanceFields{}},
		{"ignores non-array promptGuidelines", map[string]any{"promptGuidelines": "a"}, GuidanceFields{}},
		{"extracts valid description", map[string]any{"description": "tool description"}, GuidanceFields{Description: "tool description"}},
		{"ignores empty description", map[string]any{"description": ""}, GuidanceFields{}},
		{"ignores non-string description", map[string]any{"description": float64(42)}, GuidanceFields{}},
		{"extracts all fields when valid", map[string]any{
			"promptSnippet":    "snippet",
			"promptGuidelines": []any{"guide"},
			"description":      "tool description",
		}, GuidanceFields{PromptSnippet: "snippet", PromptGuidelines: []string{"guide"}, Description: "tool description"}},
		{"drops unknown keys", map[string]any{"promptSnippet": "s", "other": 1}, GuidanceFields{PromptSnippet: "s"}},
		{"re-validates a GuidanceFields value", GuidanceFields{PromptSnippet: "s", PromptGuidelines: []string{""}}, GuidanceFields{PromptSnippet: "s"}},
		{"re-validates a *GuidanceFields value", &GuidanceFields{Description: "d", PromptGuidelines: []string{"g"}}, GuidanceFields{Description: "d", PromptGuidelines: []string{"g"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateGuidanceFields(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}

	t.Run("returns a copy of promptGuidelines", func(t *testing.T) {
		src := []string{"a"}
		got := ValidateGuidanceFields(map[string]any{"promptGuidelines": src})
		got.PromptGuidelines[0] = "changed"
		if src[0] != "a" {
			t.Fatal("input slice was mutated")
		}
	})
}

// ---------------------------------------------------------------------------
// ParseModelKey
// ---------------------------------------------------------------------------

func TestParseModelKey(t *testing.T) {
	cases := []struct {
		name string
		key  string
		want ParsedModelKey
		ok   bool
	}{
		{"parses canonical provider/modelId (slash form)", "anthropic/claude-sonnet-4-20250514", ParsedModelKey{"anthropic", "claude-sonnet-4-20250514"}, true},
		{"parses legacy provider:modelId (colon form, back-compat)", "anthropic:claude-sonnet-4-20250514", ParsedModelKey{"anthropic", "claude-sonnet-4-20250514"}, true},
		{"prefers slash when both separators present (slash is canonical)", "provider:foo/bar", ParsedModelKey{"provider:foo", "bar"}, true},
		{"returns not-ok when neither separator is present", "just-a-string", ParsedModelKey{}, false},
		{"returns not-ok for leading slash", "/model-id", ParsedModelKey{}, false},
		{"returns not-ok for leading colon (legacy form)", ":model-id", ParsedModelKey{}, false},
		{"returns not-ok for the empty string", "", ParsedModelKey{}, false},
		{"handles provider with hyphens (slash form)", "google-gemini/gemini-2.5-pro", ParsedModelKey{"google-gemini", "gemini-2.5-pro"}, true},
		{"handles provider with hyphens (legacy colon form)", "google-gemini:gemini-2.5-pro", ParsedModelKey{"google-gemini", "gemini-2.5-pro"}, true},
		{"splits on the first slash only", "a/b/c", ParsedModelKey{"a", "b/c"}, true},
		{"keeps an empty model id after the separator", "anthropic/", ParsedModelKey{"anthropic", ""}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseModelKey(tc.key)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("ParseModelKey(%q) = %+v, %v; want %+v, %v", tc.key, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ModelKey
// ---------------------------------------------------------------------------

func TestModelKey(t *testing.T) {
	t.Run("emits canonical provider/modelId (slash form)", func(t *testing.T) {
		if got := ModelKey("anthropic", "claude-sonnet-4-20250514"); got != "anthropic/claude-sonnet-4-20250514" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("round-trips with ParseModelKey (slash form)", func(t *testing.T) {
		const key = "openai/o3-pro"
		parsed, ok := ParseModelKey(key)
		if !ok {
			t.Fatal("want ok")
		}
		if got := ModelKey(parsed.Provider, parsed.ModelID); got != key {
			t.Fatalf("got %q, want %q", got, key)
		}
	})

	t.Run("auto-migrates legacy colon-form input to slash-form output", func(t *testing.T) {
		// An advisor config persisted as "anthropic:opus" parses cleanly; the next save re-serialises
		// through ModelKey and writes "anthropic/opus".
		const legacy = "anthropic:opus"
		parsed, ok := ParseModelKey(legacy)
		if !ok {
			t.Fatal("want ok")
		}
		rewritten := ModelKey(parsed.Provider, parsed.ModelID)
		if rewritten != "anthropic/opus" || rewritten == legacy {
			t.Fatalf("got %q", rewritten)
		}
	})
}

// ---------------------------------------------------------------------------
// ReadEnvVar
// ---------------------------------------------------------------------------

func TestReadEnvVar(t *testing.T) {
	const key = "RPIV_TEST_VAR"
	unset := "\x00unset"
	cases := []struct {
		name     string
		value    string
		fallback []string
		want     string
	}{
		{"returns env value when set", "hello", nil, "hello"},
		{"trims whitespace from env value", "  hello  ", nil, "hello"},
		{"returns fallback when env var is unset", unset, []string{"fallback"}, "fallback"},
		{`returns "" when env var is unset and no fallback`, unset, nil, ""},
		{"returns fallback when env var is empty after trim", "   ", []string{"fallback"}, "fallback"},
		{"returns fallback when env var is empty", "", []string{"fallback"}, "fallback"},
		{"prefers the env value over the fallback", "set", []string{"fallback"}, "set"},
		{"trims JavaScript whitespace (NBSP, BOM, LS) but not NEL", string([]rune{0x00A0, 0xFEFF, 0x2028}) + "hello" + string(rune(0x0085)), nil, "hello" + string(rune(0x0085))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.value == unset {
				unsetEnv(t, key)
			} else {
				t.Setenv(key, tc.value)
			}
			if got := ReadEnvVar(key, tc.fallback...); got != tc.want {
				t.Fatalf("ReadEnvVar(%q, %v) = %q, want %q", key, tc.fallback, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ValidateConfig (the cases of the TypeScript suite; the Go contract is in validate_test.go)
// ---------------------------------------------------------------------------

type sampleConfig struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func sampleDefaults() sampleConfig { return sampleConfig{Name: "unnamed", Count: 0} }

func TestValidateConfig(t *testing.T) {
	cases := []struct {
		name   string
		value  any
		want   sampleConfig
		wantOK bool
	}{
		{"applies defaults for missing fields", map[string]any{}, sampleConfig{Name: "unnamed", Count: 0}, true},
		{"preserves provided values", map[string]any{"name": "test", "count": float64(5)}, sampleConfig{Name: "test", Count: 5}, true},
		{"strips unknown keys", map[string]any{"name": "test", "extra": "removed"}, sampleConfig{Name: "test", Count: 0}, true},
		{"keeps the defaults for non-object input (fail-soft)", "not an object", sampleDefaults(), false},
		{"keeps the defaults for nil input (fail-soft)", nil, sampleDefaults(), false},
		{"keeps the defaults for array input (fail-soft)", []any{1, 2, 3}, sampleDefaults(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dst := sampleDefaults()
			ok := ValidateConfig(&dst, tc.value)
			if ok != tc.wantOK || dst != tc.want {
				t.Fatalf("ValidateConfig(%v) = %v, dst %+v; want %v, %+v", tc.value, ok, dst, tc.wantOK, tc.want)
			}
			if got := ValidateConfigAs(sampleDefaults(), tc.value); got != tc.want {
				t.Fatalf("ValidateConfigAs(%v) = %+v, want %+v", tc.value, got, tc.want)
			}
		})
	}
}
