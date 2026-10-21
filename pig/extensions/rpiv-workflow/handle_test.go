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
		{"inline omits mime when not supplied", Inline([]byte{1, 2, 3}), ArtifactHandle{Kind: "inline", Bytes: []byte{1, 2, 3}}},
		{"inline attaches mime when supplied", Inline([]byte{1}, "image/png"), ArtifactHandle{Kind: "inline", Bytes: []byte{1}, Mime: String("image/png")}},
		{"inline keeps an explicitly empty mime", Inline([]byte{1}, ""), ArtifactHandle{Kind: "inline", Bytes: []byte{1}, Mime: String("")}},
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
		{"inline → byte length", Inline([]byte{1, 2, 3, 4}), "inline:4b"},
		{"inline → byte length + mime", Inline([]byte{1, 2}, "image/png"), "inline:2b;image/png"},
		{"inline → empty mime adds no suffix", Inline([]byte{1, 2}, ""), "inline:2b"},
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
	twelve := make([]byte, 12)
	for i := range twelve {
		twelve[i] = byte(i)
	}
	cases := []struct {
		name string
		h    ArtifactHandle
		json string
	}{
		{"fs", FS("a.md"), `{"kind":"fs","path":"a.md"}`},
		{"url keeps & unescaped like JSON.stringify", URL("https://x?a=1&b=<2>"), `{"kind":"url","href":"https://x?a=1&b=<2>"}`},
		{"opaque", Opaque("id"), `{"kind":"opaque","id":"id"}`},
		{"inline mirrors JSON.stringify(Uint8Array)", Inline([]byte{7, 9}, "x/y"), `{"kind":"inline","bytes":{"0":7,"1":9},"mime":"x/y"}`},
		{"inline keys stay in index order past 9", Inline(twelve, "m"), `{"kind":"inline","bytes":{"0":0,"1":1,"2":2,"3":3,"4":4,"5":5,"6":6,"7":7,"8":8,"9":9,"10":10,"11":11},"mime":"m"}`},
		{"inline without mime has no mime key", Inline([]byte{7}), `{"kind":"inline","bytes":{"0":7}}`},
		{"inline with empty mime keeps the key", Inline([]byte{7}, ""), `{"kind":"inline","bytes":{"0":7},"mime":""}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// jsonStringify is the non-HTML-escaping encoder every row writer
			// uses; json.Marshal would re-escape & < > in a Marshaler's output.
			if got := jsonStringify(c.h); got != c.json {
				t.Fatalf("got %s want %s", got, c.json)
			}
			var back ArtifactHandle
			if err := json.Unmarshal([]byte(c.json), &back); err != nil || !reflect.DeepEqual(back, c.h) {
				t.Fatalf("round trip %+v (%v)", back, err)
			}
		})
	}
	t.Run("artifact row omits empty role and nil meta, keeps an empty meta", func(t *testing.T) {
		raw, _ := json.Marshal(Artifact{Handle: FS("a.md")})
		if string(raw) != `{"handle":{"kind":"fs","path":"a.md"}}` {
			t.Fatalf("got %s", raw)
		}
		raw, _ = json.Marshal(Artifact{Handle: FS("a.md"), Meta: map[string]any{}})
		if string(raw) != `{"handle":{"kind":"fs","path":"a.md"},"meta":{}}` {
			t.Fatalf("got %s", raw)
		}
	})
}
