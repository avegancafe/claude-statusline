package render

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/avegancafe/claude-statusline/internal/config"
	"github.com/avegancafe/claude-statusline/internal/input"
	"github.com/avegancafe/claude-statusline/internal/modules"
	"github.com/avegancafe/claude-statusline/internal/style"
	"github.com/avegancafe/claude-statusline/internal/width"
)

// ModuleEntry pairs a module with whether it is disabled in config. Exported
// so tests can construct entries directly when overriding ModuleFactory.
type ModuleEntry struct {
	Module   modules.Module
	Disabled bool
}

// ModuleFactory builds the module registry, keyed by module name ($name
// references in the format string resolve through this map). Tests
// substitute this var to inject modules of exact known width, force render
// errors, disable a module, or count Render invocations: exercising the
// greedy selection algorithm (D4) deterministically is not possible against
// the real modules, since git_branch shells out to git and directory reads
// the real $HOME. Restore the original value when the test finishes.
var ModuleFactory = defaultModuleFactory

// defaultModuleFactory builds the production module registry from config.
func defaultModuleFactory(cfg config.Config) map[string]ModuleEntry {
	return map[string]ModuleEntry{
		"model":         {Module: modules.ModelModule{}, Disabled: cfg.Model.Disabled},
		"directory":     {Module: modules.NewDirectoryModule(), Disabled: cfg.Directory.Disabled},
		"cost":          {Module: modules.CostModule{}, Disabled: cfg.Cost.Disabled},
		"context":       {Module: modules.ContextModule{}, Disabled: cfg.Context.Disabled},
		"git_branch":    {Module: modules.GitBranchModule{}, Disabled: cfg.GitBranch.Disabled},
		"session_timer": {Module: modules.SessionTimerModule{}, Disabled: cfg.SessionTimer.Disabled},
		"lines_changed": {Module: modules.LinesChangedModule{}, Disabled: cfg.LinesChanged.Disabled},
		"usage":         {Module: modules.UsageModule{}, Disabled: cfg.Usage.Disabled},
		"version":       {Module: modules.VersionModule{}, Disabled: cfg.Version.Disabled},
		"vim_mode":      {Module: modules.VimModeModule{}, Disabled: cfg.VimMode.Disabled},
		"agent_name":    {Module: modules.AgentNameModule{}, Disabled: cfg.AgentName.Disabled},
	}
}

// tokenPattern matches module references ($word) and styled text ([text](style)).
// The order matters: styled text is matched first to avoid $-matching inside it.
// Unchanged from before module priorities existed (D2): no new grammar, no
// braces -- every format that works today keeps working byte-for-byte.
var tokenPattern = regexp.MustCompile(`\[([^\]]*)\]\(([^)]*)\)|\$([a-z_]+)`)

// segmentKind distinguishes a module reference from a literal run in a
// parsed format string.
type segmentKind int

const (
	moduleSegment segmentKind = iota
	literalSegment
)

// segment is one element of a parsed format: either a $name module
// reference or a maximal run of consecutive literal text (plain text and/or
// [text](style) spans, D2).
type segment struct {
	kind    segmentKind
	name    string // module name, for moduleSegment
	literal string // pre-rendered literal text, for literalSegment

	// decoration is only meaningful for literalSegment: true if this run
	// precedes the first module reference or follows the last one. A
	// decoration run always renders, unconditional on selection (D2).
	decoration bool
}

