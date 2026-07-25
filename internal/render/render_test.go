package render_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/felipeelias/claude-statusline/internal/config"
	"github.com/felipeelias/claude-statusline/internal/input"
	"github.com/felipeelias/claude-statusline/internal/modules"
	"github.com/felipeelias/claude-statusline/internal/render"
	"github.com/felipeelias/claude-statusline/internal/width"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unknownWidth is passed wherever a test wants the "no dropping" behaviour
// (D5): unset/empty COLUMNS.
const unknownWidth = ""

// Fixture formats shared across many selection and separator tests.
const (
	formatModelCost        = "$model$cost"
	formatModelCostContext = "$model | $cost / $context"
)

// --- test helpers -----------------------------------------------------

// fakeModule is an injectable modules.Module of exact known text (and
// therefore exact known width), used through the render.ModuleFactory seam
// to make the greedy selection algorithm (D4) deterministic. The real
// modules are unsuitable for this: git_branch shells out to git and
// directory reads the real $HOME (fixture hygiene).
type fakeModule struct {
	text  string
	err   error
	calls *int
}

func (f fakeModule) Name() string { return "fake" }

func (f fakeModule) Render(input.Data, config.Config) (string, error) {
	if f.calls != nil {
		*f.calls++
	}

	return f.text, f.err
}

// useModules overrides render.ModuleFactory for the duration of the test,
// restoring the original on cleanup. Only the given names are registered;
// any $ref in the format string that isn't among them is "unknown" (D7).
func useModules(t *testing.T, byName map[string]modules.Module) {
	t.Helper()

	original := render.ModuleFactory
	render.ModuleFactory = func(config.Config) map[string]render.ModuleEntry {
		registry := make(map[string]render.ModuleEntry, len(byName))
		for name, mod := range byName {
			registry[name] = render.ModuleEntry{Module: mod}
		}

		return registry
	}

	t.Cleanup(func() { render.ModuleFactory = original })
}

// oracleData is the canonical input JSON from the plan's pre-derived oracle
// appendix: {"model":{"display_name":"Opus"},"cwd":"/tmp/test",
// "cost":{"total_cost_usd":0.42},"context_window":{"used_percentage":42.5}}.
// Segment bytes measured against it: $model -> "\033[1mOpus\033[0m" (4
// visible), $cost -> "\033[32m$0.42\033[0m" (5), $context ->
// "\033[32m██░░░ 42%\033[0m" (9). The plan's appendix transcribes the
// context bar's empty character as "▒" (U+2592, the "blocks" bar style);
// verified directly against modules.ContextModule.Render (untouched by T4,
// so this is the "render on the pre-change binary" method, not derivation
// from the code under test) that config.Default() leaves BarStyle unset,
// which resolves to "classic": empty is "░" (U+2591), not "▒".
func oracleData() input.Data {
	return input.Data{
		Model:         input.Model{DisplayName: "Opus"},
		Cost:          input.Cost{TotalCostUSD: 0.42},
		ContextWindow: input.ContextWindow{UsedPercentage: 42.5},
	}
}

const (
	oracleModel   = "\033[1mOpus\033[0m"
	oracleCost    = "\033[32m$0.42\033[0m"
	oracleContext = "\033[32m██░░░ 42%\033[0m"
)

// --- pre-existing tests (arity bump only: Render gained a columns param;
// "" preserves today's unconditional-render behaviour byte-for-byte) ------

func TestRenderPlain(t *testing.T) {
	cfg := config.Default()
	data := input.Data{
		Model:         input.Model{DisplayName: "Claude Opus 4"},
		Cwd:           "/tmp/test",
		Cost:          input.Cost{TotalCostUSD: 0.42},
		ContextWindow: input.ContextWindow{UsedPercentage: 42.5},
	}
	result, err := render.Render(cfg, data, unknownWidth)
	require.NoError(t, err)
	assert.Contains(t, result, "Claude Opus 4")
	assert.Contains(t, result, "/tmp/test")
	assert.Contains(t, result, "$0.42")
	assert.Contains(t, result, "42%")
	assert.Contains(t, result, " | ")
}

func TestRenderDisabledModule(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "$model | $session_timer | $cost"
	data := input.Data{
		Model: input.Model{DisplayName: "Opus"},
		Cost:  input.Cost{TotalCostUSD: 1.0},
	}
	result, err := render.Render(cfg, data, unknownWidth)
	require.NoError(t, err)
	assert.Contains(t, result, "Opus")
	assert.Contains(t, result, "$1.00")
}

func TestRenderStyledText(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "[hello](bold green)"
	result, err := render.Render(cfg, input.Data{}, unknownWidth)
	require.NoError(t, err)
	assert.Contains(t, result, "\033[1;32m")
	assert.Contains(t, result, "hello")
}

