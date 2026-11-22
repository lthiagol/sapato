// Package config loads sapato's config file and resolves built-in agent defaults.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

// Agent is a coding agent whose user-level skills directory sapato manages.
type Agent struct {
	ID   string
	Path string
	// ReadDirs are the extra dirs the agent reads besides Path (spec §2), with ~
	// expanded.
	ReadDirs []string
}

// Config is the resolved configuration with ~ expanded.
type Config struct {
	// Path is the config file, whether or not it exists.
	Path string
	// FileExists reports whether Path exists. Without a file the defaults apply.
	FileExists bool
	// DefaultAgents reports that the built-in agents are in use because the file
	// has no [agents] table.
	DefaultAgents bool
	Vault         string
	Agents        []Agent
	UI            UI
}

// UI is the [ui] section: the color theme and icon set of the TUI.
type UI struct {
	Theme string
	Icons string
}

type uiSettings struct {
	Theme *string `toml:"theme"`
	Icons *string `toml:"icons"`
}

// file is the TOML document. Each [agents] value is a path string or an inline
// table, decoded as any and checked by agentFromValue.
type file struct {
	Vault  *string        `toml:"vault"`
	Agents map[string]any `toml:"agents"`
	UI     uiSettings     `toml:"ui"`
}

const (
	defaultVault = "~/sapato-vault"
	defaultTheme = "catppuccin-mocha"
	defaultIcons = "nerd"
)

// agentNamePattern is the form of an agent name in [agents] (spec §2).
var agentNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// ValidAgentName reports whether name has the form of an agent name (spec §2).
func ValidAgentName(name string) bool { return agentNamePattern.MatchString(name) }

// themeNames lists the themes of spec §9.5 in UI order. internal/tui/theme holds the same names.
var themeNames = []string{
	"catppuccin-latte", "catppuccin-frappe", "catppuccin-macchiato", "catppuccin-mocha",
	"dracula", "dracula-alucard", "nord", "nord-light",
}

// iconNames lists the icon sets of spec §9.5. internal/tui/icons holds the same names.
var iconNames = []string{"nerd", "unicode", "ascii"}

// ThemeNames returns the valid theme names in UI order.
func ThemeNames() []string { return slices.Clone(themeNames) }

// IconNames returns the valid icon set names.
func IconNames() []string { return slices.Clone(iconNames) }

// ValidateTheme returns an error listing the valid names when name is not a theme.
func ValidateTheme(name string) error {
	if slices.Contains(themeNames, name) {
		return nil
	}
	return fmt.Errorf("unknown theme %q (valid: %s)", name, strings.Join(themeNames, ", "))
}

// ValidateIcons returns an error listing the valid names when name is not an icon set.
func ValidateIcons(name string) error {
	if slices.Contains(iconNames, name) {
		return nil
	}
	return fmt.Errorf("unknown icon set %q (valid: %s)", name, strings.Join(iconNames, ", "))
}

// builtinAgents lists the built-in agents in spec order. Paths use ~.
var builtinAgents = []Agent{
	{ID: "claude", Path: "~/.claude/skills"},
	{ID: "cursor", Path: "~/.cursor/skills", ReadDirs: []string{"~/.claude/skills"}},
	{ID: "pi", Path: "~/.pi/agent/skills", ReadDirs: []string{"~/.agents/skills"}},
	{ID: "opencode", Path: "~/.config/opencode/skills", ReadDirs: []string{"~/.claude/skills", "~/.agents/skills"}},
	{ID: "agents", Path: "~/.agents/skills"},
}

