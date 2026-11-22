package ops

import (
	"fmt"
	"slices"

	"sapato/internal/vault"
)

// Compat returns the agents the vault skill skill is compatible with, or nil when
// it is compatible with every agent (spec §7).
func Compat(v *vault.Vault, skill string) ([]string, error) {
	if err := checkInVault(v, skill); err != nil {
		return nil, fmt.Errorf("compat %s: %w", skill, err)
	}
	return v.CompatAgents(skill), nil
}

// SetCompat sets the compatible list of the vault skill skill to agents (spec §3,
// §7). A nil agents list means every agent and removes the skill's table. An empty
// list is an error. Duplicate names are kept once, in the order given. Agents need
// not be configured here. When a check fails nothing is changed.
func SetCompat(v *vault.Vault, skill string, agents []string, opts Options) error {
	if err := checkInVault(v, skill); err != nil {
		return fmt.Errorf("compat %s: %w", skill, err)
	}
	var list []string
	if agents != nil {
		for _, a := range agents {
			if !slices.Contains(list, a) {
				list = append(list, a)
			}
		}
		if err := vault.ValidateCompat(list); err != nil {
			return fmt.Errorf("compat %s: %w", skill, err)
		}
	}
	if opts.DryRun {
		return nil
	}
	if err := v.EditCompat(map[string][]string{skill: list}); err != nil {
		return fmt.Errorf("compat %s: %w", skill, err)
	}
	return nil
}

// checkInVault reports an error unless skill is a skill folder in the vault.
func checkInVault(v *vault.Vault, skill string) error {
	skills, err := v.Skills()
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(skills, func(s vault.Skill) bool { return s.Name == skill }) {
		return fmt.Errorf("no skill in vault")
	}
	return nil
}
