package host

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/extensions/sdk"
	tui "github.com/juicesharp/rpiv-mono/pig/extensions/rpiv-tui"
)

// wrappedMembers are the Host methods that do not mirror an sdk.Context
// method and must not collide with one.
var wrappedMembers = map[string]bool{
	"AgentDir": true, "ConfigDirName": true, "Version": true,
	"EmitEvent": true, "SubscribeEvent": true,
	"BuildSessionContext": true, "BuildContextEntries": true, "SessionGetBranch": true,
	"Theme":     true,
	"AllModels": true, "AvailableModels": true, "FindModel": true,
	"ProviderAuthStatus": true, "ProviderDisplayName": true, "APIKeyForProvider": true, "APIKeyAndHeaders": true,
	"StreamSimple": true, "Stream": true,
}

// hostFacts are the wrapped members that wrap a package-level SDK function
// reading PIG_EXT_HOST_INFO; they work without a live context.
var hostFacts = map[string]any{
	"AgentDir":      sdk.AgentDir,
	"ConfigDirName": sdk.ConfigDirName,
	"Version":       sdk.Version,
}

// facadeDelegates names, for each wrapped member that delegates to a method
// of an SDK facade (or of sdk.Context itself), that method. SubscribeEvent is
// the one wrapped member without a same-shaped delegate: it adapts the
// handler type.
var facadeDelegates = map[string]struct {
	facade reflect.Type
	method string
}{
	"EmitEvent":           {reflect.TypeFor[sdk.EventBus](), "Emit"},
	"BuildSessionContext": {reflect.TypeFor[sdk.SessionManager](), "BuildSessionContext"},
	"BuildContextEntries": {reflect.TypeFor[sdk.SessionManager](), "BuildContextEntries"},
	"SessionGetBranch":    {reflect.TypeFor[sdk.SessionManager](), "GetBranch"},
	"Theme":               {reflect.TypeFor[sdk.Context](), "UITheme"},
	"AllModels":           {reflect.TypeFor[sdk.ModelRegistry](), "GetAll"},
	"AvailableModels":     {reflect.TypeFor[sdk.ModelRegistry](), "GetAvailable"},
	"FindModel":           {reflect.TypeFor[sdk.ModelRegistry](), "Find"},
	"ProviderAuthStatus":  {reflect.TypeFor[sdk.ModelRegistry](), "GetProviderAuthStatus"},
	"ProviderDisplayName": {reflect.TypeFor[sdk.ModelRegistry](), "GetProviderDisplayName"},
	"APIKeyForProvider":   {reflect.TypeFor[sdk.ModelRegistry](), "GetApiKeyForProvider"},
	"APIKeyAndHeaders":    {reflect.TypeFor[sdk.ModelRegistry](), "GetApiKeyAndHeaders"},
	"StreamSimple":        {reflect.TypeFor[sdk.ModelRegistry](), "StreamSimple"},
	"Stream":              {reflect.TypeFor[sdk.ModelRegistry](), "Stream"},
}

// A sibling's narrow interface written per the porting brief declares
// Theme() tui.Styler. Host satisfies it only if Theme's result is tui.Styler
// itself (host.Styler being an alias), not a look-alike interface type.
var (
	_ interface{ Theme() tui.Styler } = Host(nil)
	_ interface{ Theme() Styler }     = Host(nil)
)

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

// Every wrapped member delegates to one SDK function or facade method that
// takes the same parameters and whose results are assignable to Host's (an
// interface result such as ModelStream or tui.Styler accepts the SDK's
// concrete type). This pins FindModel to ModelRegistry.Find, APIKeyAndHeaders
// to ModelRegistry.GetApiKeyAndHeaders, ConfigDirName to sdk.ConfigDirName
// and so on: a wrapped member without a declared delegate fails.
func TestWrappedMembersDelegateToSDK(t *testing.T) {
	hostType := reflect.TypeFor[Host]()
	for name := range wrappedMembers {
		m, ok := hostType.MethodByName(name)
		if !ok {
			t.Errorf("%s: listed as wrapped but not declared on Host", name)
			continue
		}
		var delegate reflect.Type
		skip := 0 // receiver parameters on the delegate's type
		if fn, ok := hostFacts[name]; ok {
			delegate = reflect.TypeOf(fn)
		} else if d, ok := facadeDelegates[name]; ok {
			dm, ok := d.facade.MethodByName(d.method)
			if !ok {
				t.Errorf("%s: %s has no method %s", name, d.facade, d.method)
				continue
			}
			delegate, skip = dm.Type, 1
		} else if name == "SubscribeEvent" {
			continue
		} else {
			t.Errorf("%s: wrapped member with no declared SDK delegate", name)
			continue
		}
		want := m.Type
		if delegate.NumIn()-skip != want.NumIn() || delegate.NumOut() != want.NumOut() {
			t.Errorf("%s: Host has %s, SDK delegate has %s", name, want, delegate)
			continue
		}
		for j := range want.NumIn() {
			if got := delegate.In(j + skip); got != want.In(j) {
				t.Errorf("%s: parameter %d is %s on Host, %s on the SDK delegate", name, j, want.In(j), got)
			}
		}
		for j := range want.NumOut() {
			if got := delegate.Out(j); !got.AssignableTo(want.Out(j)) {
				t.Errorf("%s: result %d is %s on Host, %s on the SDK delegate", name, j, want.Out(j), got)
			}
		}
	}
	for name := range hostFacts {
		if !wrappedMembers[name] {
			t.Errorf("%s: host fact not listed as a wrapped member", name)
		}
	}
	for name := range facadeDelegates {
		if !wrappedMembers[name] {
			t.Errorf("%s: facade delegate not listed as a wrapped member", name)
		}
	}
}