// parseFormat splits format into module and literal-run segments without
// changing tokenPattern or introducing any new grammar (D2). A literal run
// is the MAXIMAL concatenation of consecutive non-module tokens: plain text
// interleaved with any [text](style) spans is rendered once, up front, since
// neither depends on module data. Consecutive styled spans with no module
// reference between them are therefore one run, not several -- a static
// multi-run gap does not exist; multi-run gaps only arise dynamically, when
// selection drops two or more module segments in a row (test 22).
func parseFormat(format string) []segment {
	matches := tokenPattern.FindAllStringSubmatchIndex(format, -1)

	var segments []segment

	var literalBuf strings.Builder

	flushLiteral := func() {
		if literalBuf.Len() > 0 {
			segments = append(segments, segment{kind: literalSegment, literal: literalBuf.String()})
			literalBuf.Reset()
		}
	}

	lastIndex := 0

	for _, loc := range matches {
		if loc[0] > lastIndex {
			literalBuf.WriteString(format[lastIndex:loc[0]])
		}

		switch {
		case loc[2] != -1 && loc[4] != -1: // styled text: [text](style)
			text := format[loc[2]:loc[3]]
			styleStr := format[loc[4]:loc[5]]

			literalBuf.WriteString(style.Parse(styleStr).Wrap(text))
		case loc[6] != -1: // module reference: $name
			flushLiteral()

			segments = append(segments, segment{kind: moduleSegment, name: format[loc[6]:loc[7]]})
		}

		lastIndex = loc[1]
	}

	if lastIndex < len(format) {
		literalBuf.WriteString(format[lastIndex:])
	}

	flushLiteral()
	markDecoration(segments)

	return segments
}

// markDecoration flags each literal-run segment as decoration (D2): a run
// that precedes the first module reference or follows the last one always
// renders, regardless of selection. A format with no module references at
// all is entirely decoration (e.g. a literal-only format).
func markDecoration(segments []segment) {
	first, last := -1, -1

	for index, seg := range segments {
		if seg.kind != moduleSegment {
			continue
		}

		if first == -1 {
			first = index
		}

		last = index
	}

	for index := range segments {
		if segments[index].kind != literalSegment {
			continue
		}

		segments[index].decoration = first == -1 || index < first || index > last
	}
}

// moduleState holds the outcome of rendering one module segment exactly
// once, plus its selection classification.
type moduleState struct {
	// mandatory segments are never dropped: unranked-but-enabled modules
	// (D1), disabled modules, and unknown $refs (corrected D7) are all
	// "present segments" that anchor their adjacent separators exactly as
	// today, regardless of available width.
	mandatory bool
	priority  int // meaningful only when !mandatory
	rendered  string
	err       error
}

// Render parses the format string from cfg, evaluates module references and
// styled text tokens, fits the result to the given terminal width, and
// returns the composed result.
//
// columns is the raw COLUMNS environment variable value, as read by the
// caller -- Render never reads the environment itself, so it stays pure and
// every test is independent of ambient width (D5). Unset, empty,
// non-numeric, or <= 0 means the width is unknown and nothing is ever
// dropped; previews must pass "" for exactly that reason.
func Render(cfg config.Config, data input.Data, columns string) (string, error) {
	format := cfg.Format
	if format == "" {
		return "", nil
	}

	segments := parseFormat(format)
	registry := ModuleFactory(cfg)

	states, err := renderModules(segments, registry, cfg, data)
	if err != nil {
		return "", err
	}

	rendered := renderedText(states)
	survives := selectSurvivors(segments, states, rendered, columns, cfg.Margin())

	return compose(segments, survives, rendered), nil
}

// renderModules resolves each module segment's config classification and
// renders it exactly once (D4): disabled modules and unknown $refs are
// mandatory and render as "" without ever calling Module.Render (the
// disabled guard is preserved verbatim, D7). Every enabled, known module is
// rendered regardless of whether selection will keep it, so a droppable
// module's broken template still fails the render (test 25). The first
// error in format order wins (D8); the error text is unchanged.
func renderModules(
	segments []segment, registry map[string]ModuleEntry, cfg config.Config, data input.Data,
) ([]moduleState, error) {
	states := make([]moduleState, len(segments))

	for index, seg := range segments {
		if seg.kind != moduleSegment {
			continue
		}

		entry, known := registry[seg.name]

		switch {
		case !known:
			states[index] = moduleState{mandatory: true}
		case entry.Disabled:
			states[index] = moduleState{mandatory: true}
		default:
			priority, ranked := cfg.Priority(seg.name)

			rendered, renderErr := entry.Module.Render(data, cfg)
			states[index] = moduleState{mandatory: !ranked, priority: priority, rendered: rendered, err: renderErr}
		}
	}

	for _, st := range states {
		if st.err != nil {
			return nil, st.err
		}
	}

	return states, nil
}

