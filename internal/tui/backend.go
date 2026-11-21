package tui

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"sapato/internal/config"
	"sapato/internal/ops"
	"sapato/internal/vault"
)

// Backend is the set of ops calls the UI makes. The real one wraps a vault;
// tests supply a fake.
type Backend interface {
	Status(agents []config.Agent) ([]ops.Row, error)
	Skills() ([]vault.Skill, error)
	Install(agents []config.Agent, skills []string, opts ops.Options) ([]ops.Action, error)
	Uninstall(agents []config.Agent, skills []string, opts ops.Options) ([]ops.Action, error)
	Update(agents []config.Agent, skills []string, opts ops.Options) ([]ops.Action, error)
	Adopt(agent config.Agent, skill string, opts ops.Options) (ops.Action, error)
	Remove(skill string, opts ops.Options) (ops.Action, error)
	Create(content []byte, opts ops.Options) (ops.Action, error)
	Import(src string, opts ops.Options) ([]ops.Action, error)
	ExportName(skill string) (string, error)
	Export(skill, path string) ([]ops.Action, error)
	Diff(agent config.Agent, skill string) ([]ops.FileChange, error)
}

// opsBackend calls the ops package on one vault. agents are every configured
// agent, enabled or not, since a removal refuses while any of them has a copy.
type opsBackend struct {
	v      *vault.Vault
	agents []config.Agent
}

func (b opsBackend) Status(agents []config.Agent) ([]ops.Row, error) {
	return ops.Status(b.v, agents, ops.Filter{})
}

// Skills lists the vault skills, valid or not, as the list command does.
func (b opsBackend) Skills() ([]vault.Skill, error) {
	return b.v.Skills()
}

func (b opsBackend) Install(agents []config.Agent, skills []string, opts ops.Options) ([]ops.Action, error) {
	return ops.Install(b.v, agents, skills, opts)
}

func (b opsBackend) Uninstall(agents []config.Agent, skills []string, opts ops.Options) ([]ops.Action, error) {
	return ops.Uninstall(b.v, agents, skills, opts)
}

func (b opsBackend) Update(agents []config.Agent, skills []string, opts ops.Options) ([]ops.Action, error) {
	return ops.Update(b.v, agents, skills, opts)
}

func (b opsBackend) Adopt(agent config.Agent, skill string, opts ops.Options) (ops.Action, error) {
	return ops.Adopt(b.v, agent, skill, opts)
}

func (b opsBackend) Remove(skill string, opts ops.Options) (ops.Action, error) {
	return ops.Remove(b.v, b.agents, skill, opts)
}

func (b opsBackend) Create(content []byte, opts ops.Options) (ops.Action, error) {
	return ops.Create(b.v, content, opts)
}

// Import takes a folder, a git URL or a tarball. A path may start with ~.
func (b opsBackend) Import(src string, opts ops.Options) ([]ops.Action, error) {
	return ops.Import(b.v, expandHome(src), ops.ImportOptions{Options: opts})
}

// ExportName is the default file name of an export of skill, as sapato export names it.
func (b opsBackend) ExportName(skill string) (string, error) {
	return ops.ExportName(b.v, []string{skill}, time.Now())
}

// Export writes skill to a new file at path, which must not exist, as sapato
// export does. A path may start with ~.
func (b opsBackend) Export(skill, path string) ([]ops.Action, error) {
	return ops.ExportFile(b.v, []string{skill}, expandHome(path))
}

func (b opsBackend) Diff(agent config.Agent, skill string) ([]ops.FileChange, error) {
	return ops.Diff(b.v, agent, skill)
}

// expandHome replaces a leading ~ with the home dir, as config paths are (spec §2).
func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[1:])
}
