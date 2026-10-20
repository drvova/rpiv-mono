package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

// DefaultConfigFile is the file name ConfigPath, LegacyConfigPath and LoadJSONConfigWithLegacyFallback use
// when the caller gives none.
const DefaultConfigFile = "config.json"

// configFileMode is the mode SaveJSONConfig sets on a written file (user read/write only).
const configFileMode = 0o600

// ---------------------------------------------------------------------------
// Path resolution
// ---------------------------------------------------------------------------

// homeDir is Node's os.homedir(): $HOME when set, else the home directory of the effective user's passwd
// entry, else "" (which makes every path below relative, exactly as join("", ...) does in Node).
func homeDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	if u, err := user.Current(); err == nil {
		return u.HomeDir
	}
	return ""
}

// expandTilde expands a leading "~" to the user's home directory. It mirrors Pi's expandTildePath and uses
// filepath.Join (not concatenation) so the separator is right on every platform.
//
// Only a bare "~" and "~/…" expand. A "~user" form is returned unchanged (the XDG spec defines no "~user"
// expansion), so it fails the downstream absolute-path check and routes to the default config directory.
func expandTilde(p string) string {
	if p == "~" {
		return homeDir()
	}
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(homeDir(), rest)
	}
	return p
}

// defaultConfigDir is "~/.config", the single source of the ".config" literal.
func defaultConfigDir() string {
	return filepath.Join(homeDir(), ".config")
}

// resolveConfigDir resolves the config directory honoring XDG_CONFIG_HOME.
//
// XDG_CONFIG_HOME governs the rpiv-* sibling config layer only. Host-native paths (PiG's ~/.pig, Pi's
// ~/.pi) are a separate, orthogonal concern and are deliberately not unified here.
//
// XDG spec compliance:
//   - unset / empty after trimming / whitespace-only → the default ("~/.config")
//   - relative path → the default (XDG mandates an absolute path)
//   - "~" or "~/…" → the tilde is expanded, then the result must be absolute
//   - "~user…" → not expanded (XDG defines no "~user" form); it silently routes to the default like any
//     other relative path
//   - absolute → used verbatim
func resolveConfigDir() string {
	xdg := ReadEnvVar("XDG_CONFIG_HOME")
	if xdg == "" {
		return defaultConfigDir()
	}
	expanded := expandTilde(xdg)
	if filepath.IsAbs(expanded) {
		return expanded
	}
	return defaultConfigDir()
}

// fileName returns the optional file argument of the path functions, or DefaultConfigFile. Only the first
// element is meaningful; the variadic form stands in for the TypeScript optional parameter.
func fileName(file []string) string {
	if len(file) > 0 {
		return file[0]
	}
	return DefaultConfigFile
}

// LegacyConfigPath is the always-legacy config path "~/.config/<name>/<file>" (file defaults to
// "config.json"). The legacy-fallback reader uses it so a pre-XDG config file is still discovered after an
// operator sets XDG_CONFIG_HOME. It ignores XDG_CONFIG_HOME by design.
func LegacyConfigPath(name string, file ...string) string {
	return filepath.Join(defaultConfigDir(), name, fileName(file))
}

// ConfigPath resolves a config file path under the XDG-aware config directory:
// "<config dir>/<name>/<file>", which defaults to "~/.config/<name>/config.json" when XDG_CONFIG_HOME is
// unset, empty, whitespace or relative (see resolveConfigDir for the full rules).
//
// name is the package directory name (for example "rpiv-todo"); file is the config file name and defaults
// to "config.json". The result is absolute whenever a home directory can be determined. It is the same
// path the TypeScript packages use, so Pi and PiG share one config file per package.
func ConfigPath(name string, file ...string) string {
	return filepath.Join(resolveConfigDir(), name, fileName(file))
}

// ---------------------------------------------------------------------------
// JSON config load
// ---------------------------------------------------------------------------

// warnInvalid is the diagnostic for the malformed-config path. A silent save plus a silent load would
// produce identical user-visible symptoms (state reverts on next start) with no way to tell "never saved"
// from "saved but unreadable". The file lives in the user's HOME, so the warning is owner-only.
func warnInvalid(path string, err error) {
	fmt.Fprintf(os.Stderr, "rpiv-config: invalid JSON at %s, using default ({}) — %s\n", path, err)
}

