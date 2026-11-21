package ops

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"sapato/internal/config"
	"sapato/internal/vault"
)

// ReasonModified is the Reason of a skip made because a copy is modified and
// Options.Force is not set.
const ReasonModified = "modified"

// Update re-installs the outdated skills matched by skills into each agent
// (spec §7). With no skills it covers every skill. A modified copy is skipped
// unless opts.Force is set. An outdated skill whose vault copy is invalid is
// never installed: a literal name fails the call and a glob skips it, as in
// Install. When a check fails nothing is changed.
func Update(v *vault.Vault, agents []config.Agent, skills []string, opts Options) ([]Action, error) {
	if err := checkAgents(agents); err != nil {
		return nil, fmt.Errorf("update: %w", err)
	}
	rows, err := Status(v, agents, Filter{Skills: skills})
	if err != nil {
		return nil, fmt.Errorf("update: %w", err)
	}
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		names = append(names, r.Skill)
	}
	if err := checkMatched(skills, names); err != nil {
		return nil, fmt.Errorf("update: %w", err)
	}
	vaultSkills, err := v.Skills()
	if err != nil {
		return nil, fmt.Errorf("update: %w", err)
	}
	byName := make(map[string]vault.Skill, len(vaultSkills))
	for _, s := range vaultSkills {
		byName[s.Name] = s
	}

	literal := literalNames(skills)
	dirs := agentDirs(agents)
	var actions []Action
	invalid := make(map[string]string)
	for _, r := range rows {
		if r.Status != Outdated && r.Status != Modified {
			continue
		}
		s, ok := byName[r.Skill]
		if !ok {
			// A modified copy with no vault skill has nothing to install from.
			continue
		}
		if !s.Valid {
			if literal[r.Skill] {
				invalid[r.Skill] = strings.Join(s.Problems, "; ")
				continue
			}
			actions = append(actions, Action{Op: OpSkip, Skill: r.Skill, Agent: r.Agent, Reason: "invalid"})
			continue
		}
		if r.Status == Modified && !opts.Force {
			actions = append(actions, Action{Op: OpSkip, Skill: r.Skill, Agent: r.Agent, Reason: ReasonModified})
			continue
		}
		actions = append(actions, Action{Op: OpUpdate, Skill: r.Skill, Agent: r.Agent, Path: filepath.Join(dirs[r.Agent], r.Skill)})
	}
	if len(invalid) > 0 {
		var list []string
		for _, name := range slices.Sorted(maps.Keys(invalid)) {
			list = append(list, fmt.Sprintf("%s (%s)", name, invalid[name]))
		}
		return nil, fmt.Errorf("update: invalid skills are never installed: %s", strings.Join(list, ", "))
	}
	if opts.DryRun {
		return actions, nil
	}
	if err := applyActions(actions, byName, dirs); err != nil {
		return nil, err
	}
	return actions, nil
}