func TestRenderUnknownModule(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "$unknown_module"
	result, err := render.Render(cfg, input.Data{}, unknownWidth)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestRenderPowerline(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "[](bg:blue)$model[](fg:blue bg:cyan)$directory[](fg:cyan)"
	data := input.Data{
		Model: input.Model{DisplayName: "Opus"},
		Cwd:   "/tmp",
	}
	result, err := render.Render(cfg, data, unknownWidth)
	require.NoError(t, err)
	assert.Contains(t, result, "Opus")
	assert.Contains(t, result, "/tmp")
	assert.Contains(t, result, "\033[") // ANSI codes present
}

func TestRenderLiteralText(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "<<< $model >>>"
	data := input.Data{
		Model: input.Model{DisplayName: "Opus"},
	}
	result, err := render.Render(cfg, data, unknownWidth)
	require.NoError(t, err)
	assert.Contains(t, result, "<<<")
	assert.Contains(t, result, ">>>")
	assert.Contains(t, result, "Opus")
}

func TestRenderEmptyFormat(t *testing.T) {
	cfg := config.Default()
	cfg.Format = ""
	result, err := render.Render(cfg, input.Data{}, unknownWidth)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestRenderInlineStyle(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "[text](cyan)"
	result, err := render.Render(cfg, input.Data{}, unknownWidth)
	require.NoError(t, err)
	assert.Contains(t, result, "\033[36m")
	assert.Contains(t, result, "text")
}

// --- backward compatibility (tests 1-6) --------------------------------

// Test 1: no priorities + narrow COLUMNS => byte-identical to today. The
// fixture deliberately contains a disabled module (usage, disabled by
// default) and an unknown $ref, so a defect that treats either as "not a
// survivor" would be caught (D7).
func TestRenderNoPrioritiesNarrowColumnsByteIdentical(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "$model | $usage | $unknown_module | $cost"
	data := input.Data{
		Model: input.Model{DisplayName: "Claude Opus 4"},
		Cost:  input.Cost{TotalCostUSD: 0.42},
	}

	result, err := render.Render(cfg, data, "1") // absurdly narrow
	require.NoError(t, err)

	modelR := "\033[1mClaude Opus 4\033[0m"
	costR := "\033[32m$0.42\033[0m"
	expected := modelR + " | " + " | " + " | " + costR

	assert.Equal(t, expected, result)
}

// D7 acceptance value: default preset, no priorities, a Cwd that is not a
// git repo, wide width => byte-identical to today, including the doubled
// separator around the empty $git_branch segment. git_branch renders ""
// outside a repo (gitbranch.go), but corrected D7 says it is still a
// present segment: both of its adjacent separators must survive.
//
// CP2 follow-up: the original version of this test asserted
// Contains(" |  | "), which is killed by zero of 19 mutants -- it would
// also pass if separators were never collapsed ANYWHERE, proving nothing
// about git_branch specifically. This version is byte-exact, built from the
// real, untouched module Renderers (directory, git_branch, model, cost,
// context -- none of which T4/T4b touch), not derived from render.go's own
// logic.
func TestRenderDefaultPresetGitBranchEmptyKeepsDoubledSeparator(t *testing.T) {
	cfg := config.Default()

	// t.TempDir() guarantees a fresh directory, not a non-repo one -- git
	// walks upward through ancestors. Holds on a normal machine (TMPDIR
	// unset, no git ancestor above /tmp); if TMPDIR ever sat inside a repo,
	// this fails loudly via require.Empty(branchR) below, not silently.
	data := input.Data{
		Model:         input.Model{DisplayName: "Claude Opus 4"},
		Cwd:           t.TempDir(),
		Cost:          input.Cost{TotalCostUSD: 0.42},
		ContextWindow: input.ContextWindow{UsedPercentage: 42.5},
	}

	result, err := render.Render(cfg, data, "200") // wide: nothing dropped anyway (no priorities)
	require.NoError(t, err)

	dirR, err := modules.NewDirectoryModule().Render(data, cfg)
	require.NoError(t, err)

	branchR, err := modules.GitBranchModule{}.Render(data, cfg)
	require.NoError(t, err)
	require.Empty(t, branchR, "fixture must exercise git_branch's empty-render path")

	modelR, err := modules.ModelModule{}.Render(data, cfg)
	require.NoError(t, err)

	costR, err := modules.CostModule{}.Render(data, cfg)
	require.NoError(t, err)

	contextR, err := modules.ContextModule{}.Render(data, cfg)
	require.NoError(t, err)

	expected := dirR + " | " + branchR + " | " + modelR + " | " + costR + " | " + contextR
	assert.Equal(t, expected, result)
}

// Test 2: COLUMNS unset/""/"abc"/"80x24"/"0"/"-5" => everything rendered.
func TestRenderUnknownColumnsVariants(t *testing.T) {
	cfg := config.Default()
	cfg.Format = formatModelCost
	cfg.Cost.Priority = new(1)

	useModules(t, map[string]modules.Module{
		"model": fakeModule{text: strings.Repeat("m", 10)},
		"cost":  fakeModule{text: strings.Repeat("c", 100)},
	})

	expected := strings.Repeat("m", 10) + strings.Repeat("c", 100)

	for _, columns := range []string{"", "abc", "80x24", "0", "-5"} {
		t.Run("columns="+columns, func(t *testing.T) {
			result, err := render.Render(cfg, input.Data{}, columns)
			require.NoError(t, err)
			assert.Equal(t, expected, result)
		})
	}
}

// Test 3: " 80 " => 80.
func TestRenderColumnsTrimsWhitespace(t *testing.T) {
	cfg := config.Default()
	cfg.Format = formatModelCost
	cfg.Cost.Priority = new(1)

	useModules(t, map[string]modules.Module{
		"model": fakeModule{text: strings.Repeat("m", 10)},
		"cost":  fakeModule{text: strings.Repeat("c", 100)},
	})

	bare, err := render.Render(cfg, input.Data{}, "80")
	require.NoError(t, err)

	padded, err := render.Render(cfg, input.Data{}, " 80 ")
	require.NoError(t, err)

	assert.Equal(t, bare, padded)
	assert.Equal(t, strings.Repeat("m", 10), bare, "cost must be dropped at columns=80")
}

// Test 4: all 6 presets x {wide, narrow, unknown} x no priorities =>
// byte-identical (self-consistency: no preset ships any priority, D6).
func TestRenderPresetsByteIdenticalAcrossWidths(t *testing.T) {
	data := input.Data{
		Model:         input.Model{DisplayName: "Claude Opus 4"},
		Cwd:           "/tmp/test",
		Cost:          input.Cost{TotalCostUSD: 0.42},
		ContextWindow: input.ContextWindow{UsedPercentage: 42.5},
	}

	for _, name := range config.PresetNames() {
		t.Run(name, func(t *testing.T) {
			cfg, ok := config.ApplyPreset(name)
			require.True(t, ok)

			unknown, err := render.Render(cfg, data, unknownWidth)
			require.NoError(t, err)

			narrow, err := render.Render(cfg, data, "5")
			require.NoError(t, err)

			wide, err := render.Render(cfg, data, "200")
			require.NoError(t, err)

			assert.Equal(t, unknown, narrow)
			assert.Equal(t, unknown, wide)
		})
	}
}

// Test 5: wide + a ranked module rendering empty => byte-identical, doubled
// separator preserved (D7: survivor != non-empty).
func TestRenderRankedEmptyModuleWideKeepsDoubledSeparator(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "$model | $version | $cost"
	cfg.Version.Priority = new(50)

	useModules(t, map[string]modules.Module{
		"model":   modules.ModelModule{},
		"cost":    modules.CostModule{},
		"version": fakeModule{text: ""},
	})

	result, err := render.Render(cfg, oracleData(), "200")
	require.NoError(t, err)

	expected := oracleModel + " | " + " | " + oracleCost
	assert.Equal(t, expected, result)
}

// New (plan reconciled to code): an early draft of the plan called an
// enabled module that renders "" "not rankable", but render.go derives
// rankability from configured priority alone -- a ranked empty module IS a
// genuine candidate and CAN be dropped at narrow width, recovering its
// separators' columns. The code's behaviour is correct; this pins it so it
// cannot silently flip.
func TestRenderRankedEmptyModuleCanBeDroppedToRecoverSeparatorWidth(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "$model | $version | $cost"
	cfg.Version.Priority = new(1)

	useModules(t, map[string]modules.Module{
		"model":   fakeModule{text: strings.Repeat("m", 10)},
		"version": fakeModule{text: ""},
		"cost":    fakeModule{text: strings.Repeat("c", 5)},
	})

	result, err := render.Render(cfg, input.Data{}, "19")
	require.NoError(t, err)

	// Keeping version (empty, but ranked) would cost 21: both its
	// separators render around its empty content (10+3+0+3+5). Dropping it
	// merges the two gaps into one via the LAST-run rule, costing 18 <= 19.
	assert.Equal(t, strings.Repeat("m", 10)+" | "+strings.Repeat("c", 5), result)
}

// Test 6: literal braces render exactly as today, regardless of ranking --
// braces are ordinary literals, never grammar (D2).
func TestRenderLiteralBracesUnaffected(t *testing.T) {
	tests := map[string]struct {
		format   string
		expected string
	}{
		"dollar-brace":  {"{$model}", "{" + oracleModel + "}"},
		"go-template":   {"{{.Dir}}", "{{.Dir}}"},
		"styled-braces": {"[{](dim)$model[}](dim)", "\033[2m{\033[0m" + oracleModel + "\033[2m}\033[0m"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Format = test.format

			result, err := render.Render(cfg, oracleData(), unknownWidth)
			require.NoError(t, err)
			assert.Equal(t, test.expected, result)
		})
	}
}

// --- literal-run maximality (CP2 BLOCKING 1) ----------------------------
// Untested at T4, and the break is severe: a mutation that flushes the
// literal buffer after every styled span (instead of only before the next
// module ref) leaves the entire 32-test suite green, yet silently discards
// every separator built from consecutive styled spans, or the
// "text [styled] text" idiom -- the normal way a developer writes a styled
// separator in their own config.

// (a) consecutive styled spans with no module ref between them are one
// maximal run; both spans must survive.
func TestRenderConsecutiveStyledSpansFormOneMaximalRun(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "$model[A](dim)[B](dim)$cost"

	useModules(t, map[string]modules.Module{
		"model": fakeModule{text: "M"},
		"cost":  fakeModule{text: "C"},
	})

	result, err := render.Render(cfg, input.Data{}, unknownWidth)
	require.NoError(t, err)
	assert.Equal(t, "M\033[2mA\033[0m\033[2mB\033[0mC", result)
}

// (b) the "text [styled] text" separator idiom -- plain text around a
// styled span, with no module ref between them -- is also one maximal run;
// the whole thing survives or collapses as a single unit.
func TestRenderStyledSeparatorIdiomSurvivesAsOneRun(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "$model [·](dim) $cost"

	useModules(t, map[string]modules.Module{
		"model": fakeModule{text: "M"},
		"cost":  fakeModule{text: "C"},
	})

	result, err := render.Render(cfg, input.Data{}, unknownWidth)
	require.NoError(t, err)
	assert.Equal(t, "M \033[2m·\033[0m C", result)
}

// --- selection (tests 7-16) --------------------------------------------

// Tests 7-9: cumulative-width bookkeeping across the greedy loop.
func TestRenderSelectionCumulativeWidth(t *testing.T) {
	tests := map[string]struct {
		widths   map[string]int // model, cost, context, version
		priority map[string]int // cost, context, version (model always mandatory)
		usable   string
		survives map[string]bool
	}{
		// A(30,w10) B(20,w50) C(10,w5), usable 30 => keep M,A,C. Kills
		// cumulative dropping (a buggy impl that keeps B's width in the
		// running total after rejecting it would wrongly reject C too).
		"kills cumulative dropping": {
			widths:   map[string]int{"model": 10, "cost": 10, "context": 50, "version": 5},
			priority: map[string]int{"cost": 30, "context": 20, "version": 10},
			usable:   "30",
			survives: map[string]bool{"model": true, "cost": true, "context": false, "version": true},
		},
		// A(30,w6) B(20,w6), usable 20, mandatory w10 => keep M,A only.
		// Kills isolated-vs-cumulative checking (B alone fits in 20, but
		// M+A+B does not).
		"kills isolated checking": {
			widths:   map[string]int{"model": 10, "cost": 6, "context": 6},
			priority: map[string]int{"cost": 30, "context": 20},
			usable:   "20",
			survives: map[string]bool{"model": true, "cost": true, "context": false},
		},
		// A(40,w50) B(30,w40) C(20,w5), usable 20, mandatory w10 => keep
		// M,C. Kills early-break (an impl that stops trying after the
		// first rejection never reaches C).
		"kills early break": {
			widths:   map[string]int{"model": 10, "cost": 50, "context": 40, "version": 5},
			priority: map[string]int{"cost": 40, "context": 30, "version": 20},
			usable:   "20",
			survives: map[string]bool{"model": true, "cost": false, "context": false, "version": true},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Format = "$model$cost$context$version"

			for mod, pri := range test.priority {
				setPriority(&cfg, mod, pri)
			}

			byName := map[string]modules.Module{}
			for mod, w := range test.widths {
				byName[mod] = fakeModule{text: strings.Repeat("x", w)}
			}

			useModules(t, byName)

			result, err := render.Render(cfg, input.Data{}, test.usable)
			require.NoError(t, err)

			var expected strings.Builder
			for _, mod := range []string{"model", "cost", "context", "version"} {
				if test.survives[mod] {
					expected.WriteString(strings.Repeat("x", test.widths[mod]))
				}
			}

			assert.Equal(t, expected.String(), result)
		})
	}
}

