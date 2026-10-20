package host

import (
	"errors"
	"reflect"
	"testing"

	"github.com/MichaelKinsy/PiG/extensions/sdk"
)

// wrappedMembers are the Host methods that do not mirror an sdk.Context
// method and must not collide with one.
var wrappedMembers = map[string]bool{
	"AgentDir": true, "Version": true,
	"EmitEvent": true, "SubscribeEvent": true,
	"BuildSessionContext": true, "BuildContextEntries": true, "SessionGetBranch": true,
	"Theme":     true,
	"AllModels": true, "AvailableModels": true, "ProviderAuthStatus": true, "ProviderDisplayName": true, "APIKeyForProvider": true,
	"StreamSimple": true, "Stream": true,
}

func TestFromContextReturnsEmbeddingAdapter(t *testing.T) {
	h := FromContext(sdk.Context{})
	if h == nil {
		t.Fatal("FromContext returned nil")
	}
	rt := reflect.TypeOf(h)
	if rt.Kind() != reflect.Struct {
		t.Fatalf("adapter is %s, want a struct embedding sdk.Context", rt.Kind())
	}
	field, ok := rt.FieldByName("Context")
	if !ok || !field.Anonymous || field.Type != reflect.TypeFor[sdk.Context]() {
		t.Fatalf("adapter must embed sdk.Context; got field %+v, found=%v", field, ok)
	}
}

// Every Host method that is not a wrapped member must exist on sdk.Context
// with the identical parameter and result types, so that the adapter stays a
// pure embedding.
func TestHostMethodsMirrorContext(t *testing.T) {
	hostType := reflect.TypeFor[Host]()
	ctxType := reflect.TypeFor[sdk.Context]()
	mirrored := 0
	for i := range hostType.NumMethod() {
		m := hostType.Method(i)
		if wrappedMembers[m.Name] {
			continue
		}
		mirrored++
		cm, ok := ctxType.MethodByName(m.Name)
		if !ok {
			t.Errorf("%s: declared on Host but not on sdk.Context", m.Name)
			continue
		}
		// A struct method's type carries the receiver as its first parameter.
		want, got := m.Type, cm.Type
		if got.NumIn()-1 != want.NumIn() || got.NumOut() != want.NumOut() || got.IsVariadic() != want.IsVariadic() {
			t.Errorf("%s: Host has %s, sdk.Context has %s", m.Name, want, got)
			continue
		}
		for j := range want.NumIn() {
			if got.In(j+1) != want.In(j) {
				t.Errorf("%s: parameter %d is %s on Host, %s on sdk.Context", m.Name, j, want.In(j), got.In(j+1))
			}
		}
		for j := range want.NumOut() {
			if got.Out(j) != want.Out(j) {
				t.Errorf("%s: result %d is %s on Host, %s on sdk.Context", m.Name, j, want.Out(j), got.Out(j))
			}
		}
	}
	if mirrored == 0 {
		t.Fatal("no mirrored methods found")
	}
	if wrapped := hostType.NumMethod() - mirrored; wrapped != len(wrappedMembers) {
		t.Errorf("Host declares %d wrapped members, test lists %d", wrapped, len(wrappedMembers))
	}
}

// A wrapped member must not shadow an sdk.Context method, or the adapter's
// declaration would silently replace a promoted one.
func TestWrappedMembersDoNotShadowContext(t *testing.T) {
	ctxType := reflect.TypeFor[sdk.Context]()
	hostType := reflect.TypeFor[Host]()
	for name := range wrappedMembers {
		if _, ok := hostType.MethodByName(name); !ok {
			t.Errorf("%s: listed as wrapped but not declared on Host", name)
		}
		if _, ok := ctxType.MethodByName(name); ok {
			t.Errorf("%s: wrapped member shadows an sdk.Context method", name)
		}
	}
}

func TestAgentDirAndVersionFromHostInfo(t *testing.T) {
	cases := []struct {
		name        string
		env         string
		wantDir     string
		wantDirErr  bool
		wantVersion string
		wantVerErr  bool
	}{
		{name: "no host", env: "", wantDirErr: true, wantVerErr: true},
		{name: "malformed", env: "{not json", wantDirErr: true, wantVerErr: true},
		{name: "full", env: `{"agentDir":"/home/u/.pig/agent","configDirName":".pig","version":"0.5.0"}`, wantDir: "/home/u/.pig/agent", wantVersion: "0.5.0"},
		{name: "missing agentDir", env: `{"version":"0.5.0"}`, wantDirErr: true, wantVersion: "0.5.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PIG_EXT_HOST_INFO", tc.env)
			h := FromContext(sdk.Context{})

			dir, err := h.AgentDir()
			if tc.wantDirErr {
				if !errors.Is(err, sdk.ErrHostInfoUnavailable) {
					t.Errorf("AgentDir error = %v, want ErrHostInfoUnavailable", err)
				}
			} else if err != nil || dir != tc.wantDir {
				t.Errorf("AgentDir = %q, %v; want %q", dir, err, tc.wantDir)
			}

			version, err := h.Version()
			if tc.wantVerErr {
				if !errors.Is(err, sdk.ErrHostInfoUnavailable) {
					t.Errorf("Version error = %v, want ErrHostInfoUnavailable", err)
				}
			} else if err != nil || version != tc.wantVersion {
				t.Errorf("Version = %q, %v; want %q", version, err, tc.wantVersion)
			}
		})
	}
}

func TestBusHandlerForwardsPayloadOnly(t *testing.T) {
	var seen []any
	wantErr := errors.New("handler failed")
	adapted := busHandler(func(data any) error {
		seen = append(seen, data)
		if data == nil {
			return wantErr
		}
		return nil
	})
	payload := map[string]any{"id": "x", "n": float64(1)}
	if err := adapted(sdk.Context{}, payload); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := adapted(sdk.Context{}, nil); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if len(seen) != 2 || !reflect.DeepEqual(seen[0], payload) || seen[1] != nil {
		t.Fatalf("handler saw %#v", seen)
	}
}

// A theme without a palette is the identity for colors and still applies the
// modifiers; this is what a fake may hand out and what Styler promises.
func TestStylerZeroThemeStyles(t *testing.T) {
	var s Styler = sdk.UITheme{}
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"Fg", s.Fg("accent", "x"), "x"},
		{"Bg", s.Bg("accent", "x"), "x"},
		{"Bold", s.Bold("x"), "\x1b[1mx\x1b[22m"},
		{"Italic", s.Italic("x"), "\x1b[3mx\x1b[23m"},
		{"Underline", s.Underline("x"), "\x1b[4mx\x1b[24m"},
		{"Strikethrough", s.Strikethrough("x"), "\x1b[9mx\x1b[29m"},
		{"Inverse", s.Inverse("x"), "\x1b[7mx\x1b[27m"},
		{"Bold empty", s.Bold(""), ""},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}
