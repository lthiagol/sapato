package tui

import (
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
	Diff(agent config.Agent, skill string) ([]ops.FileChange, error)
}

// opsBackend calls the ops package on one vault.
type opsBackend struct {
	v *vault.Vault
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

func (b opsBackend) Diff(agent config.Agent, skill string) ([]ops.FileChange, error) {
	return ops.Diff(b.v, agent, skill)
}
