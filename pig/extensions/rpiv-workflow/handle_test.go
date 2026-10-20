package workflow

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestHandleConstructors(t *testing.T) {
	cases := []struct {
		name string
		got  ArtifactHandle
		want ArtifactHandle
	}{
		{"fs tags a path", FS("/tmp/a.md"), ArtifactHandle{Kind: "fs", Path: "/tmp/a.md"}},
		{"url tags an href", URL("https://example.com/x"), ArtifactHandle{Kind: "url", Href: "https://example.com/x"}},
		{"opaque tags an id", Opaque("SHA-123"), ArtifactHandle{Kind: "opaque", ID: "SHA-123"}},
		{"inline omits mime", Inline([]byte{1, 2, 3}, ""), ArtifactHandle{Kind: "inline", Bytes: []byte{1, 2, 3}}},
		{"inline attaches mime", Inline([]byte{1}, "image/png"), ArtifactHandle{Kind: "inline", Bytes: []byte{1}, Mime: "image/png"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !reflect.DeepEqual(c.got, c.want) {
				t.Fatalf("got %+v want %+v", c.got, c.want)
			}
		})
	}
}

func TestHandleToString(t *testing.T) {
	cases := []struct {
		name string
		h    ArtifactHandle
		want string
	}{
		{"fs → path", FS("/tmp/a.md"), "/tmp/a.md"},
		{"url → href", URL("https://example.com/x"), "https://example.com/x"},
		{"opaque → id", Opaque("SHA-123"), "SHA-123"},
		{"inline → byte length", Inline([]byte{1, 2, 3, 4}, ""), "inline:4b"},
		{"inline → byte length + mime", Inline([]byte{1, 2}, "image/png"), "inline:2b;image/png"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HandleToString(c.h); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestHandleJSON(t *testing.T) {
	cases := []struct {
		name string
		h    ArtifactHandle
		json string
	}{
		{"fs", FS("a.md"), `{"kind":"fs","path":"a.md"}`},
		{"url", URL("https://x"), `{"kind":"url","href":"https://x"}`},
		{"opaque", Opaque("id"), `{"kind":"opaque","id":"id"}`},
		{"inline mirrors JSON.stringify(Uint8Array)", Inline([]byte{7, 9}, "x/y"), `{"kind":"inline","bytes":{"0":7,"1":9},"mime":"x/y"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw, err := json.Marshal(c.h)
			if err != nil || string(raw) != c.json {
				t.Fatalf("got %s (%v) want %s", raw, err, c.json)
			}
			var back ArtifactHandle
			if err := json.Unmarshal(raw, &back); err != nil || !reflect.DeepEqual(back, c.h) {
				t.Fatalf("round trip %+v (%v)", back, err)
			}
		})
	}
	t.Run("artifact row omits empty role/meta", func(t *testing.T) {
		raw, _ := json.Marshal(Artifact{Handle: FS("a.md")})
		if string(raw) != `{"handle":{"kind":"fs","path":"a.md"}}` {
			t.Fatalf("got %s", raw)
		}
	})
}