// LoadJSONConfig loads and parses a JSON config file.
//
// It returns an empty (non-nil) map for a missing file, for malformed JSON or an unreadable file (both
// warn on stderr as "rpiv-config: invalid JSON at <path>, using default ({}) — <error>"), and silently for
// a valid JSON document that is not a plain object (null, a string, a number, a boolean or an array):
// config files are always plain objects. Numbers decode as float64, as they do in JavaScript.
func LoadJSONConfig(path string) map[string]any {
	if _, err := os.Stat(path); err != nil {
		return map[string]any{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		warnInvalid(path, err)
		return map[string]any{}
	}
	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		warnInvalid(path, err)
		return map[string]any{}
	}
	obj, ok := parsed.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return obj
}

// LoadJSONConfigWithLegacyFallback loads a JSON config, preferring the XDG-resolved path and falling back
// to the legacy "~/.config/<name>/<file>" location only when the XDG path is missing.
//
//   - XDG file present (valid or malformed) → its parse result wins. A malformed XDG file warns and returns
//     an empty map but does not silently fall back to legacy: corruption is surfaced, not masked.
//   - XDG file missing → the legacy path is read (empty map if also missing, warning plus empty map if
//     malformed).
//   - Both missing → an empty map.
//
// name is the package directory name (for example "rpiv-todo"); file defaults to "config.json".
func LoadJSONConfigWithLegacyFallback(name string, file ...string) map[string]any {
	xdgPath := ConfigPath(name, file...)
	if _, err := os.Stat(xdgPath); err == nil {
		return LoadJSONConfig(xdgPath)
	}
	return LoadJSONConfig(LegacyConfigPath(name, file...))
}

// ---------------------------------------------------------------------------
// JSON config save
// ---------------------------------------------------------------------------

// marshalConfig encodes data the way JSON.stringify(data, null, 2) + "\n" does: two-space indentation,
// no HTML escaping of "<", ">" and "&", and exactly one trailing newline.
func marshalConfig(data any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// SaveJSONConfig persists data as formatted JSON (two-space indent, trailing newline). It creates the parent
// directories and returns true on a successful write, false on any filesystem or encoding failure (disk
// full, EACCES, EROFS, a value encoding/json cannot encode, …).
//
// Every rpiv-* save call site is user-initiated and shows a "Saved …" notification; callers must guard the
// success notification on the returned bool so a silent disk failure cannot make the message lie.
//
// After the write the file mode is set to 0600 on a best-effort basis: some filesystems (tmpfs, network
// mounts, Windows-style permissions) ignore chmod, and the attempt never affects the return value.
func SaveJSONConfig(path string, data any) bool {
	// Node's mkdirSync and writeFileSync defaults (0777 and 0666 before the umask); chmod narrows the file.
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		return false
	}
	encoded, err := marshalConfig(data)
	if err != nil {
		return false
	}
	if err := os.WriteFile(path, encoded, 0o666); err != nil {
		return false
	}
	_ = os.Chmod(path, configFileMode) // best effort only, never gates success
	return true
}

// ---------------------------------------------------------------------------
// GuidanceFields
// ---------------------------------------------------------------------------

// GuidanceFields are the optional prompt-guidance overrides a tool-providing package reads from its config
// file. A zero field means "absent"; ValidateGuidanceFields never sets an empty value. The JSON names are
// the TypeScript ones, so the struct can be embedded in a larger config struct the way GuidanceFieldsSchema
// is composed into a larger TypeBox schema.
type GuidanceFields struct {
	PromptSnippet    string   `json:"promptSnippet,omitempty"`
	PromptGuidelines []string `json:"promptGuidelines,omitempty"`
	Description      string   `json:"description,omitempty"`
}

