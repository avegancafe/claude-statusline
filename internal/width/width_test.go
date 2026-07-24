package width_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
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

// bannedRunewidthSymbols are the package-level go-runewidth entry points that
// are locale-derived: runewidth's init() -> handleEnv() reads LANG/LC_CTYPE
// once at process start and mutates the shared DefaultCondition, so e.g. "▓"
// measures 1 under en_US.UTF-8 but 2 under a CJK locale through this path.
// Visible must go through an explicit *runewidth.Condition instead.
//
// A runtime test that flips LANG via t.Setenv cannot catch a regression here:
// the package's env read already happened before any test body runs, so it
// passes against both a correct and an incorrect implementation alike (see
// the retired TestVisibleLocaleIndependent, which was exactly that trap).
// This guard parses the AST of the package's own non-test sources instead,
// so it fails the moment a call to any of these symbols is reintroduced.
var bannedRunewidthSymbols = map[string]bool{
	"StringWidth":      true,
	"NewCondition":     true,
	"RuneWidth":        true,
	"DefaultCondition": true,
}

// TestNoPackageLevelRunewidthCalls is the structural guard described above.
func TestNoPackageLevelRunewidthCalls(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading package directory: %v", err)
	}

	fset := token.NewFileSet()

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		checkFileForBannedRunewidthRefs(t, fset, name)
	}
}

// checkFileForBannedRunewidthRefs parses one source file and fails the test
// for every AST reference shaped like runewidth.<bannedSymbol> it finds.
// Walking the syntax tree (rather than scanning raw text) means explanatory
// comments that name these symbols in prose never trip the guard.
func checkFileForBannedRunewidthRefs(t *testing.T, fset *token.FileSet, name string) {
	t.Helper()

	file, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}

	ast.Inspect(file, func(n ast.Node) bool {
		sel, isSelector := n.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}

		pkgIdent, isIdent := sel.X.(*ast.Ident)
		if !isIdent || pkgIdent.Name != "runewidth" || !bannedRunewidthSymbols[sel.Sel.Name] {
			return true
		}

		t.Errorf("%s:%d: banned package-level reference runewidth.%s (locale-derived; use an explicit *runewidth.Condition)",
			name, fset.Position(sel.Pos()).Line, sel.Sel.Name)

		return true
	})
}