// Load reads the config at path. An empty path means <home>/.config/sapato/config.toml.
// A missing file yields the defaults.
func Load(path string, home string) (Config, error) {
	path = Path(path, home)
	cfg := Config{Path: path, Vault: defaultVault, UI: UI{Theme: defaultTheme, Icons: defaultIcons}}

	var f file
	var names []string
	var hasAgents bool
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	default:
		cfg.FileExists = true
		if err := toml.Unmarshal(data, &f); err != nil {
			return Config{}, fmt.Errorf("parse config %s: %w", path, err)
		}
		names, hasAgents, err = agentNames(data)
		if err != nil {
			return Config{}, fmt.Errorf("config %s: %w", path, err)
		}
	}

	if f.Vault != nil {
		if *f.Vault == "" {
			return Config{}, fmt.Errorf("config %s: vault must not be empty", path)
		}
		cfg.Vault = *f.Vault
	}
	cfg.Vault = expandHome(cfg.Vault, home)

	if f.UI.Theme != nil {
		cfg.UI.Theme = *f.UI.Theme
	}
	if err := ValidateTheme(cfg.UI.Theme); err != nil {
		return Config{}, fmt.Errorf("config %s: ui.theme: %w", path, err)
	}
	if f.UI.Icons != nil {
		cfg.UI.Icons = *f.UI.Icons
	}
	if err := ValidateIcons(cfg.UI.Icons); err != nil {
		return Config{}, fmt.Errorf("config %s: ui.icons: %w", path, err)
	}

	if hasAgents {
		cfg.Agents, err = parseAgents(names, f.Agents, home)
		if err != nil {
			return Config{}, fmt.Errorf("config %s: %w", path, err)
		}
	} else {
		cfg.DefaultAgents = true
		cfg.Agents = defaultAgents(home)
	}

	owner := make(map[string]string, len(cfg.Agents))
	for _, a := range cfg.Agents {
		clean := filepath.Clean(a.Path)
		if other, ok := owner[clean]; ok {
			return Config{}, fmt.Errorf("config %s: %s and %s both use %s", path, other, a.ID, clean)
		}
		owner[clean] = a.ID
	}

	return cfg, nil
}

// defaultAgents returns the built-in agents with ~ expanded.
func defaultAgents(home string) []Agent {
	agents := make([]Agent, 0, len(builtinAgents))
	for _, def := range builtinAgents {
		agents = append(agents, Agent{ID: def.ID, Path: expandHome(def.Path, home), ReadDirs: expandHomeAll(def.ReadDirs, home)})
	}
	return agents
}

