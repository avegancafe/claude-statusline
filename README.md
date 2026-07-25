# claude-statusline

Configurable status line for [Claude Code](https://docs.anthropic.com/en/docs/claude-code).

![claude-statusline](assets/screenshot.webp)

## Installation

This is a fork of [felipeelias/claude-statusline](https://github.com/felipeelias/claude-statusline)
adding width-aware module priorities (see [Priorities](#priorities)).

```bash
go install github.com/avegancafe/claude-statusline@feat/module-priorities
```

Note the explicit ref: this fork inherited upstream's `v0.x` tags, so `@latest`
would resolve to `v0.9.0` and silently give you a build *without* priorities.
Pin a branch or a fork-specific tag.

## Setup

Add to your Claude Code settings (`.claude/settings.json` or global settings):

```json
{
  "statusLine": {
    "type": "command",
    "command": "claude-statusline prompt"
  }
}
```

Generate a starter config:

```bash
claude-statusline init
```

Preview with mock data:

```bash
claude-statusline test
claude-statusline themes
```

## Commands

| Command | Description |
|---------|-------------|
| `prompt` | Render the status line (also the default when no command is given) |
| `init` | Create default config at `~/.config/claude-statusline/config.toml` |
| `test` | Render with your config and mock data (for config iteration) |
| `themes` | Preview all built-in presets with mock data |

Global flags: `--config / -c` to override config path, `--version`.

## Configuration

Config file location: `~/.config/claude-statusline/config.toml`

Works with zero config. The default format is:

```toml
format = "$directory | $git_branch | $model | $cost | $context"
```

## Presets

Presets are inspired by [Starship presets](https://starship.rs/presets/). Each preset defines the layout, separators, colors, and module configuration.

```toml
preset = "catppuccin"
```

Preview all presets: `claude-statusline themes`

### Built-in presets

| Preset | Description | Nerd Font |
|--------|-------------|-----------|
| `default` | Flat with `\|` pipes, standard colors | No |
| `minimal` | Clean spacing, no separators | No |
| `pastel-powerline` | Pastel powerline arrows (pink/peach/blue/teal) | Yes |
| `tokyo-night` | Dark blues rounded powerline with gradient | Yes |
| `gruvbox-rainbow` | Earthy rainbow powerline | Yes |
| `catppuccin` | Catppuccin Mocha powerline | Yes |

### Overriding preset defaults

Presets set the format string and module configs, but you can override any field:

```toml
preset = "catppuccin"

# Override just one module
[model]
format = " {{.DisplayName}} "
style = "fg:#11111b bg:#cba6f7 bold"
```

## Modules

| Module | Default | Description |
|--------|---------|-------------|
| `directory` | on | Current directory (tilde-collapsed, truncated) |
| `git_branch` | on | Git branch with status indicators (dirty, ahead/behind, worktree) |
| `model` | on | Model name (display name, short name, or raw ID) |
| `cost` | on | Session cost in USD |
| `context` | on | Context window usage with progress bar |
| `session_timer` | off | Session elapsed time |
| `lines_changed` | off | Lines added/removed |
| `usage` | off | Plan usage limits (5-hour block and weekly) |
| `vim_mode` | off | Vim mode indicator (NORMAL, INSERT, etc.) |
| `agent_name` | off | Agent name when running with `--agent` |

Any module can be made droppable when the line doesn't fit your terminal — see [Priorities](#priorities) below.

### Enabling modules

To enable a disabled module, set `disabled = false` and add it to the format string:

```toml
format = "$directory | $git_branch | $model | $cost | $context | $session_timer"

[session_timer]
disabled = false
```

### Priorities

By default every module is mandatory: nothing is ever dropped, no matter how narrow the terminal. Set `priority = N` on a module to make it droppable. When the line doesn't fit `COLUMNS`, droppable modules are considered highest priority first, and each one is kept only if it — together with everything already kept — still fits. A module that's too wide to fit at that point is skipped, and lower-priority modules are still considered after it, so a wide high-priority module can be dropped while a narrower low-priority one survives. A module with no `priority` set stays mandatory regardless of what priorities other modules have. `priority` can be any integer, including negative — `priority = -5` is valid and simply ranks below `0`.

No format rewriting is needed — this works on the stock default format the moment you set a priority:

```toml
format = "$model | $cost | $context"

[cost]
priority = 20

[context]
priority = 10
```

| `COLUMNS` | Output |
|-----------|--------|
| 60 | `Opus \| $0.42 \| ██░░░ 42%` |
| 24 | `Opus \| $0.42 \| ██░░░ 42%` (exact fit) |
| 18 | `Opus \| $0.42` |
| 10 | `Opus` |

`$model` has no priority, so it's mandatory and never drops. `$context` (priority 10) drops before `$cost` (priority 20) once the line stops fitting.

This example's priority order happens to match its width order (the lower-priority module is also the wider one), which can make "highest priority first" look the same as "narrowest first." They aren't the same rule. Swap which module is wider and priority order wins even though it keeps the wider output:

```toml
format = "$model | $context | $cost"

[context]
priority = 40

[cost]
priority = 10
```

| `COLUMNS` | Output |
|-----------|--------|
| 30 | `Opus \| ██░░░ 42% \| $0.42` |
| 20 | `Opus \| ██░░░ 42%` |
| 14 | `Opus \| $0.42` |

At `COLUMNS` 14, `$context` (priority 40, 9 columns wide) is tried first and doesn't fit even by itself alongside `$model`, so it's skipped — not dropped in some later pass, skipped in place, then selection keeps going. `$cost` (priority 10, 5 columns wide) is tried next and does fit, so it's admitted. The result keeps the *lower*-priority module and drops the higher-priority one, because priority sets the trial order, not a width-based drop order.

#### Separators travel with their module

There's no separate syntax for a separator — a literal run of text between two modules (the ` | ` above, an arrow in a powerline preset) is inferred from the format string and travels with whichever modules are still standing. When a module between two others is dropped, its separator disappears with it, and exactly one separator renders between whichever two modules end up next to each other:

`format = "$model | $context | $cost"` with `$context` dropped renders `Opus | $0.42` — one separator, not two and not zero.

A literal run before the first module or after the last one (tokyo-night's leading `[░▒▓]` gradient, a powerline preset's trailing cap) is decoration, not a separator, and always renders regardless of what gets dropped.

#### `[fit] margin`

```toml
[fit]
margin = 3
```

`margin` is subtracted from `COLUMNS` before fitting; it defaults to `0`. Set `margin = 3` for Claude Code: it renders the status line inside its own chrome, and content reaching the exact terminal edge gets truncated by that chrome rather than by claude-statusline. A negative margin is accepted rather than rejected, and simply clamps to `0`.

`COLUMNS` is only exported by Claude Code on **v2.1.153 and later**. On older versions it's unset, so nothing is ever dropped — priorities are silently inert (the status line always renders in full), not broken.

#### Known limitations

- **A ranked module can't be un-ranked once a preset sets it.** TOML has no `null`, so a config decoded over a preset that already set `priority` on a module can only replace that number with another one — `priority = 0` still means "ranked lowest," not "back to mandatory." No built-in preset sets any priorities today, so this only matters if a future preset does.
- **An empty module still costs its separator's width.** A module that renders no text — `$git_branch` outside a repository, for example — is still present and still anchors the separators next to it. `format = "$git_branch | $cost"` outside a repo with `[cost] priority = 1` renders ` | $0.42` at `COLUMNS` 8 and above, but renders blank at `COLUMNS` 5 through 7, even though `$0.42` alone (5 columns) would otherwise fit. At a more realistic pane width this can starve a whole module, not just a few edge columns: ranking an empty `$git_branch` at `90` above `$cost` at `80` renders `/tmp/notarepo |  | Opus` at `COLUMNS = 28` (5 columns unused, `$cost` starved), where giving `$git_branch` a lower priority than `$cost` (e.g. `60`) renders `/tmp/notarepo | Opus | $0.42` at that same width. This isn't changed here, and there's no cost-free variant that would change it: the starved columns *are* the separators, so recovering them requires a ranked module that renders nothing to stop anchoring its separators at all — not just at narrow widths, but at every width. A wide terminal with that same ranked, empty module currently keeps both of its separators once it comfortably fits (a "doubled separator," pinned by a test), and giving up the separator cost to fix the narrow case would have to give up that guarantee too.
- **Ranking every default-on module can render a completely blank status line.** Nothing stops you from setting `priority` on all five modules that ship enabled — the commented-out priority lines in the generated `claude-statusline init` config do exactly that if you uncomment all of them — which removes the mandatory floor entirely. At a narrow enough terminal, every module can end up skipped, rendering zero bytes. This is reachable through a non-error path, so it isn't caught by the `(statusline error)` marker: the render itself succeeds, it just has nothing left to say.
- **Output can exceed `COLUMNS`.** Decoration always renders and mandatory modules never drop, so if those alone are wider than the terminal, the rendered line is wider than `COLUMNS` too — fitting only ever removes droppable modules, it never truncates what's left.
- **Dropping a module on a powerline preset can leave one mis-hued glyph.** Dropping composes cleanly **iff adjacent styled runs share a single background** — no background at all (`default`, `minimal`), or one background shared between them. It fails only where a separator encodes a transition between two *different* backgrounds, which is exactly what the arrows in the four powerline presets (`pastel-powerline`, `tokyo-night`, `gruvbox-rainbow`, `catppuccin`) do: the trailing cap is decoration, so it always renders, and once a drop removes the arrow that used to lead into it, the cap is left painted for a block that's no longer next to it — one glyph in a colour matching nothing adjacent. This is a property of those presets' colour scheme, not a bug in a particular format string, and it isn't introduced by priorities: the same four presets already show two directly-adjacent arrows around an empty `$git_branch` today, with or without any priority set — that's pre-existing behaviour, unchanged by this feature.

### Model module

Template fields:

| Field | Description | Example |
|-------|-------------|---------|
| `{{.DisplayName}}` | Display name from Claude Code (default) | `Claude Sonnet 4.6` |
| `{{.Short}}` | Compact name extracted from model ID | `Sonnet 4.6` |
| `{{.ID}}` | Raw model ID | `claude-sonnet-4-6-20250514` |

```toml
[model]
format = "{{.Short}}"
style = "bold"
```

### Usage module

The `usage` module shows your Claude plan usage limits (5-hour rolling window and 7-day). Requires Claude Code 2.1.80+ which provides `rate_limits` in the status line payload.

```toml
format = "$directory | $git_branch | $model | $cost | $context | $usage"

[usage]
disabled = false
```

Template fields:

| Field | Description |
|-------|-------------|
| `{{.BlockPct}}` | 5-hour rolling window usage (0-100) |
| `{{.WeeklyPct}}` | 7-day usage (0-100) |
| `{{.BlockBar}}` | Progress bar for 5-hour window |
| `{{.WeeklyBar}}` | Progress bar for 7-day window |
| `{{.BlockResets}}` | Time until 5-hour reset (e.g. "2h13m") |
| `{{.WeeklyResets}}` | Time until 7-day reset (e.g. "3d2h") |

To only show usage when it exceeds a threshold (e.g. 5-hour block above 70%, weekly above 80%):

```toml
[usage]
disabled = false
format = '{{if ge .BlockPct 70.0}}{{.BlockBar}} {{printf "%.0f" .BlockPct}}%{{end}}{{if ge .WeeklyPct 80.0}} W:{{printf "%.0f" .WeeklyPct}}%{{end}}'
```

The module renders empty if `rate_limits` is not present in the Claude Code payload (older versions).

### Vim mode module

The `vim_mode` module shows the current vim editor mode when vim mode is enabled in Claude Code.

```toml
format = "$vim_mode | $directory | $git_branch | $model | $cost | $context"

[vim_mode]
disabled = false
```

Template fields:

| Field | Description |
|-------|-------------|
| `{{.Mode}}` | Current vim mode (e.g. `NORMAL`, `INSERT`) |

The module renders empty if vim mode is not enabled or the mode string is empty.

## Clickable hyperlinks (OSC 8)

Modules can wrap their output in [OSC 8 terminal hyperlinks](https://gist.github.com/egmontkob/eb114294efbcd5adb1944c9f3cb5feda), making text clickable in supported terminals.

### git_branch

When enabled, the branch name becomes a clickable link to the branch page on the remote. The base URL is auto-detected from `git remote get-url origin`, and the branch path pattern is selected based on the host:

- **GitHub** (default): `/tree/<branch>`
- **GitLab** (hosts containing "gitlab"): `/-/tree/<branch>`
- **Bitbucket** (hosts containing "bitbucket"): `/src/<branch>`

Branch names are percent-encoded so characters like `#` don't break the URL.

```toml
[git_branch]
hyperlink = true
# hyperlink_base_url = "https://github.com/owner/repo"  # override auto-detection
```

### directory

When enabled, the directory text links to the path using a configurable URL template. The default opens `file://` URLs with properly encoded paths; set `hyperlink_url_template` for VS Code or other editors.

Template fields:
- `{{.AbsPathEncoded}}` — percent-encoded absolute path (use for URLs)
- `{{.AbsPath}}` — raw absolute path (use for schemes that handle raw paths, like `vscode://`)

```toml
[directory]
hyperlink = true
# hyperlink_url_template = "file://{{.AbsPathEncoded}}"  # default
# hyperlink_url_template = "vscode://file{{.AbsPath}}"   # open in VS Code
```

## Style system

Modules support a `style` field that accepts several formats:

| Format | Example |
|--------|---------|
| Named | `red`, `green`, `cyan`, `bold`, `dim`, `italic` |
| Hex | `fg:#ff5500`, `bg:#333333` |
| 256-color | `208`, `fg:208`, `bg:238` |
| Combined | `fg:#aabbcc bg:#333333 bold` |

## Alternatives

Other statusline tools from the [awesome-claude-code](https://github.com/hesreallyhim/awesome-claude-code) list:

- [claude-powerline](https://github.com/Owloops/claude-powerline)
- [CCometixLine](https://github.com/Haleclipse/CCometixLine)
- [claudia-statusline](https://github.com/hagan/claudia-statusline)
- [ccstatusline](https://github.com/sirmalloc/ccstatusline)

## Contributors

- [@sammcj](https://github.com/sammcj)

## License

MIT
