// Package config is the shared config I/O library of the rpiv-* PiG extensions, the Go port of
// @juicesharp/rpiv-config: XDG-aware config paths, JSON config load and save with crash-resistant
// defaults, guidance-field validation, the model-key codec, env-var fallback and schema-driven defaults.
// It is stateless: no package-level mutable state, no caches, no side effects at init.
//
// # Shared files
//
// The paths are the ones the TypeScript packages use, byte for byte: "<config dir>/<name>/<file>" with
// the config dir taken from XDG_CONFIG_HOME when it is absolute (a leading "~" or "~/" expands to the home
// directory, "~user" does not), else "~/.config". Files are written as two-space indented JSON with a
// trailing newline and mode 0600, and a pre-XDG file under "~/.config" is still read when the XDG path has
// none. A user running both Pi (TypeScript) and PiG (Go) therefore edits one config file per package, and
// either host picks up what the other saved. The home directory is $HOME, else the passwd entry of the
// effective user, as Node's os.homedir() resolves it.
//
// XDG_CONFIG_HOME governs this layer only. Host-native paths (PiG's ~/.pig and sdk.ConfigHome, Pi's ~/.pi)
// are an orthogonal concern and are not unified here.
//
// # Host dependencies
//
// None. The package is a plain library over the standard library; it never talks to the host and reads
// only HOME, XDG_CONFIG_HOME and the variables callers name in ReadEnvVar.
//
// # Config files
//
// The package owns no config file of its own; it defines where the sibling packages keep theirs
// (ConfigPath) and how they read and write them.
//
// # Divergences
//
// The port keeps every contract of the TypeScript source; where Go's type system or standard library
// cannot reproduce JavaScript, the Go behavior is the following.
//
//   - The untyped loaders return a map[string]any, so object key order is not preserved, and SaveJSONConfig
//     writes a map's keys sorted where JSON.stringify writes them in insertion order. A struct value is
//     written in field order, which consumers use to keep the TypeScript key order.
//   - SaveJSONConfig escapes U+2028 and U+2029 as   and   (encoding/json always does; the value
//     read back is identical), fails (returns false) for NaN and infinities where JSON.stringify writes
//     null, and writes a []byte as base64.
//   - The malformed-JSON warning has the TypeScript format with the Go decoder's error text after the dash.
//   - ValidateConfig is typed by its destination struct. A value of the wrong JSON type is dropped and the
//     field keeps its default, where TypeBox's Value.Clean would pass it through; a non-object input leaves
//     the defaults in place where the TypeScript version returns an empty object; and the `,string` tag
//     option is not applied. Top-level keys are matched exactly, as TypeBox matches them; inside a nested
//     value encoding/json's case-insensitive fallback applies.
//   - GuidanceFieldsSchema has no counterpart: a consumer composes guidance into a larger config struct by
//     embedding GuidanceFields, whose JSON names are the TypeScript ones.
//   - ReadEnvVar returns "" for the TypeScript undefined, and ParseModelKey returns (ParsedModelKey, false)
//     for it.
package config