// renderedText extracts each segment's rendered text (empty for literal
// segments, which compose reads from segment.literal instead).
func renderedText(states []moduleState) []string {
	text := make([]string, len(states))
	for i, st := range states {
		text[i] = st.rendered
	}

	return text
}

// candidate is a droppable module segment awaiting a selection trial.
type candidate struct {
	index    int
	priority int
}

// mandatorySurvivors seeds a survives slice with every mandatory module
// segment (D1, D7): unranked-but-enabled modules, disabled modules, and
// unknown $refs are all present segments, never subject to selection.
func mandatorySurvivors(segments []segment, states []moduleState) []bool {
	survives := make([]bool, len(segments))

	for index, seg := range segments {
		if seg.kind == moduleSegment && states[index].mandatory {
			survives[index] = true
		}
	}

	return survives
}

// rankedCandidates collects every droppable module segment, sorted by
// descending priority -- ties broken by format order via a stable sort
// (sort.Slice is unstable and would pass a tie by luck, test 13).
func rankedCandidates(segments []segment, states []moduleState) []candidate {
	var candidates []candidate

	for index, seg := range segments {
		if seg.kind == moduleSegment && !states[index].mandatory {
			candidates = append(candidates, candidate{index: index, priority: states[index].priority})
		}
	}

	sort.SliceStable(candidates, func(a, b int) bool {
		return candidates[a].priority > candidates[b].priority
	})

	return candidates
}

// selectSurvivors runs D4's greedy selection. Mandatory module segments
// always survive. Ranked segments are trialled in descending priority order
// and admitted iff the composed candidate still fits within usable columns.
// When the width is unknown, nothing is ever dropped (D5).
func selectSurvivors(
	segments []segment, states []moduleState, rendered []string, columns string, margin int,
) []bool {
	survives := mandatorySurvivors(segments, states)

	usable, known := usableWidth(columns, margin)
	if !known {
		for index, seg := range segments {
			if seg.kind == moduleSegment {
				survives[index] = true
			}
		}

		return survives
	}

	for _, cand := range rankedCandidates(segments, states) {
		survives[cand.index] = true

		if width.Visible(compose(segments, survives, rendered)) > usable {
			survives[cand.index] = false
		}
	}

	return survives
}

// usableWidth parses the raw COLUMNS value and margin into a usable column
// count (D5, D9). Unset, empty, non-numeric, or <= 0 COLUMNS, or a margin
// that consumes the whole width, all mean "unknown": nothing is dropped.
func usableWidth(columns string, margin int) (int, bool) {
	trimmed := strings.TrimSpace(columns)
	if trimmed == "" {
		return 0, false
	}

	total, err := strconv.Atoi(trimmed)
	if err != nil || total <= 0 {
		return 0, false
	}

	usable := total - margin
	if usable <= 0 {
		return 0, false
	}

	return usable, true
}

// compose walks segments in format order and renders the final output for a
// given survivor set. This is the ONLY function used both to measure a
// selection trial's width (via width.Visible(compose(...))) and to emit the
// final result -- measuring anything other than the bytes that will be
// emitted is the bug class that reintroduces priority inversion while still
// passing every structural test (test 26).
//
// Decoration runs always render. A run between two module refs only renders
// once a survivor precedes it and a survivor follows it; among all runs
// accumulated in a gap since the last surviving module, only the LAST one
// renders (D2) -- a multi-run gap arises only when two or more interior
// module segments are dropped in a row (test 22).
func compose(segments []segment, survives []bool, rendered []string) string {
	var result strings.Builder

	var pendingGap []string

	havePreceding := false

	for index, seg := range segments {
		if seg.kind == literalSegment {
			if seg.decoration {
				result.WriteString(seg.literal)
			} else {
				pendingGap = append(pendingGap, seg.literal)
			}

			continue
		}

		if !survives[index] {
			continue
		}

		if havePreceding && len(pendingGap) > 0 {
			result.WriteString(pendingGap[len(pendingGap)-1])
		}

		pendingGap = pendingGap[:0]
		result.WriteString(rendered[index])

		havePreceding = true
	}

	return result.String()
}
