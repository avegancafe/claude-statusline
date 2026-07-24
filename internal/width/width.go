// Package width measures the terminal display columns of styled statusline
// output.
package width

import (
	"regexp"

	"github.com/mattn/go-runewidth"
)

const esc = "\x1b"

var (
	// ansiPattern strips SGR escape codes: truecolor (38;2;r;g;b / 48;2;r;g;b),
	// 256-colour (38;5;n), compound sequences (1;32), and the bare reset
	// (\033[0m) that style.Wrap emits after every span.
	ansiPattern = regexp.MustCompile(esc + `\[[0-9;]*m`)

	// oscHyperlinkPattern strips OSC 8 hyperlink wrappers, including the URL
	// they carry — WrapHyperlink emits it both as the opening sequence's
	// payload and as the empty-URL closing sequence, and neither is drawn.
	oscHyperlinkPattern = regexp.MustCompile(esc + `\]8;;[^` + esc + `]*` + esc + `\\`)
)

// condition is an explicit, locale-independent width table. The package-level
// runewidth.StringWidth and runewidth.NewCondition() are BANNED here: both
// are locale-derived (init() -> handleEnv() reads LANG/LC_CTYPE and mutates
// the shared default), so the same glyph measures differently depending on
// the calling process's environment. An explicit Condition sidesteps that.
var condition = &runewidth.Condition{EastAsianWidth: false, StrictEmojiNeutral: false}

// Visible returns the number of terminal display columns s would occupy,
// ignoring ANSI SGR escape codes and OSC 8 hyperlink wrappers.
func Visible(s string) int {
	s = ansiPattern.ReplaceAllString(s, "")
	s = oscHyperlinkPattern.ReplaceAllString(s, "")

	return condition.StringWidth(s)
}