// Theme's declared result is tui.Styler itself, and host.Styler is an alias
// of it rather than a second interface with the same method set.
func TestThemeResultIsTuiStyler(t *testing.T) {
	m, ok := reflect.TypeFor[Host]().MethodByName("Theme")
	if !ok {
		t.Fatal("Host has no Theme")
	}
	if got, want := m.Type.Out(0), reflect.TypeFor[tui.Styler](); got != want {
		t.Fatalf("Theme returns %s, want %s", got, want)
	}
	if got, want := reflect.TypeFor[Styler](), reflect.TypeFor[tui.Styler](); got != want {
		t.Fatalf("host.Styler is %s, want an alias of %s", got, want)
	}
}

func TestHostFactsFromHostInfo(t *testing.T) {
	cases := []struct {
		name        string
		env         string
		wantDir     string
		wantDirErr  bool
		wantCfg     string
		wantCfgErr  bool
		wantVersion string
		wantVerErr  bool
	}{
		{name: "no host", env: "", wantDirErr: true, wantCfgErr: true, wantVerErr: true},
		{name: "malformed", env: "{not json", wantDirErr: true, wantCfgErr: true, wantVerErr: true},
		{name: "full", env: `{"agentDir":"/home/u/.pig/agent","configDirName":".pig","version":"0.5.0"}`, wantDir: "/home/u/.pig/agent", wantCfg: ".pig", wantVersion: "0.5.0"},
		{name: "shares Pi's tree", env: `{"agentDir":"/home/u/.pi/agent","configDirName":".pi","version":"0.5.0"}`, wantDir: "/home/u/.pi/agent", wantCfg: ".pi", wantVersion: "0.5.0"},
		{name: "missing agentDir", env: `{"version":"0.5.0"}`, wantDirErr: true, wantCfg: "", wantVersion: "0.5.0"},
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

			cfg, err := h.ConfigDirName()
			if tc.wantCfgErr {
				if !errors.Is(err, sdk.ErrHostInfoUnavailable) {
					t.Errorf("ConfigDirName error = %v, want ErrHostInfoUnavailable", err)
				}
			} else if err != nil || cfg != tc.wantCfg {
				t.Errorf("ConfigDirName = %q, %v; want %q", cfg, err, tc.wantCfg)
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

// The members the package documentation promises are usable on a Host built
// from a zero sdk.Context return their zero values instead of panicking.
func TestZeroContextSafeMembers(t *testing.T) {
	h := FromContext(sdk.Context{})
	if done := h.Done(); done != nil {
		t.Errorf("Done = %v, want nil", done)
	}
	if err := h.Err(); err != nil {
		t.Errorf("Err = %v, want nil", err)
	}
	if id := h.ToolCallID(); id != "" {
		t.Errorf("ToolCallID = %q, want empty", id)
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

// A nil handler never reaches the host: SubscribeEvent rejects it up front
// with the bus's own error and a callable no-op unsubscribe, and busHandler
// adapts nil to nil so the bus's check would fire even if it were reached.
func TestSubscribeEventRejectsNilHandler(t *testing.T) {
	if busHandler(nil) != nil {
		t.Fatal("busHandler(nil) must be a nil bus handler, not a closure over nil")
	}
	h := FromContext(sdk.Context{})
	unsubscribe, err := h.SubscribeEvent("rpiv:test", nil)
	if !errors.Is(err, ErrNilHandler) {
		t.Fatalf("error = %v, want ErrNilHandler", err)
	}
	if unsubscribe == nil {
		t.Fatal("unsubscribe must be a no-op function, not nil")
	}
	unsubscribe()
	unsubscribe()
	_, busErr := sdk.Context{}.Events().On("rpiv:test", nil)
	if busErr == nil || busErr.Error() != ErrNilHandler.Error() {
		t.Fatalf("the SDK bus rejects a nil handler with %v, ErrNilHandler says %v", busErr, ErrNilHandler)
	}
}

// Every wrapped member's doc comment says whether it works without a live
// context, so a reader of the interface sees it without opening the package
// documentation.
func TestWrappedMemberDocsStateContextNeed(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "host.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	var iface *ast.InterfaceType
	ast.Inspect(file, func(n ast.Node) bool {
		if spec, ok := n.(*ast.TypeSpec); ok && spec.Name.Name == "Host" {
			iface, _ = spec.Type.(*ast.InterfaceType)
			return false
		}
		return true
	})
	if iface == nil {
		t.Fatal("host.go declares no Host interface")
	}
	seen := 0
	for _, field := range iface.Methods.List {
		for _, ident := range field.Names {
			if !wrappedMembers[ident.Name] {
				continue
			}
			seen++
			want := "requires a live context"
			if _, fact := hostFacts[ident.Name]; fact {
				want = "without a live context"
			}
			// Comments wrap at the gofmt margin, so compare on single spaces.
			doc := strings.Join(strings.Fields(field.Doc.Text()), " ")
			if !strings.Contains(doc, want) {
				t.Errorf("%s: doc comment must say %q; has %q", ident.Name, want, doc)
			}
		}
	}
	if seen != len(wrappedMembers) {
		t.Errorf("found %d wrapped members in host.go, want %d", seen, len(wrappedMembers))
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
