package tui

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"sapato/internal/config"
	"sapato/internal/ops"
	"sapato/internal/target"
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
	Adopt(from config.Agent, skills []string, opts ops.Options) ([]ops.Action, error)
	Unmanaged(agents []config.Agent) ([]ops.UnmanagedCopy, error)
	ToggleCompat(skill string, agents, configured []string) ([]string, error)
	Remove(skill string, opts ops.Options) (ops.Action, error)
	Create(content []byte, opts ops.Options) (ops.Action, error)
	Scaffold(name, description string, compatible []string, opts ops.Options) (ops.Action, error)
	Discover(src string) ([]ops.Candidate, error)
	Import(src string, opts ops.ImportOptions) ([]ops.Action, error)
	ExportName(skill string) (string, error)
	Export(skill, path string) ([]ops.Action, error)
	Diff(agent config.Agent, skill string) ([]ops.FileChange, error)
	Files(dir string) ([]ops.File, error)
	LockEntry(agentDir, skill string) (target.LockEntry, bool, error)
}

// opsBackend calls the ops package on one vault. agents are every configured
// agent, since a removal refuses while any of them has a copy.
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

// Adopt adopts skills from the agent from, and records identical copies in the other
// configured agents.
func (b opsBackend) Adopt(from config.Agent, skills []string, opts ops.Options) ([]ops.Action, error) {
	return ops.Adopt(b.v, b.agents, from, skills, opts)
}

// Unmanaged lists the unmanaged copies in agents, with their hashes, for ADOPT.
func (b opsBackend) Unmanaged(agents []config.Agent) ([]ops.UnmanagedCopy, error) {
	return ops.UnmanagedCopies(b.v, agents)
}

// ToggleCompat flips the compatibility of skill with each agent, and returns the new
// list, nil for every agent.
func (b opsBackend) ToggleCompat(skill string, agents, configured []string) ([]string, error) {
	return ops.ToggleCompat(b.v, skill, agents, configured)
}

func (b opsBackend) Remove(skill string, opts ops.Options) (ops.Action, error) {
	return ops.Remove(b.v, b.agents, skill, opts)
}

func (b opsBackend) Create(content []byte, opts ops.Options) (ops.Action, error) {
	return ops.Create(b.v, content, opts)
}

// Scaffold writes the template of a new skill, as the NEW wizard does.
func (b opsBackend) Scaffold(name, description string, compatible []string, opts ops.Options) (ops.Action, error) {
	return ops.Scaffold(b.v, name, description, compatible, opts)
}

// Discover lists the skills of an import source without changing the vault. The
// source is read again by Import, so the temp copy is removed here. A path may start
// with ~.
func (b opsBackend) Discover(src string) ([]ops.Candidate, error) {
	cands, cleanup, err := ops.Discover(expandHome(src), "")
	cleanup()
	return cands, err
}

// Import takes a folder, a git URL or a tarball. A path may start with ~.
func (b opsBackend) Import(src string, opts ops.ImportOptions) ([]ops.Action, error) {
	return ops.Import(b.v, expandHome(src), opts)
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

// Files lists the files of the skill folder dir, as the skill screen shows them.
func (b opsBackend) Files(dir string) ([]ops.File, error) {
	return ops.SkillFiles(dir)
}

// LockEntry reads the lock entry of a skill in the agent folder agentDir.
func (b opsBackend) LockEntry(agentDir, skill string) (target.LockEntry, bool, error) {
	return ops.LockEntry(agentDir, skill)
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
