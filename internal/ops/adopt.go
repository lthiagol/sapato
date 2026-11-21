package ops

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"sapato/internal/config"
	"sapato/internal/vault"
)

// Adopt copies the unmanaged skill folder skill of agent into the vault and
// records it in the agent's lock, so it becomes synced (spec §7). The folder must
// be a valid skill. A vault skill with the same name is refused unless
// opts.Force is set. When a check fails nothing is changed.
func Adopt(v *vault.Vault, agent config.Agent, skill string, opts Options) (Action, error) {
	if !managedName(skill) {
		return Action{}, fmt.Errorf("adopt %s in %s: not a skill folder name", skill, agent.ID)
	}
	rows, err := Status(v, []config.Agent{agent}, Filter{})
	if err != nil {
		return Action{}, fmt.Errorf("adopt %s in %s: %w", skill, agent.ID, err)
	}
	status := NotInstalled
	for _, r := range rows {
		if r.Skill == skill {
			status = r.Status
		}
	}
	if status != Unmanaged {
		return Action{}, fmt.Errorf("adopt %s in %s: %s, want unmanaged", skill, agent.ID, status)
	}

	dir := filepath.Join(agent.Path, skill)
	s, err := vault.Load(dir, skill)
	if err != nil {
		return Action{}, fmt.Errorf("adopt %s in %s: %w", skill, agent.ID, err)
	}
	if !s.Valid {
		return Action{}, fmt.Errorf("adopt %s in %s: invalid skill: %s", skill, agent.ID, strings.Join(s.Problems, "; "))
	}

	dest := v.SkillDir(skill)
	present, err := pathExists(dest)
	if err != nil {
		return Action{}, fmt.Errorf("adopt %s in %s: %w", skill, agent.ID, err)
	}
	if present && !opts.Force {
		return Action{}, fmt.Errorf("adopt %s in %s: %w", skill, agent.ID, forceRefusal("already in the vault (use --force)"))
	}

	action := Action{Op: OpAdopt, Skill: skill, Agent: agent.ID, Path: dir}
	if opts.DryRun {
		return action, nil
	}
	commit := func() error { return recordLock(agent.Path, s, time.Now().UTC()) }
	if err := placeSkillThen(dir, dest, commit); err != nil {
		return Action{}, fmt.Errorf("adopt %s in %s: %w", skill, agent.ID, err)
	}
	return action, nil
}
