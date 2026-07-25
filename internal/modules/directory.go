package modules

import (
	"bytes"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/avegancafe/claude-statusline/internal/config"
	"github.com/avegancafe/claude-statusline/internal/input"
)

// DirectoryModule renders the current working directory with tilde substitution and truncation.
type DirectoryModule struct {
	homeDir string
}

// NewDirectoryModule creates a DirectoryModule that uses the real home directory.
func NewDirectoryModule() DirectoryModule {
	home, _ := os.UserHomeDir()

	return DirectoryModule{homeDir: home}
}

// NewDirectoryModuleWithHome creates a DirectoryModule with a custom home directory for testing.
func NewDirectoryModuleWithHome(home string) DirectoryModule {
	return DirectoryModule{homeDir: home}
}

func (DirectoryModule) Name() string { return "directory" }

// directoryTemplateData is the template context for the directory module.
//
// RepoName is a METHOD, not a field, on purpose: text/template only calls it
// when a format string actually references {{.RepoName}}, so the `git rev-parse`
// subprocess is never spawned for the default format. Making it a field would
// shell out on every render.
type directoryTemplateData struct {
	Dir  string
	Base string

	cwd string
}

// RepoName is the basename of the enclosing git worktree, falling back to Base
// outside a repository. This is what a repo-labelled statusline block usually
// wants: it stays stable as you cd into subdirectories, where Base changes.
func (d directoryTemplateData) RepoName() string {
	cmd := exec.Command("git", "-C", d.cwd, "--no-optional-locks", "rev-parse", "--show-toplevel")

	out, err := cmd.Output()
	if err != nil {
		return d.Base
	}

	root := strings.TrimSpace(string(out))
	if root == "" {
		return d.Base
	}

	return filepath.Base(root)
}

func (m DirectoryModule) Render(data input.Data, cfg config.Config) (string, error) {
	cwd := data.Cwd
	if cwd == "" {
		return "", nil
	}

	home := m.homeDir
	if home == "" {
		home, _ = os.UserHomeDir()
	}

	// Tilde substitution.
	dir := cwd
	if home != "" {
		if dir == home {
			dir = "~"
		} else if strings.HasPrefix(dir, home+"/") {
			dir = "~" + dir[len(home):]
		}
	}

	dir = truncatePath(dir, cfg.Directory.TruncationLength)

	templateData := directoryTemplateData{Dir: dir, Base: filepath.Base(cwd), cwd: cwd}

	result, err := renderTemplate("directory", cfg.Directory.Format, templateData)
	if err != nil {
		return "", err
	}

	if cfg.Directory.Hyperlink {
		linkURL := resolveDirectoryHyperlink(cfg.Directory.HyperlinkURLTemplate, cwd)
		result = WrapHyperlink(linkURL, result)
	}

	return wrapStyle(result, cfg.Directory.Style), nil
}

// truncatePath keeps the last maxSegments path segments fully and abbreviates earlier ones
// to their first character. The leading "/" or "~/" prefix is preserved.
func truncatePath(path string, maxSegments int) string {
	if maxSegments <= 0 {
		return path
	}

	prefix, segmentStr := splitPathPrefix(path)
	if segmentStr == "" {
		return prefix
	}

	segments := strings.Split(segmentStr, "/")

	if len(segments) <= maxSegments {
		return path
	}

	cutoff := len(segments) - maxSegments
	for i := range cutoff {
		if len(segments[i]) > 0 {
			runes := []rune(segments[i])
			segments[i] = string(runes[0])
		}
	}

	return prefix + strings.Join(segments, "/")
}

func splitPathPrefix(path string) (string, string) {
	if strings.HasPrefix(path, "~/") {
		return "~/", path[2:]
	}

	if path == "~" {
		return "~", ""
	}

	if strings.HasPrefix(path, "/") {
		return "/", path[1:]
	}

	return "", path
}

// resolveDirectoryHyperlink executes the URL template with the absolute path.
// Returns empty string if the template is empty or fails to execute.
func resolveDirectoryHyperlink(urlTemplate, absPath string) string {
	if urlTemplate == "" {
		return ""
	}

	tmpl, err := template.New("hyperlink_url").Parse(urlTemplate)
	if err != nil {
		return ""
	}

	data := struct {
		AbsPath        string
		AbsPathEncoded string
	}{
		AbsPath:        absPath,
		AbsPathEncoded: (&url.URL{Path: absPath}).EscapedPath(),
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	if err != nil {
		return ""
	}

	return buf.String()
}
