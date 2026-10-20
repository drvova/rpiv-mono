package tui

// Styler is the subset of sdk.UITheme the extensions render with: a theme
// color token applied as foreground or background, and the text modifiers.
// sdk.UITheme satisfies it structurally; PlainStyler and TagStyler stand in
// for it in tests.
type Styler interface {
	Fg(token, text string) string
	Bg(token, text string) string
	Bold(text string) string
	Italic(text string) string
	Underline(text string) string
	Strikethrough(text string) string
	Inverse(text string) string
}

// PlainStyler returns every text unchanged, like the identity theme the
// TypeScript tests pass to components whose layout is under test.
type PlainStyler struct{}

// Fg returns text unchanged.
func (PlainStyler) Fg(_, text string) string { return text }

// Bg returns text unchanged.
func (PlainStyler) Bg(_, text string) string { return text }

// Bold returns text unchanged.
func (PlainStyler) Bold(text string) string { return text }

// Italic returns text unchanged.
func (PlainStyler) Italic(text string) string { return text }

// Underline returns text unchanged.
func (PlainStyler) Underline(text string) string { return text }

// Strikethrough returns text unchanged.
func (PlainStyler) Strikethrough(text string) string { return text }

// Inverse returns text unchanged.
func (PlainStyler) Inverse(text string) string { return text }

// TagStyler wraps text in readable tags, like the tagged theme fixture of the
// TypeScript tests: Fg gives <token>text</token>, Bg gives
// <bg:token>text</bg:token>, and the modifiers give <b>, <i>, <u>, <s> and
// <inv> elements, so a rendered line asserts which styles were applied where.
type TagStyler struct{}

// Fg wraps text as <token>text</token>.
func (TagStyler) Fg(token, text string) string { return "<" + token + ">" + text + "</" + token + ">" }

// Bg wraps text as <bg:token>text</bg:token>.
func (TagStyler) Bg(token, text string) string {
	return "<bg:" + token + ">" + text + "</bg:" + token + ">"
}

// Bold wraps text as <b>text</b>.
func (TagStyler) Bold(text string) string { return "<b>" + text + "</b>" }

// Italic wraps text as <i>text</i>.
func (TagStyler) Italic(text string) string { return "<i>" + text + "</i>" }

// Underline wraps text as <u>text</u>.
func (TagStyler) Underline(text string) string { return "<u>" + text + "</u>" }

// Strikethrough wraps text as <s>text</s>.
func (TagStyler) Strikethrough(text string) string { return "<s>" + text + "</s>" }

// Inverse wraps text as <inv>text</inv>.
func (TagStyler) Inverse(text string) string { return "<inv>" + text + "</inv>" }
