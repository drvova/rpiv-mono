package workflow

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ArtifactHandle is the storage-agnostic reference a collector emits and a
// parser consumes — a tagged union on Kind so collectors and parsers narrow
// structurally. Four built-in kinds cover the practical universe:
//   - "fs"     — cwd-relative or absolute filesystem path (Path).
//   - "url"    — RFC-3986 reference (Href).
//   - "opaque" — external system id (ID).
//   - "inline" — bytes the collector gathered directly (Bytes, optional Mime).
//
// Authors who need another kind write a custom collector that emits "opaque"
// and a custom parser that knows how to dereference it.
type ArtifactHandle struct {
	Kind  string
	Path  string
	Href  string
	ID    string
	Bytes []byte
	// Mime is an inline handle's optional media type: nil when the collector
	// gave none (no `mime` key in the row), otherwise the string it gave —
	// "" included, which the row keeps as `"mime": ""`.
	Mime *string
}

// Handle kind discriminators.
const (
	HandleKindFS     = "fs"
	HandleKindURL    = "url"
	HandleKindOpaque = "opaque"
	HandleKindInline = "inline"
)

// Artifact is one artifact a stage produced. The handle is the storage
// reference; Role is an optional user-facing label ("primary", "patch", "log")
// downstream stages can route on; Meta carries collector-attached hints the
// matching parser needs — absent when nil, kept (`"meta": {}`) when empty.
// The framework reads artifacts[0] as the "primary" artifact for chain
// inheritance; Role is metadata only.
type Artifact struct {
	Handle ArtifactHandle `json:"handle"`
	Role   string         `json:"role,omitempty"`
	Meta   map[string]any `json:"meta,omitzero"`
}

// FS builds a filesystem handle.
func FS(path string) ArtifactHandle { return ArtifactHandle{Kind: HandleKindFS, Path: path} }

// URL builds a URL handle.
func URL(href string) ArtifactHandle { return ArtifactHandle{Kind: HandleKindURL, Href: href} }

// Opaque builds an external-id handle.
func Opaque(id string) ArtifactHandle { return ArtifactHandle{Kind: HandleKindOpaque, ID: id} }

// Inline builds an inline-bytes handle. The optional mime (at most one) is
// recorded when given — even when empty — and absent otherwise.
func Inline(bytes []byte, mime ...string) ArtifactHandle {
	h := ArtifactHandle{Kind: HandleKindInline, Bytes: bytes}
	if len(mime) > 0 {
		given := mime[0]
		h.Mime = &given
	}
	return h
}

// Inline-handle wire format — the lossy placeholder HandleToString emits for
// inline artifact bytes: `inline:` + byte length + `b` [+ `;` mime].
const (
	inlineHandlePrefix        = "inline:"
	inlineHandleByteUnit      = "b"
	inlineHandleMimeSeparator = ";"
)

// HandleToString serialises a handle to a human-readable string — what the
// runner threads into a downstream stage's prompt input. Inline handles
// serialise to their byte length since their content isn't promptable; the
// mime suffix appears only for a non-empty mime.
func HandleToString(h ArtifactHandle) string {
	switch h.Kind {
	case HandleKindFS:
		return h.Path
	case HandleKindURL:
		return h.Href
	case HandleKindOpaque:
		return h.ID
	case HandleKindInline:
		s := inlineHandlePrefix + strconv.Itoa(len(h.Bytes)) + inlineHandleByteUnit
		if h.Mime != nil && *h.Mime != "" {
			s += inlineHandleMimeSeparator + *h.Mime
		}
		return s
	}
	return ""
}

// MarshalJSON emits only the fields the handle's kind carries, in the
// TypeScript insertion order and without HTML escaping, so persisted rows
// stay byte-identical to JSON.stringify's. Inline bytes serialise the way
// JSON.stringify renders a Uint8Array: an object of index keys in index
// order.
func (h ArtifactHandle) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	b.WriteString(`{"kind":`)
	b.WriteString(jsonStringify(h.Kind))
	switch h.Kind {
	case HandleKindFS:
		b.WriteString(`,"path":`)
		b.WriteString(jsonStringify(h.Path))
	case HandleKindURL:
		b.WriteString(`,"href":`)
		b.WriteString(jsonStringify(h.Href))
	case HandleKindOpaque:
		b.WriteString(`,"id":`)
		b.WriteString(jsonStringify(h.ID))
	case HandleKindInline:
		b.WriteString(`,"bytes":{`)
		for i, v := range h.Bytes {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteByte('"')
			b.WriteString(strconv.Itoa(i))
			b.WriteString(`":`)
			b.WriteString(strconv.Itoa(int(v)))
		}
		b.WriteByte('}')
		if h.Mime != nil {
			b.WriteString(`,"mime":`)
			b.WriteString(jsonStringify(*h.Mime))
		}
	default:
		return nil, fmt.Errorf("handle: unknown kind %q", h.Kind)
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}

// UnmarshalJSON reads any of the four shapes MarshalJSON writes; inline bytes
// are accepted as an index-keyed object, a JSON array of numbers, or base64.
func (h *ArtifactHandle) UnmarshalJSON(data []byte) error {
	var raw struct {
		Kind  string          `json:"kind"`
		Path  string          `json:"path"`
		Href  string          `json:"href"`
		ID    string          `json:"id"`
		Mime  *string         `json:"mime"`
		Bytes json.RawMessage `json:"bytes"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*h = ArtifactHandle{Kind: raw.Kind, Path: raw.Path, Href: raw.Href, ID: raw.ID, Mime: raw.Mime}
	if raw.Kind != HandleKindInline || len(raw.Bytes) == 0 {
		return nil
	}
	var indexed map[string]int
	if err := json.Unmarshal(raw.Bytes, &indexed); err == nil {
		keys := make([]int, 0, len(indexed))
		for k := range indexed {
			i, err := strconv.Atoi(k)
			if err != nil {
				return fmt.Errorf("handle: inline bytes key %q is not an index", k)
			}
			keys = append(keys, i)
		}
		sort.Ints(keys)
		h.Bytes = make([]byte, 0, len(keys))
		for _, i := range keys {
			h.Bytes = append(h.Bytes, byte(indexed[strconv.Itoa(i)]))
		}
		return nil
	}
	var list []int
	if err := json.Unmarshal(raw.Bytes, &list); err == nil {
		h.Bytes = make([]byte, len(list))
		for i, b := range list {
			h.Bytes[i] = byte(b)
		}
		return nil
	}
	var b64 []byte
	if err := json.Unmarshal(raw.Bytes, &b64); err != nil {
		return err
	}
	h.Bytes = b64
	return nil
}
