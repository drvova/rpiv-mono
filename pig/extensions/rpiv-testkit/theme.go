package testkit

import (
	"sync"

	host "github.com/juicesharp/rpiv-mono/pig/extensions/rpiv-host"
)

// TagStyler is a Styler whose output names the style it applied, so a test
// can assert on tokens without decoding escape sequences: Fg wraps text in
// "<token>...</token>", Bg in "<bg:token>...</bg:token>", and the modifiers
// in "<b>", "<i>", "<u>", "<s>" and "<inv>". Empty text stays empty, as
// sdk.UITheme leaves it. It is the Fake's default theme; tui.PlainStyler is
// the identity alternative.
type TagStyler struct{}

var _ host.Styler = TagStyler{}

// Fg wraps text in <token></token>.
func (TagStyler) Fg(token, text string) string { return tag(token, text) }

// Bg wraps text in <bg:token></bg:token>.
func (TagStyler) Bg(token, text string) string { return tag("bg:"+token, text) }

// Bold wraps text in <b></b>.
func (TagStyler) Bold(text string) string { return tag("b", text) }

// Italic wraps text in <i></i>.
func (TagStyler) Italic(text string) string { return tag("i", text) }

// Underline wraps text in <u></u>.
func (TagStyler) Underline(text string) string { return tag("u", text) }

// Strikethrough wraps text in <s></s>.
func (TagStyler) Strikethrough(text string) string { return tag("s", text) }

// Inverse wraps text in <inv></inv>.
func (TagStyler) Inverse(text string) string { return tag("inv", text) }

func tag(name, text string) string {
	if text == "" {
		return ""
	}
	return "<" + name + ">" + text + "</" + name + ">"
}

// InvalidateSpy counts the redraw requests a component makes: hand Func to
// the component's SetInvalidate (the subprocess form of tui.requestRender)
// and read Calls.
type InvalidateSpy struct {
	mu    sync.Mutex
	calls int
}

// Invalidate counts one request.
func (s *InvalidateSpy) Invalidate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
}

// Calls is the number of requests so far.
func (s *InvalidateSpy) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// Func is Invalidate as the callback a component takes.
func (s *InvalidateSpy) Func() func() { return s.Invalidate }