// setPriority sets the priority field on the named module in cfg. Test-only
// helper so the table above can stay data-driven.
func setPriority(cfg *config.Config, name string, pri int) {
	switch name {
	case "model":
		cfg.Model.Priority = new(pri)
	case "cost":
		cfg.Cost.Priority = new(pri)
	case "context":
		cfg.Context.Priority = new(pri)
	case "version":
		cfg.Version.Priority = new(pri)
	case "agent_name":
		cfg.AgentName.Priority = new(pri)
	}
}

// Test 10: mixed ranked/unranked, narrow => unranked survives (even in
// overflow), the ranked one drops. Kills nil-as-0: if an absent priority
// were treated as priority 0 instead of "mandatory", model would compete
// as the LOWEST-priority candidate and lose to cost, inverting D1.
func TestRenderMixedRankedUnrankedKillsNilAsZero(t *testing.T) {
	cfg := config.Default()
	cfg.Format = formatModelCost
	cfg.Cost.Priority = new(100) // model has no priority: mandatory

	useModules(t, map[string]modules.Module{
		"model": fakeModule{text: strings.Repeat("m", 50)},
		"cost":  fakeModule{text: strings.Repeat("c", 5)},
	})

	result, err := render.Render(cfg, input.Data{}, "10")
	require.NoError(t, err)

	// Correct: model (mandatory) always present, even in overflow (50 > 10);
	// cost (ranked) rejected because 50+5=55 > 10.
	// Buggy nil-as-0: model competes at fake priority 0, tried after cost
	// (100); candidate={cost} fits (5<=10), then model is rejected
	// (5+50=55>10) -- the inverse of correct behaviour.
	assert.Equal(t, strings.Repeat("m", 50), result)
}

