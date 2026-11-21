package ops

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"sapato/internal/config"
	"sapato/internal/vault"
)

// Remove deletes the vault skill called skill (spec §7). A skill installed in
// any of agents is refused unless opts.Force is set; with Force its installed
// copies become orphaned. When a check fails nothing is changed.
func Remove(v *vault.Vault, agents []config.Agent, skill string, opts Options) (Action, error) {
	skills, err := v.Skills()
	if err != nil {
		return Action{}, fmt.Errorf("remove %s: %w", skill, err)
	}
	i := slices.IndexFunc(skills, func(s vault.Skill) bool { return s.Name == skill })
	if i < 0 {
		return Action{}, fmt.Errorf("remove %s: no skill in vault", skill)
	}

	rows, err := Status(v, agents, Filter{})
	if err != nil {
		return Action{}, fmt.Errorf("remove %s: %w", skill, err)
	}
	var installed []string
	for _, r := range rows {
		if r.Skill == skill && (r.Status == Synced || r.Status == Outdated || r.Status == Modified) {
			installed = append(installed, r.Agent)
		}
	}
	if len(installed) > 0 && !opts.Force {
		where := "installed in " + strings.Join(installed, ", ")
		return Action{}, fmt.Errorf("remove %s: %w", skill, forceRefusal(where+" (use --force)", where))
	}

	action := Action{Op: OpRemove, Skill: skill, Path: skills[i].Dir}
	if opts.DryRun {
		return action, nil
	}
	if err := os.RemoveAll(skills[i].Dir); err != nil {
		return Action{}, fmt.Errorf("remove %s: %w", skill, err)
	}
	return action, nil
}
