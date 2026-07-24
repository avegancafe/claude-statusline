package width_test

import (
	"testing"

	"github.com/felipeelias/claude-statusline/internal/width"
)

// TestVisibleGlyphs pins the verified go-runewidth v0.0.27 values for the
// glyph classes every built-in preset relies on. Do not fight the library:
// these are the values it returns under an explicit, non-East-Asian
// Condition, and that is the contract.
func TestVisibleGlyphs(t *testing.T) {
	tests := map[string]struct {
		input string
		want  int
	}{
		"empty string":                {"", 0},
		"ascii":                       {"abc", 3},
		"powerline solid right arrow": {"", 1},
		"powerline solid left arrow":  {"", 1},
		"medium shade block":          {"▓", 1},
		"middle dot":                  {"·", 1},
		"braille pattern":             {"⣿", 1},
		"cjk ideograph":               {"中", 2}, //nolint:gosmopolitan // pinned verified value, not stray text
		"combining acute accent":      {"é", 1},
		"warning sign with vs16":      {"⚠️", 1},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := width.Visible(tc.input); got != tc.want {
				t.Errorf("Visible(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

// TestVisibleStripsANSI asserts that SGR escape sequences never count toward
// visible width, including the bare reset Style.Wrap emits after every span.
func TestVisibleStripsANSI(t *testing.T) {
	tests := map[string]struct {
		input string
		want  int
	}{
		"truecolor foreground": {"\x1b[38;2;255;0;0mred\x1b[0m", 3},
		"truecolor background": {"\x1b[48;2;0;255;0mgreen\x1b[0m", 5},
		"256-colour":           {"\x1b[38;5;196mred\x1b[0m", 3},
		"compound bold+color":  {"\x1b[1;32mtext\x1b[0m", 4},
		"bare reset only":      {"plain\x1b[0m", 5},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := width.Visible(tc.input); got != tc.want {
				t.Errorf("Visible(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

// TestVisibleStripsHyperlink asserts that OSC 8 hyperlink wrappers disappear
// entirely, including the URL they carry, which is never drawn.
func TestVisibleStripsHyperlink(t *testing.T) {
	tests := map[string]struct {
		input string
		want  int
	}{
		"hyperlink hides url": {
			"\x1b]8;;https://example.com/very/long/path\x1b\\label\x1b]8;;\x1b\\", 5,
		},
		"empty url hyperlink": {
			"\x1b]8;;\x1b\\label\x1b]8;;\x1b\\", 5,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := width.Visible(tc.input); got != tc.want {
				t.Errorf("Visible(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

// TestVisibleLocaleIndependent guards against the package-level
// runewidth.StringWidth/NewCondition() trap: those are locale-derived via
// init()->handleEnv() reading LANG/LC_CTYPE, so "▓" measures 1 under
// en_US.UTF-8 but 2 under ja_JP.UTF-8 through that entry point. Visible must
// use an explicit Condition and stay 1 regardless of the process locale.
func TestVisibleLocaleIndependent(t *testing.T) {
	t.Setenv("LANG", "ja_JP.UTF-8")

	if got := width.Visible("▓"); got != 1 {
		t.Errorf("Visible(▓) = %d, want 1 under ja_JP.UTF-8", got)
	}
}