// Test 11: priorities inverted vs format position, wide => output in
// FORMAT order regardless of trial order.
func TestRenderOutputFormatOrderNotPriorityOrder(t *testing.T) {
	cfg := config.Default()
	cfg.Format = formatModelCost
	cfg.Model.Priority = new(1)
	cfg.Cost.Priority = new(99)

	useModules(t, map[string]modules.Module{
		"model": fakeModule{text: "MMM"},
		"cost":  fakeModule{text: "CCC"},
	})

	result, err := render.Render(cfg, input.Data{}, "100")
	require.NoError(t, err)
	assert.Equal(t, "MMMCCC", result)
}

// Test 12: total == usable => no drop; total == usable+1 => exactly one
// drop.
func TestRenderExactFitBoundary(t *testing.T) {
	cfg := config.Default()
	cfg.Format = formatModelCost
	cfg.Cost.Priority = new(5)

	useModules(t, map[string]modules.Module{
		"model": fakeModule{text: strings.Repeat("m", 10)},
		"cost":  fakeModule{text: strings.Repeat("c", 8)},
	})

	exact, err := render.Render(cfg, input.Data{}, "18")
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("m", 10)+strings.Repeat("c", 8), exact, "total==usable: no drop")

	oneOver, err := render.Render(cfg, input.Data{}, "17")
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("m", 10), oneOver, "total==usable+1: exactly one drop")
}

