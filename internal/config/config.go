// Package config loads sapato's config file and resolves built-in agent defaults.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Agent is a coding agent whose user-level skills directory sapato manages.
type Agent struct {
	ID      string
	Path    string
	Enabled bool
	// ReadDirs are the fixed dirs the agent reads besides Path (spec §2), with ~
	// expanded. They do not depend on the configured Path.
	ReadDirs []string
}

// Config is the resolved configuration with ~ expanded.
type Config struct {
	Vault  string
	Agents []Agent
}

type agentSettings struct {
	Path    *string `toml:"path"`
	Enabled *bool   `toml:"enabled"`
}

type file struct {
	Vault  *string                  `toml:"vault"`
	Agents map[string]agentSettings `toml:"agents"`
}

const defaultVault = "~/sapato-vault"

// builtinAgents lists the built-in agents in spec order. Paths use ~.
var builtinAgents = []Agent{
	{ID: "claude", Path: "~/.claude/skills", Enabled: true},
	{ID: "cursor", Path: "~/.cursor/skills", Enabled: true, ReadDirs: []string{"~/.claude/skills"}},
	{ID: "pi", Path: "~/.pi/agent/skills", Enabled: true, ReadDirs: []string{"~/.agents/skills"}},
	{ID: "opencode", Path: "~/.config/opencode/skills", Enabled: true, ReadDirs: []string{"~/.claude/skills", "~/.agents/skills"}},
	{ID: "agents", Path: "~/.agents/skills", Enabled: true},
}

// Load reads the config at path. An empty path means <home>/.config/sapato/config.toml.
// A missing file yields the defaults.
func Load(path string, home string) (Config, error) {
	if path == "" {
		path = filepath.Join(home, ".config", "sapato", "config.toml")
	}
	path = expandHome(path, home)

	var f file
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	default:
		if err := toml.Unmarshal(data, &f); err != nil {
			return Config{}, fmt.Errorf("parse config %s: %w", path, err)
		}
	}

	cfg := Config{Vault: defaultVault}
	if f.Vault != nil {
		if *f.Vault == "" {
			return Config{}, fmt.Errorf("config %s: vault must not be empty", path)
		}
		cfg.Vault = *f.Vault
	}
	cfg.Vault = expandHome(cfg.Vault, home)

	for _, def := range builtinAgents {
		a := def
		if s, ok := f.Agents[def.ID]; ok {
			if s.Path != nil {
				if *s.Path == "" {
					return Config{}, fmt.Errorf("config %s: agents.%s.path must not be empty", path, def.ID)
				}
				a.Path = *s.Path
			}
			if s.Enabled != nil {
				a.Enabled = *s.Enabled
			}
		}
		a.Path = expandHome(a.Path, home)
		a.ReadDirs = expandHomeAll(def.ReadDirs, home)
		cfg.Agents = append(cfg.Agents, a)
	}

	known := make(map[string]bool, len(builtinAgents))
	for _, a := range builtinAgents {
		known[a.ID] = true
	}
	var unknown []string
	for id := range f.Agents {
		if !known[id] {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		slices.Sort(unknown)
		return Config{}, fmt.Errorf("config %s: unknown agent %q", path, unknown[0])
	}

	owner := make(map[string]string, len(cfg.Agents))
	for _, a := range cfg.Agents {
		if !a.Enabled {
			continue
		}
		clean := filepath.Clean(a.Path)
		if other, ok := owner[clean]; ok {
			return Config{}, fmt.Errorf("config %s: %s and %s both use %s", path, other, a.ID, clean)
		}
		owner[clean] = a.ID
	}

	return cfg, nil
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
