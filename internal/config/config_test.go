package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/felipeelias/claude-statusline/internal/config"
	"github.com/felipeelias/claude-statusline/internal/render"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefault(t *testing.T) {
	cfg := config.Default()
	assert.Equal(t, "default", cfg.Preset)
	assert.Equal(t, "$directory | $git_branch | $model | $cost | $context", cfg.Format)
	assert.Equal(t, "cyan", cfg.Directory.Style)
	assert.Equal(t, "bold", cfg.Model.Style)
	assert.False(t, cfg.Model.Disabled)
	assert.True(t, cfg.SessionTimer.Disabled)
	assert.True(t, cfg.LinesChanged.Disabled)
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
format = "$model | $cost"
[model]
style = "italic"
`), 0o644))

	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "$model | $cost", cfg.Format)
	assert.Equal(t, "italic", cfg.Model.Style)
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := config.Load("/nonexistent/path/config.toml")
	require.NoError(t, err)
	assert.Equal(t, config.Default().Format, cfg.Format)
}

func TestLoadWithPreset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
preset = "catppuccin"
`), 0o644))

	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "catppuccin", cfg.Preset)
	assert.NotEqual(t, config.Default().Format, cfg.Format)
	assert.Contains(t, cfg.Format, "$directory")
}

func TestLoadWithPresetAndOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
preset = "pure"

[model]
format = "CUSTOM: {{.DisplayName}}"
`), 0o644))

	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "pure", cfg.Preset)
	assert.Equal(t, "CUSTOM: {{.DisplayName}}", cfg.Model.Format)
}

func TestLoadWithUnknownPreset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
preset = "nonexistent"
`), 0o644))

	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, config.Default().Format, cfg.Format)
}

func TestSampleConfig(t *testing.T) {
	sample := config.SampleConfig()
	assert.Contains(t, sample, `preset = "default"`)
	assert.Contains(t, sample, "format =")
	assert.Contains(t, sample, "minimal")
	assert.Contains(t, sample, "pastel-powerline")
	assert.Contains(t, sample, "tokyo-night")
	assert.Contains(t, sample, "gruvbox-rainbow")
	assert.Contains(t, sample, "catppuccin")
	assert.Contains(t, sample, "# [model]")
	assert.Contains(t, sample, "# [cost]")
	assert.Contains(t, sample, "# [context]")
	assert.Contains(t, sample, "# [session_timer]")
}

func TestDefaultPath(t *testing.T) {
	path := config.DefaultPath()
	assert.Contains(t, path, "claude-statusline")
	assert.Contains(t, path, "config.toml")
}

func TestPriorityAbsentIsUnranked(t *testing.T) {
	cfg := config.Default()

	priority, ranked := cfg.Priority("model")

	assert.False(t, ranked)
	assert.Equal(t, 0, priority)
}

func TestPriorityExplicitZeroIsRanked(t *testing.T) {
	cfg := config.Default()
	explicitZero := 0
	cfg.Model.Priority = &explicitZero

	priority, ranked := cfg.Priority("model")

	assert.True(t, ranked)
	assert.Equal(t, 0, priority)
}

func TestPriorityUnknownModuleIsUnranked(t *testing.T) {
	cfg := config.Default()

	priority, ranked := cfg.Priority("does_not_exist")

	assert.False(t, ranked)
	assert.Equal(t, 0, priority)
}

// TestPriorityPointerSurvivesPartialTableOverride pins BurntSushi/toml's
// decode semantics (D1): decoding a TOML table that omits `priority` must
// not clear a priority the base config already set.
func TestPriorityPointerSurvivesPartialTableOverride(t *testing.T) {
	var cfg config.Config

	baseline := 10
	cfg.Model.Priority = &baseline

	_, err := toml.Decode(`
[model]
style = "italic"
`, &cfg)
	require.NoError(t, err)

	require.NotNil(t, cfg.Model.Priority)
	assert.Equal(t, baseline, *cfg.Model.Priority)
	assert.Equal(t, "italic", cfg.Model.Style)
}

func TestLoadUserTOMLSetsPriorityOverPresetBase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
preset = "catppuccin"

[model]
priority = 7
`), 0o644))

	cfg, err := config.Load(path)
	require.NoError(t, err)

	priority, ranked := cfg.Priority("model")
	assert.True(t, ranked)
	assert.Equal(t, 7, priority)

	// The preset itself ships zero priorities (D6): every OTHER module
	// stays unranked, even though the preset base sets other fields on them.
	otherModules := []string{
		"directory", "cost", "context", "git_branch", "session_timer",
		"lines_changed", "usage", "version", "vim_mode", "agent_name",
	}
	for _, name := range otherModules {
		otherPriority, otherRanked := cfg.Priority(name)
		assert.False(t, otherRanked, "module %s should be unranked", name)
		assert.Equal(t, 0, otherPriority, "module %s priority", name)
	}
}

// TestLoadPartialModuleTableLeavesPriorityUnranked exercises the real
// two-pass config.Load() path (preset header -> ApplyPreset -> user TOML
// decoded on top), not a raw toml.Decode against a synthetic Config. T4
// depends on Load(), so a [model] table that sets only style (no priority)
// must leave Priority unranked while still applying the style override.
func TestLoadPartialModuleTableLeavesPriorityUnranked(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
preset = "tokyo-night"

[model]
style = "italic"
`), 0o644))

	cfg, err := config.Load(path)
	require.NoError(t, err)

	assert.Nil(t, cfg.Model.Priority)

	priority, ranked := cfg.Priority("model")
	assert.False(t, ranked)
	assert.Equal(t, 0, priority)
	assert.Equal(t, "italic", cfg.Model.Style)
}