// Test 13: three-way tie => format order.
//
// CP2 correction: this fixture does not empirically discriminate
// sort.Slice from sort.SliceStable on the current toolchain -- verified by
// mutation (swapping in sort.Slice here still passes this test, and every
// other test in the suite). A fully-tied comparator reports every pair as
// "not less", so Go's current sort.Slice implementation does no reordering
// work at all and happens to preserve input order regardless of size (also
// verified up to 50 tied elements). A larger, interspersed
// tied/non-tied pattern CAN be made to diverge (proven separately), but
// that construction depends on undocumented pdqsort partitioning behaviour
// that Go's sort.Slice contract explicitly does not guarantee, and could
// silently stop discriminating on a future Go release. sort.SliceStable is
// used here because determinism is a correctness requirement BY CONTRACT
// (D4/D10 need reproducible format-order output for ties), not because
// this specific fixture can currently catch a regression to sort.Slice.
func TestRenderThreeWayTieFormatOrder(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "$model$cost$context"
	cfg.Model.Priority = new(50)
	cfg.Cost.Priority = new(50)
	cfg.Context.Priority = new(50)

	useModules(t, map[string]modules.Module{
		"model":   fakeModule{text: strings.Repeat("x", 10)},
		"cost":    fakeModule{text: strings.Repeat("x", 10)},
		"context": fakeModule{text: strings.Repeat("x", 10)},
	})

	result, err := render.Render(cfg, input.Data{}, "20")
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("x", 20), result, "model and cost (format order) survive, context drops")
}

// Test 14: sweep COLUMNS 1->120 => Visible(output) <= usable, or only the
// mandatory baseline remains. The fixture includes a leading decoration
// run, a powerline glyph, and a CJK rune, per the plan's fixture-hygiene
// warning that a narrower fixture cannot catch either a separator
// accounting error or len()-instead-of-Visible.
func TestRenderColumnsSweepNeverExceedsUsable(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "[](dim) $model $cost $context"
	cfg.Cost.Priority = new(100)   // contains a CJK rune
	cfg.Context.Priority = new(50) // wide ascii run

	useModules(t, map[string]modules.Module{
		"model":   fakeModule{text: "M"},
		"cost":    fakeModule{text: "中中"}, //nolint:gosmopolitan // deliberate CJK fixture, width 4
		"context": fakeModule{text: strings.Repeat("B", 10)},
	})

	baseline, err := render.Render(cfg, input.Data{}, "1")
	require.NoError(t, err)

	for columns := 1; columns <= 120; columns++ {
		usable := columns

		output, err := render.Render(cfg, input.Data{}, strconv.Itoa(columns))
		require.NoError(t, err)

		if width.Visible(output) > usable {
			assert.Equal(t, baseline, output, "columns=%d: only the mandatory baseline may exceed usable", columns)
		}
	}
}

// Test 14 (complementary): the sweep above only catches OVER-counting
// (which causes overflow); it is blind to UNDER-counting (e.g.
// len()-instead-of-Visible on a multi-byte rune), because under-counting
// only causes unnecessary under-filling, never overflow, so the sweep's
// "Visible(output) <= usable" assertion holds regardless. This test pins
// one admission decision directly: a CJK candidate is genuinely 4 visible
// columns but 6 bytes -- usable=5 admits it only if measured in display
// columns, not bytes.
func TestRenderColumnsSweepCatchesUndercounting(t *testing.T) {
	cfg := config.Default()
	cfg.Format = formatModelCost
	cfg.Cost.Priority = new(1)

	useModules(t, map[string]modules.Module{
		"model": fakeModule{text: "M"},
		"cost":  fakeModule{text: "中中"}, //nolint:gosmopolitan // deliberate CJK fixture, 4 cols/6 bytes
	})

	result, err := render.Render(cfg, input.Data{}, "5")
	require.NoError(t, err)
	//nolint:gosmopolitan // deliberate CJK fixture, matches the injected fake module's text
	assert.Equal(t, "M中中", result, "1+4=5<=5 in display columns, even though 1+6=7>5 in bytes")
}

