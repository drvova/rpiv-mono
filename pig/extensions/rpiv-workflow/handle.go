package workflow

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
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
	Mime  string
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
// matching parser needs. The framework reads artifacts[0] as the "primary"
// artifact for chain inheritance; Role is metadata only.
type Artifact struct {
	Handle ArtifactHandle `json:"handle"`
	Role   string         `json:"role,omitempty"`
	Meta   map[string]any `json:"meta,omitempty"`
}

// FS builds a filesystem handle.
func FS(path string) ArtifactHandle { return ArtifactHandle{Kind: HandleKindFS, Path: path} }

// URL builds a URL handle.
func URL(href string) ArtifactHandle { return ArtifactHandle{Kind: HandleKindURL, Href: href} }

// Opaque builds an external-id handle.
func Opaque(id string) ArtifactHandle { return ArtifactHandle{Kind: HandleKindOpaque, ID: id} }

// Inline builds an inline-bytes handle; an empty mime is omitted.
func Inline(bytes []byte, mime string) ArtifactHandle {
	return ArtifactHandle{Kind: HandleKindInline, Bytes: bytes, Mime: mime}
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
// serialise to their byte length since their content isn't promptable.
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
		if h.Mime != "" {
			s += inlineHandleMimeSeparator + h.Mime
		}
		return s
	}
	return ""
}

// MarshalJSON emits only the fields the handle's kind carries, in the
// TypeScript insertion order, so persisted rows stay byte-identical. Inline
// bytes serialise the way JSON.stringify renders a Uint8Array (an object of
// index keys), the shape the TypeScript implementation writes.
func (h ArtifactHandle) MarshalJSON() ([]byte, error) {
	switch h.Kind {
	case HandleKindFS:
		return json.Marshal(struct {
			Kind string `json:"kind"`
			Path string `json:"path"`
		}{h.Kind, h.Path})
	case HandleKindURL:
		return json.Marshal(struct {
			Kind string `json:"kind"`
			Href string `json:"href"`
		}{h.Kind, h.Href})
	case HandleKindOpaque:
		return json.Marshal(struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
		}{h.Kind, h.ID})
	case HandleKindInline:
		indexed := make(map[string]int, len(h.Bytes))
		for i, b := range h.Bytes {
			indexed[strconv.Itoa(i)] = int(b)
		}
		return json.Marshal(struct {
			Kind  string         `json:"kind"`
			Bytes map[string]int `json:"bytes"`
			Mime  string         `json:"mime,omitempty"`
		}{h.Kind, indexed, h.Mime})
	}
	return nil, fmt.Errorf("handle: unknown kind %q", h.Kind)
}

// UnmarshalJSON reads any of the four shapes MarshalJSON writes; inline bytes
// are accepted as an index-keyed object, a JSON array of numbers, or base64.
func (h *ArtifactHandle) UnmarshalJSON(data []byte) error {
	var raw struct {
		Kind  string          `json:"kind"`
		Path  string          `json:"path"`
		Href  string          `json:"href"`
		ID    string          `json:"id"`
		Mime  string          `json:"mime"`
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
