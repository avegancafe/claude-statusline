package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appcli "github.com/felipeelias/claude-statusline/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptCommand(t *testing.T) {
	t.Setenv("CLAUDE_STATUSLINE_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	jsonInput := `{
		"model": {"display_name": "Claude Opus 4"},
		"cwd": "/tmp/test",
		"cost": {"total_cost_usd": 0.42},
		"context_window": {"used_percentage": 42.5}
	}`

	var stdout bytes.Buffer
	app := appcli.New("test")
	app.Reader = strings.NewReader(jsonInput)
	app.Writer = &stdout

	err := app.Run([]string{"claude-statusline", "prompt"})
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Claude Opus 4")
	assert.Contains(t, stdout.String(), "$0.42")
	assert.Contains(t, stdout.String(), "42%")
}

func TestDefaultAction(t *testing.T) {
	t.Setenv("CLAUDE_STATUSLINE_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	jsonInput := `{
		"model": {"display_name": "Test Model"},
		"cwd": "/tmp",
		"cost": {"total_cost_usd": 0.10},
		"context_window": {"used_percentage": 10}
	}`

	var stdout bytes.Buffer
	app := appcli.New("test")
	app.Reader = strings.NewReader(jsonInput)
	app.Writer = &stdout

	err := app.Run([]string{"claude-statusline"})
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Test Model")
}

func TestInitCommand(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "claude-statusline", "config.toml")

	var stdout bytes.Buffer
	app := appcli.New("test")
	app.Writer = &stdout

	err := app.Run([]string{"claude-statusline", "--config", configPath, "init"})
	require.NoError(t, err)

	assert.Contains(t, stdout.String(), "Config created")

	content, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "format =")
	assert.Contains(t, string(content), `preset = "default"`)
}

func TestInitCommandAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	err := os.WriteFile(configPath, []byte("existing"), 0644)
	require.NoError(t, err)

	app := appcli.New("test")
	err = app.Run([]string{"claude-statusline", "--config", configPath, "init"})
	assert.Error(t, err, "should fail if config already exists")
}

func TestTestCommand(t *testing.T) {
	t.Setenv("CLAUDE_STATUSLINE_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	app := appcli.New("test")
	app.Writer = &stdout

	err := app.Run([]string{"claude-statusline", "test"})
	require.NoError(t, err)

	result := stdout.String()
	assert.Contains(t, result, "Claude Opus 4")
	assert.Contains(t, result, "$0.42")
	assert.Contains(t, result, "42%")
}

func TestThemesCommand(t *testing.T) {
	t.Setenv("CLAUDE_STATUSLINE_CONFIG", filepath.Join(t.TempDir(), "config.toml"))

	var stdout bytes.Buffer
	app := appcli.New("test")
	app.Writer = &stdout

	err := app.Run([]string{"claude-statusline", "themes"})
	require.NoError(t, err)

	result := stdout.String()
	assert.Contains(t, result, "current:")

	// Preset names
	assert.Contains(t, result, "default:")
	assert.Contains(t, result, "minimal:")
	assert.Contains(t, result, "pastel-powerline:")
	assert.Contains(t, result, "tokyo-night:")
	assert.Contains(t, result, "gruvbox-rainbow:")
	assert.Contains(t, result, "catppuccin:")
}

// TestPromptCommandRenderErrorShowsStdoutMarker verifies D8: a render error
// used to be invisible (ErrWriter only, exit 0, blank stdout). It must now
// also emit a short marker to stdout so Claude Code's statusline shows
// something -- while leaving the ErrWriter text and the nil exit unchanged.
func TestPromptCommandRenderErrorShowsStdoutMarker(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	err := os.WriteFile(configPath, []byte("[model]\nformat = \"{{.Bogus}}\"\n"), 0644)
	require.NoError(t, err)

	jsonInput := `{"model": {"display_name": "Claude Opus 4"}}`

	var stdout, stderr bytes.Buffer
	app := appcli.New("test")
	app.Reader = strings.NewReader(jsonInput)
	app.Writer = &stdout
	app.ErrWriter = &stderr

	runErr := app.Run([]string{"claude-statusline", "--config", configPath, "prompt"})
	require.NoError(t, runErr, "D8: still exit 0 on render error")

	assert.Contains(t, stdout.String(), "(statusline error)")
	assert.Contains(t, stderr.String(), "render error:")
}

// TestPromptCommandColumnsDropsLowPriorityModule verifies D5's plumbing end
// to end: COLUMNS is read from the environment in internal/cli and reaches
// render.Render, so a narrow terminal actually drops the lower-priority
// module.
func TestPromptCommandColumnsDropsLowPriorityModule(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	tomlConfig := "format = \"$model | $context\"\n\n[model]\npriority = 10\n\n[context]\npriority = 90\n"
	err := os.WriteFile(configPath, []byte(tomlConfig), 0644)
	require.NoError(t, err)

	jsonInput := `{
		"model": {"display_name": "Claude Opus 4"},
		"context_window": {"used_percentage": 42.5}
	}`

	var stdout bytes.Buffer
	app := appcli.New("test")
	app.Reader = strings.NewReader(jsonInput)
	app.Writer = &stdout

	t.Setenv("COLUMNS", "10")

	err = app.Run([]string{"claude-statusline", "--config", configPath, "prompt"})
	require.NoError(t, err)

	result := stdout.String()
	assert.NotContains(t, result, "Claude Opus 4", "model (pri10) should lose to context (pri90) at columns=10")
	assert.Contains(t, result, "42%")
}

func TestVersionFlag(t *testing.T) {
	var stdout bytes.Buffer
	app := appcli.New("1.2.3")
	app.Writer = &stdout

	err := app.Run([]string{"claude-statusline", "--version"})
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "1.2.3")
}