// Test 15: mandatory alone > usable => emitted, not truncated, not blanked.
func TestRenderMandatoryOverflowNeverTruncated(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "$model"

	useModules(t, map[string]modules.Module{
		"model": fakeModule{text: strings.Repeat("m", 50)},
	})

	result, err := render.Render(cfg, input.Data{}, "5")
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("m", 50), result)
}

// Test 16: usable <= 0 (via a margin that consumes all of COLUMNS) =>
// unknown => everything rendered.
func TestRenderMarginClampsUsableToUnknown(t *testing.T) {
	tests := map[string]struct {
		columns string
		margin  int
	}{
		"usable < 0":  {columns: "10", margin: 20}, // usable = 10-20 = -10
		"usable == 0": {columns: "20", margin: 20}, // usable = 20-20 = 0 (the actual boundary)
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Format = formatModelCost
			cfg.Cost.Priority = new(1)
			cfg.Fit.Margin = test.margin

			useModules(t, map[string]modules.Module{
				"model": fakeModule{text: strings.Repeat("m", 10)},
				"cost":  fakeModule{text: strings.Repeat("c", 50)},
			})

			result, err := render.Render(cfg, input.Data{}, test.columns)
			require.NoError(t, err)
			assert.Equal(t, strings.Repeat("m", 10)+strings.Repeat("c", 50), result)
		})
	}
}

// --- separator inference (tests 17-24, 26-27): the core of the design ---
// Fixture and expected byte strings for 17-21/24 are taken verbatim from
// the plan's pre-derived oracle appendix (rendered on the pre-change
// binary), never computed from this implementation.

// Test 24: nothing dropped => every gap separator renders verbatim.
func TestRenderSeparatorNothingDropped(t *testing.T) {
	cfg := config.Default()
	cfg.Format = formatModelCostContext

	result, err := render.Render(cfg, oracleData(), unknownWidth)
	require.NoError(t, err)
	assert.Equal(t, oracleModel+" | "+oracleCost+" / "+oracleContext, result)
}

// Test 17: middle module dropped => gap collapse, keeps the surviving gap
// verbatim ("a / c", not "a  c" or "a | c").
func TestRenderSeparatorMiddleDroppedCollapses(t *testing.T) {
	cfg := config.Default()
	cfg.Format = formatModelCostContext
	cfg.Cost.Priority = new(1) // only cost is ranked; rejected at columns=20

	result, err := render.Render(cfg, oracleData(), "20")
	require.NoError(t, err)
	assert.Equal(t, oracleModel+" / "+oracleContext, result)
}

// Test 18: first module dropped => its following gap separator disappears,
// no leading orphan.
func TestRenderSeparatorFirstDroppedNoLeadingOrphan(t *testing.T) {
	cfg := config.Default()
	cfg.Format = formatModelCostContext
	cfg.Model.Priority = new(1)

	result, err := render.Render(cfg, oracleData(), "20")
	require.NoError(t, err)
	assert.Equal(t, oracleCost+" / "+oracleContext, result)
}

// Test 19: last module dropped => its preceding gap separator disappears.
func TestRenderSeparatorLastDroppedNoTrailingOrphan(t *testing.T) {
	cfg := config.Default()
	cfg.Format = formatModelCostContext
	cfg.Context.Priority = new(1)

	result, err := render.Render(cfg, oracleData(), "20")
	require.NoError(t, err)
	assert.Equal(t, oracleModel+" | "+oracleCost, result)
}

// Test 20: leading decoration + first module dropped => decoration
// survives verbatim, no separator invented between it and the new first
// survivor.
func TestRenderSeparatorLeadingDecoration(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "[>>](dim)$model | $cost"
	cfg.Model.Priority = new(1)

	decoration := "\033[2m>>\033[0m"

	nothingDropped, err := render.Render(cfg, oracleData(), unknownWidth)
	require.NoError(t, err)
	assert.Equal(t, decoration+oracleModel+" | "+oracleCost, nothingDropped)

	modelDropped, err := render.Render(cfg, oracleData(), "10")
	require.NoError(t, err)
	assert.Equal(t, decoration+oracleCost, modelDropped)
}

// Test 21: two consecutive modules dropped => still exactly one separator
// between the surviving neighbours (here, zero neighbours survive on one
// side, so no separator at all -- "one survivor, no separator").
func TestRenderSeparatorTwoConsecutiveDropped(t *testing.T) {
	cfg := config.Default()
	cfg.Format = formatModelCostContext
	cfg.Model.Priority = new(2)
	cfg.Cost.Priority = new(1)

	result, err := render.Render(cfg, oracleData(), "10")
	require.NoError(t, err)
	assert.Equal(t, oracleContext, result)
}