// ValidateGuidanceFields validates and extracts guidance fields from an unknown value and returns a clean
// GuidanceFields holding only the valid entries: promptSnippet and description when each is a non-empty
// string, promptGuidelines when it is a non-empty list of non-empty strings. Everything else is dropped.
//
// fields is normally the raw sub-object of a loaded config (a map[string]any, with promptGuidelines as a
// []any); a GuidanceFields or *GuidanceFields value is re-validated by the same rules. Any other value,
// including nil, yields the zero GuidanceFields. The returned slice is a copy.
func ValidateGuidanceFields(fields any) GuidanceFields {
	var snippet, guidelines, description any
	switch v := fields.(type) {
	case map[string]any:
		snippet, guidelines, description = v["promptSnippet"], v["promptGuidelines"], v["description"]
	case GuidanceFields:
		snippet, guidelines, description = v.PromptSnippet, v.PromptGuidelines, v.Description
	case *GuidanceFields:
		if v == nil {
			return GuidanceFields{}
		}
		snippet, guidelines, description = v.PromptSnippet, v.PromptGuidelines, v.Description
	default:
		return GuidanceFields{}
	}
	var result GuidanceFields
	if s, ok := snippet.(string); ok && s != "" {
		result.PromptSnippet = s
	}
	if list, ok := nonEmptyStrings(guidelines); ok {
		result.PromptGuidelines = list
	}
	if s, ok := description.(string); ok && s != "" {
		result.Description = s
	}
	return result
}

// nonEmptyStrings reports whether v is a non-empty list ([]any as decoded from JSON, or []string) whose
// every element is a non-empty string, and returns a fresh copy of it.
func nonEmptyStrings(v any) ([]string, bool) {
	switch list := v.(type) {
	case []string:
		if len(list) == 0 || slices.Contains(list, "") {
			return nil, false
		}
		return slices.Clone(list), true
	case []any:
		if len(list) == 0 {
			return nil, false
		}
		out := make([]string, 0, len(list))
		for _, e := range list {
			s, ok := e.(string)
			if !ok || s == "" {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}

// ---------------------------------------------------------------------------
// Model key codec: "provider/modelId" string ↔ ParsedModelKey
// ---------------------------------------------------------------------------

// ParsedModelKey is the result of ParseModelKey: the provider and model id halves of a model key.
type ParsedModelKey struct {
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`
}

// ParseModelKey parses a model key string into its components and reports whether it had one.
//
// It accepts either separator on read:
//   - "provider/modelId", the canonical form (emitted by ModelKey and persisted by every consumer since the
//     slash-canonical migration).
//   - "provider:modelId", the legacy form (persisted by rpiv-advisor 1.16+ before the migration). It is
//     rewritten on the next save by any consumer that re-serialises through ModelKey.
//
// The slash is preferred when present: "provider:foo/bar" splits on "/", so the model id is "bar" rather
// than re-introducing the legacy interpretation. No current model id contains a "/", so the asymmetry has
// no real-world ambiguity. A separator at index 0 or no separator at all is not a key (ok is false).
func ParseModelKey(key string) (parsed ParsedModelKey, ok bool) {
	if i := strings.Index(key, "/"); i >= 1 {
		return ParsedModelKey{Provider: key[:i], ModelID: key[i+1:]}, true
	}
	if i := strings.Index(key, ":"); i >= 1 {
		return ParsedModelKey{Provider: key[:i], ModelID: key[i+1:]}, true
	}
	return ParsedModelKey{}, false
}

// ModelKey composes the canonical "provider/modelId" string from a provider and a model id (the "provider"
// and "id" of a host model record). It emits the slash form only; paired with ParseModelKey's tolerant
// read, legacy colon-form persisted values migrate the next time any consumer re-serialises through it.
func ModelKey(provider, id string) string {
	return provider + "/" + id
}

// ---------------------------------------------------------------------------
// Env-var fallback
// ---------------------------------------------------------------------------

// ReadEnvVar reads an environment variable with an optional fallback.
//
// The value is trimmed the way JavaScript's String.prototype.trim trims it. When the variable is unset or
// empty after trimming, the fallback is returned, or "" when none is given (the Go spelling of the
// TypeScript undefined). Only the first fallback element is used.
func ReadEnvVar(key string, fallback ...string) string {
	if v := trimJS(os.Getenv(key)); v != "" {
		return v
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return ""
}

// trimJS trims exactly the characters String.prototype.trim removes: ECMAScript WhiteSpace (TAB, VT, FF,
// SP, NBSP, ZWNBSP and the Zs category) and LineTerminator (LF, CR, LS, PS). strings.TrimSpace differs at
// the edges: it also strips U+0085 and keeps U+FEFF.
func trimJS(s string) string {
	return strings.TrimFunc(s, isJSWhitespace)
}

func isJSWhitespace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x00A0, 0xFEFF, 0x2028, 0x2029:
		return true
	}
	return unicode.Is(unicode.Zs, r)
}