// TestLoadFitMarginRoundTripsAndClamps confirms [fit] margin survives the
// real config.Load() path (not just direct struct mutation) and that a
// negative margin loaded from disk still clamps to 0 via Margin() (D8).
func TestLoadFitMarginRoundTripsAndClamps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[fit]
margin = -1
`), 0o644))

	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, -1, cfg.Fit.Margin)
	assert.Equal(t, 0, cfg.Margin())
}

func TestPriorityDecodesForEveryModule(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[model]
priority = 1
[directory]
priority = 2
[cost]
priority = 3
[context]
priority = 4
[git_branch]
priority = 5
[session_timer]
priority = 6
[lines_changed]
priority = 7
[usage]
priority = 8
[version]
priority = 9
[vim_mode]
priority = 10
[agent_name]
priority = 11
`), 0o644))

	cfg, err := config.Load(path)
	require.NoError(t, err)

	cases := []struct {
		module       string
		wantPriority int
	}{
		{"model", 1},
		{"directory", 2},
		{"cost", 3},
		{"context", 4},
		{"git_branch", 5},
		{"session_timer", 6},
		{"lines_changed", 7},
		{"usage", 8},
		{"version", 9},
		{"vim_mode", 10},
		{"agent_name", 11},
	}

	for _, tc := range cases {
		priority, ranked := cfg.Priority(tc.module)
		assert.True(t, ranked, "module %s should be ranked", tc.module)
		assert.Equal(t, tc.wantPriority, priority, "module %s priority", tc.module)
	}
}

func TestFitMarginParses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[fit]
margin = 3
`), 0o644))

	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, 3, cfg.Fit.Margin)
	assert.Equal(t, 3, cfg.Margin())
}

func TestFitNegativeMarginClampsToZero(t *testing.T) {
	cfg := config.Default()
	cfg.Fit.Margin = -5

	assert.Equal(t, 0, cfg.Margin())
}

// moduleNames lists every module name registered with the statusline --
// shared between TestAllPresetsShipZeroRankedModules and
// TestModuleKeysetsStaySynchronized so there is only one hand-maintained
// copy of it in this package.
var moduleNames = []string{
	"model", "directory", "cost", "context", "git_branch",
	"session_timer", "lines_changed", "usage", "version",
	"vim_mode", "agent_name",
}

func TestAllPresetsShipZeroRankedModules(t *testing.T) {
	for _, presetName := range config.PresetNames() {
		cfg, ok := config.ApplyPreset(presetName)
		require.True(t, ok, "preset %s should be found", presetName)

		for _, moduleName := range moduleNames {
			_, ranked := cfg.Priority(moduleName)
			assert.False(t, ranked, "preset %s module %s should ship unranked", presetName, moduleName)
		}
	}
}

// TestModuleKeysetsStaySynchronized guards a latent footgun flagged at CP1:
// there are three independent copies of the 11-module list --
// internal/render/render.go's registry, this package's Priority accessor,
// and moduleNames above. Adding a 12th module and missing just one of them
// makes it silently mandatory forever: no error, no test failure, verified
// by the expediter. This derives the ground-truth set reflectively from
// Config's own struct shape (every field whose type has its own Priority
// field), independent of any hand-maintained list, and checks all three
// against it.
func TestModuleKeysetsStaySynchronized(t *testing.T) {
	expected := make(map[string]bool)

	cfgType := reflect.TypeFor[config.Config]()
	for fieldIndex := range cfgType.NumField() {
		field := cfgType.Field(fieldIndex)
		if field.Type.Kind() != reflect.Struct {
			continue
		}

		if _, hasPriority := field.Type.FieldByName("Priority"); !hasPriority {
			continue
		}

		tag, hasTag := field.Tag.Lookup("toml")
		require.True(t, hasTag, "field %s has no toml tag", field.Name)

		expected[tag] = true

		// The Priority accessor must recognize this name: setting only
		// this module's own Priority field must make cfg.Priority(tag)
		// report ranked=true.
		cfg := config.Default()

		priorityField := reflect.ValueOf(&cfg).Elem().Field(fieldIndex).FieldByName("Priority")
		require.True(t, priorityField.IsValid())

		priority := 42
		priorityField.Set(reflect.ValueOf(&priority))

		_, ranked := cfg.Priority(tag)
		assert.True(t, ranked, "config.Priority must recognize %q once its own Priority field is set", tag)
	}

	registry := render.ModuleFactory(config.Default())

	registryNames := make(map[string]bool, len(registry))
	for name := range registry {
		registryNames[name] = true
	}

	assert.Equal(t, expected, registryNames, "render's module registry must match Config's Priority-bearing fields")

	sweepNames := make(map[string]bool, len(moduleNames))
	for _, name := range moduleNames {
		sweepNames[name] = true
	}

	assert.Equal(t, expected, sweepNames, "the preset-sweep module list must match Config's Priority-bearing fields")
}