// Test 22: a multi-run gap, produced dynamically by dropping two interior
// modules, collapses to the LAST run. Uses the seam (not $directory, per
// fixture hygiene) for full determinism.
func TestRenderSeparatorMultiRunGapKeepsLast(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "$model | $cost / $context ; $version"
	cfg.Cost.Priority = new(2)
	cfg.Context.Priority = new(1)

	useModules(t, map[string]modules.Module{
		"model":   fakeModule{text: "F"},
		"cost":    fakeModule{text: strings.Repeat("x", 50)},
		"context": fakeModule{text: strings.Repeat("x", 50)},
		"version": fakeModule{text: "L"},
	})

	result, err := render.Render(cfg, input.Data{}, "5")
	require.NoError(t, err)
	assert.Equal(t, "F ; L", result, "only the LAST run of the 3-run gap (\" ; \") survives")
}

// Test 22 (powerline variant): the surviving separator's background must
// match the following surviving block, per D2's LAST rule.
func TestRenderSeparatorMultiRunGapPowerlineBackground(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "$model[  ](bg:red)$cost[  ](bg:green)$context[  ](bg:blue)$version"
	cfg.Cost.Priority = new(2)
	cfg.Context.Priority = new(1)

	useModules(t, map[string]modules.Module{
		"model":   fakeModule{text: "F"},
		"cost":    fakeModule{text: strings.Repeat("x", 50)},
		"context": fakeModule{text: strings.Repeat("x", 50)},
		"version": fakeModule{text: "L"},
	})

	result, err := render.Render(cfg, input.Data{}, "5")
	require.NoError(t, err)

	assert.NotContains(t, result, "41m") // red bg
	assert.NotContains(t, result, "42m") // green bg
	assert.Contains(t, result, "44m")    // blue bg: matches the following block ($version)
}

// Test 23: adjacent refs with no literal between => no separator invented.
func TestRenderSeparatorAdjacentRefsNoneInvented(t *testing.T) {
	cfg := config.Default()
	cfg.Format = formatModelCost

	result, err := render.Render(cfg, oracleData(), unknownWidth)
	require.NoError(t, err)
	assert.Equal(t, oracleModel+oracleCost, result)
}

// Test 23b: duplicate refs (D10) are independent segments, measured
// separately; when only one fits, the first in format order survives. This
// also pins that selection iterates per segment, not per module name -- a
// name-keyed loop would admit or reject both occurrences together.
func TestRenderDuplicateRefsIndependentSegments(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "$model $model"
	cfg.Model.Priority = new(1)

	useModules(t, map[string]modules.Module{
		"model": fakeModule{text: strings.Repeat("m", 10)},
	})

	result, err := render.Render(cfg, input.Data{}, "10")
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("m", 10), result, "only the first occurrence survives")
}

// Test 23c: zero survivors (every module ranked, tiny width) => decoration
// alone renders.
func TestRenderZeroSurvivorsDecorationAlone(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "[>>](dim)$model | $cost"
	cfg.Model.Priority = new(2)
	cfg.Cost.Priority = new(1)

	useModules(t, map[string]modules.Module{
		"model": fakeModule{text: strings.Repeat("m", 50)},
		"cost":  fakeModule{text: strings.Repeat("c", 50)},
	})

	result, err := render.Render(cfg, input.Data{}, "1")
	require.NoError(t, err)
	assert.Equal(t, "\033[2m>>\033[0m", result)
}

// Test 26: the measure-vs-emit oracle. Measuring the separator against a
// candidate that doesn't include $model is the bug -- tests 7-9 are blind
// to it (mandatory M always survives so gaps always render), test 14 is
// blind to it (this failure under-fills rather than overflows).
func TestRenderMeasureVsEmitOracle(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "$model | $context"
	cfg.Model.Priority = new(10)
	cfg.Context.Priority = new(90)

	result, err := render.Render(cfg, oracleData(), "10")
	require.NoError(t, err)
	assert.Equal(t, oracleContext, result, "context alone (9<=10), not model (16>10 with model+sep+context)")
}

// Test 27: a module that fits only because its gap separator collapsed
// must be admitted. Pinned for the LAST-run rule: format $M R1 $b R2 $c,
// R1=3 cols, R2=10 cols, M mandatory w10, b pri20 w50 (rejected), c pri10
// w5; usable=28. b is rejected (10+3+50=63>28); c is then admitted because
// gap(M,c) collapses to the LAST run R2=10 => 10+10+5=25<=28. The surviving
// separator must be R2, not R1.
func TestRenderGapCollapseAdmitsFollowingModule(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "$model - $cost----------$context"
	cfg.Cost.Priority = new(20)
	cfg.Context.Priority = new(10)

	useModules(t, map[string]modules.Module{
		"model":   fakeModule{text: strings.Repeat("m", 10)},
		"cost":    fakeModule{text: strings.Repeat("b", 50)},
		"context": fakeModule{text: strings.Repeat("c", 5)},
	})

	result, err := render.Render(cfg, input.Data{}, "28")
	require.NoError(t, err)

	expected := strings.Repeat("m", 10) + "----------" + strings.Repeat("c", 5)
	assert.Equal(t, expected, result, "surviving separator must be R2 (the LAST run), not R1 (\" - \")")
}