// parseAgents returns the agents of an [agents] table, in the order of names,
// which is the file order.
func parseAgents(names []string, entries map[string]any, home string) ([]Agent, error) {
	for _, key := range slices.Sorted(maps.Keys(entries)) {
		if !slices.Contains(names, key) {
			return nil, fmt.Errorf("agents.%s: use %s = \"path\" directly under [agents] (see docs/spec.md §2)", key, key)
		}
	}

	agents := make([]Agent, 0, len(names))
	for _, name := range names {
		if !agentNamePattern.MatchString(name) {
			return nil, fmt.Errorf("agent name %q must match %s", name, agentNamePattern)
		}
		a, err := agentFromValue(name, entries[name], home)
		if err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, nil
}

// agentFromValue reads one [agents] entry: a path string, or an inline table with
// path and an optional reads list. A plain path under a built-in name inherits
// the built-in's read dirs; an inline table's reads replace them.
func agentFromValue(name string, value any, home string) (Agent, error) {
	switch v := value.(type) {
	case string:
		if v == "" {
			return Agent{}, fmt.Errorf("agents.%s: path must not be empty", name)
		}
		return Agent{ID: name, Path: expandHome(v, home), ReadDirs: builtinReads(name, home)}, nil
	case map[string]any:
		for _, key := range slices.Sorted(maps.Keys(v)) {
			if key != "path" && key != "reads" {
				return Agent{}, fmt.Errorf("agents.%s: unknown key %q (valid: path, reads)", name, key)
			}
		}
		p, ok := v["path"].(string)
		if !ok || p == "" {
			return Agent{}, fmt.Errorf("agents.%s: path must be a non-empty string", name)
		}
		var reads []string
		if raw, ok := v["reads"]; ok {
			list, ok := raw.([]any)
			if !ok {
				return Agent{}, fmt.Errorf("agents.%s: reads must be a list of paths", name)
			}
			for _, item := range list {
				s, ok := item.(string)
				if !ok {
					return Agent{}, fmt.Errorf("agents.%s: reads must be a list of paths", name)
				}
				reads = append(reads, expandHome(s, home))
			}
		}
		return Agent{ID: name, Path: expandHome(p, home), ReadDirs: reads}, nil
	default:
		return Agent{}, fmt.Errorf("agents.%s: must be a path string or an inline table with path and reads", name)
	}
}

// builtinReads returns the read dirs of the built-in agent called name, with ~
// expanded, or nil when name is not a built-in.
func builtinReads(name, home string) []string {
	i := slices.IndexFunc(builtinAgents, func(a Agent) bool { return a.ID == name })
	if i < 0 {
		return nil
	}
	return expandHomeAll(builtinAgents[i].ReadDirs, home)
}

// agentNames returns the keys of the [agents] table in file order, and whether the
// document has an [agents] table at all. The entries may be written as name = ...
// lines under [agents], as dotted keys at the top level, or as an inline table. A
// [agents.<name>] table is the old form and is an error.
func agentNames(data []byte) (names []string, present bool, err error) {
	var p unstable.Parser
	p.Reset(data)
	root := true // no table header seen yet, so the current table is the root
	inAgents := false
	for p.NextExpression() {
		e := p.Expression()
		switch e.Kind {
		case unstable.Table, unstable.ArrayTable:
			root = false
			keys := keyParts(e)
			inAgents = len(keys) == 1 && keys[0] == "agents"
			present = present || inAgents
			if len(keys) >= 2 && keys[0] == "agents" {
				return nil, false, fmt.Errorf("agents.%s: the [agents.%s] table form is replaced by %s = \"path\" under [agents] (see docs/spec.md §2)", keys[1], keys[1], keys[1])
			}
		case unstable.KeyValue:
			keys := keyParts(e)
			switch {
			case inAgents && len(keys) == 1:
				names = append(names, keys[0])
			case root && len(keys) >= 2 && keys[0] == "agents":
				present = true
				if len(keys) > 2 {
					return nil, false, fmt.Errorf("agents.%s: use %s = \"path\" directly under [agents] (see docs/spec.md §2)", keys[1], keys[1])
				}
				names = append(names, keys[1])
			case root && len(keys) == 1 && keys[0] == "agents" && e.Value().Kind == unstable.InlineTable:
				present = true
				for it := e.Value().Children(); it.Next(); {
					names = append(names, keyParts(it.Node())[0])
				}
			}
		}
	}
	if err := p.Error(); err != nil {
		return nil, false, err
	}
	return names, present, nil
}

// keyParts returns the parts of the key of a table header or key-value.
func keyParts(n *unstable.Node) []string {
	var parts []string
	for it := n.Key(); it.Next(); {
		parts = append(parts, string(it.Node().Data))
	}
	return parts
}

// Path resolves the config file path: path with ~ expanded, or
// <home>/.config/sapato/config.toml when path is empty.
func Path(path, home string) string {
	if path == "" {
		return filepath.Join(home, ".config", "sapato", "config.toml")
	}
	return expandHome(path, home)
}

// Starter returns the commented starter config. Its [agents] table lists the
// built-in agents, so it loads to the defaults. themes and icons list the valid
// names for [ui]; the caller passes them from the theme and icon packages, which
// this package cannot import.
func Starter(themes, icons []string) string {
	var b strings.Builder
	b.WriteString("# sapato config. Every key is optional; a missing key keeps its default.\n")
	b.WriteString("# Reference: docs/spec.md §2 in the sapato repo.\n\n")
	fmt.Fprintf(&b, "vault = %q        # default if omitted\n\n", defaultVault)

	b.WriteString("[ui]\n")
	fmt.Fprintf(&b, "# Color theme: %s\n", strings.Join(themes, ", "))
	fmt.Fprintf(&b, "theme = %q\n", defaultTheme)
	fmt.Fprintf(&b, "# Icon set: %s\n", strings.Join(icons, " | "))
	fmt.Fprintf(&b, "icons = %q\n\n", defaultIcons)

	b.WriteString("[agents]                        # name = path; order here = order in the UI\n")
	width := 0
	for _, a := range builtinAgents {
		width = max(width, len(a.ID))
	}
	for _, a := range builtinAgents {
		fmt.Fprintf(&b, "%-*s = %q\n", width, a.ID, a.Path)
	}
	b.WriteString("# A custom agent is one more line, for example:\n")
	fmt.Fprintf(&b, "# %-*s = %q\n", width, "codex", "~/.codex/skills")
	b.WriteString("# Inline form, with extra dirs the agent also reads (sapato doctor reports overlaps):\n")
	fmt.Fprintf(&b, "# %-*s = { path = %q, reads = [%q] }\n", width, "work", "~/work/.claude/skills", "~/.agents/skills")
	return b.String()
}

// WriteStarter writes content to path, creating parent dirs (0755) and the file
// (0644). An existing file is an error unless force is set.
func WriteStarter(path, content string, force bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir for %s: %w", path, err)
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("config %s already exists (use --force to overwrite)", path)
	}
	if err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return fmt.Errorf("write config %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}

// expandHome replaces a leading "~" with home.
func expandHome(p, home string) string {
	if p == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return p
}

// expandHomeAll applies expandHome to each path. A nil list stays nil.
func expandHomeAll(paths []string, home string) []string {
	if paths == nil {
		return nil
	}
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = expandHome(p, home)
	}
	return out
}