// --- modules / errors (tests 25, 28, 29, 31, 32) ------------------------

// Test 25: template error in a droppable (ranked) module still fails the
// whole render (D8) -- every enabled module is rendered up front,
// regardless of whether selection will keep it.
func TestRenderDroppableModuleErrorFailsRender(t *testing.T) {
	cfg := config.Default()
	cfg.Format = formatModelCost
	cfg.Cost.Priority = new(1) // droppable

	boom := assert.AnError

	useModules(t, map[string]modules.Module{
		"model": fakeModule{text: "m"},
		"cost":  fakeModule{err: boom},
	})

	_, err := render.Render(cfg, input.Data{}, "1") // width tiny enough that cost would be rejected anyway
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

// Test 28: ranked + disabled, narrow => excluded from candidacy AND its
// separators are preserved, not collapsed. Setting a priority on one module
// must never change how an unrelated disabled module renders.
//
// CP2 BLOCKING 2: the original version of this test ran at unknownWidth,
// where selectSurvivors returns before the selection loop runs at all, so
// it could never observe candidacy -- proven vacuous: a mutation that lets
// a disabled+ranked module become a droppable candidate still passed it.
// This version forces a real selection decision by using a narrow width
// with a genuine ranked competitor (cost), so session_timer's exclusion
// from candidacy is actually exercised: session_timer's own priority (100,
// the highest in the fixture) would, if it were ever treated as a real
// candidate, be tried first and could be rejected by width like any other
// -- but being disabled, it must always survive regardless.
func TestRenderDisabledExcludedFromCandidacyAtNarrowWidth(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "$model | $session_timer | $cost"
	cfg.SessionTimer.Priority = new(100) // highest priority, tried first if ever a candidate
	cfg.Cost.Priority = new(1)

	original := render.ModuleFactory
	render.ModuleFactory = func(config.Config) map[string]render.ModuleEntry {
		return map[string]render.ModuleEntry{
			"model":         {Module: fakeModule{text: strings.Repeat("M", 10)}},
			"session_timer": {Module: fakeModule{text: "SHOULD-NEVER-RENDER"}, Disabled: true},
			"cost":          {Module: fakeModule{text: strings.Repeat("C", 5)}},
		}
	}
	t.Cleanup(func() { render.ModuleFactory = original })

	result, err := render.Render(cfg, input.Data{}, "12")
	require.NoError(t, err)

	// session_timer (disabled) always survives and anchors its preceding
	// separator, regardless of its own (highest) priority and regardless
	// of width. cost is genuinely too wide to fit alongside it (10+3+5=18
	// > 12) and is correctly dropped -- but that is unrelated to session_timer,
	// which must never be subject to a width check at all.
	assert.Equal(t, strings.Repeat("M", 10)+" | ", result)
}

// Test 29: broken template behind disabled=true => no error (D7: never
// rendered, so never a source of errors).
func TestRenderDisabledModuleBrokenTemplateNoError(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "$model | $session_timer"
	cfg.SessionTimer.Format = "{{.NoSuchField}}" // would error if ever executed

	result, err := render.Render(cfg, oracleData(), unknownWidth)
	require.NoError(t, err)
	assert.Equal(t, oracleModel+" | ", result)
}

// Test 31: each module's Render is invoked exactly once across a full
// selection sweep (D4 cache) -- compose runs many times during greedy
// selection, but must never re-render.
func TestRenderModulesInvokedExactlyOnce(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "$model$cost$context$version"
	cfg.Cost.Priority = new(30)
	cfg.Context.Priority = new(20)
	cfg.Version.Priority = new(10)

	var modelCalls, costCalls, contextCalls, versionCalls int

	useModules(t, map[string]modules.Module{
		"model":   fakeModule{text: strings.Repeat("x", 10), calls: &modelCalls},
		"cost":    fakeModule{text: strings.Repeat("x", 10), calls: &costCalls},
		"context": fakeModule{text: strings.Repeat("x", 50), calls: &contextCalls},
		"version": fakeModule{text: strings.Repeat("x", 5), calls: &versionCalls},
	})

	_, err := render.Render(cfg, input.Data{}, "30")
	require.NoError(t, err)

	assert.Equal(t, 1, modelCalls)
	assert.Equal(t, 1, costCalls)
	assert.Equal(t, 1, contextCalls)
	assert.Equal(t, 1, versionCalls)
}

// Test 32: $unknown_module still renders empty and anchors its separators
// as a present segment (corrected D7).
func TestRenderUnknownModuleAnchorsSeparators(t *testing.T) {
	cfg := config.Default()
	cfg.Format = "$model | $unknown_module | $cost"

	result, err := render.Render(cfg, oracleData(), unknownWidth)
	require.NoError(t, err)
	assert.Equal(t, oracleModel+" | "+" | "+oracleCost, result)
}
