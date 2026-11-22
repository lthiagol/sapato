package ops

import (
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"sapato/internal/config"
	"sapato/internal/vault"
)

// SyncOptions are the flags of Sync.
type SyncOptions struct {
	Options
	// Prune uninstalls the managed skills that the assignments no longer name.
	Prune bool
}

// Sync applies the [assignments] of the vault to the agents (spec §3, §7). An
// agent with an assignment entry gets its assigned skills installed when missing
// and updated when outdated, through Install and Update. With Prune its managed
// skills that are not assigned are uninstalled, through Uninstall. An assignment
// that names an agent not in agents is skipped as "not configured here", never an
// error. Every check runs before the first change, so a refusal changes nothing.
func Sync(v *vault.Vault, agents []config.Agent, opts SyncOptions) ([]Action, error) {
	skills, err := v.Skills()
	if err != nil {
		return nil, fmt.Errorf("sync: %w", err)
	}

	var targets []config.Agent
	for _, a := range agents {
		if _, ok := v.Assignments[a.ID]; ok {
			targets = append(targets, a)
		}
	}
	rows, err := Status(v, targets, Filter{})
	if err != nil {
		return nil, fmt.Errorf("sync: %w", err)
	}
	states := make(map[string]map[string]State)
	for _, r := range rows {
		if states[r.Agent] == nil {
			states[r.Agent] = make(map[string]State)
		}
		states[r.Agent][r.Skill] = r.Status
	}

	var plan []Action
	var steps []func(Options) ([]Action, error)
	for _, a := range targets {
		valid, invalid, err := expandAssignment(v.Assignments[a.ID], skills)
		if err != nil {
			return nil, fmt.Errorf("sync %s: %w", a.ID, err)
		}
		for _, name := range invalid {
			plan = append(plan, Action{Op: OpSkip, Skill: name, Agent: a.ID, Reason: "invalid"})
		}

		assignedNames := make(map[string]bool, len(valid)+len(invalid))
		for _, name := range slices.Concat(valid, invalid) {
			assignedNames[name] = true
		}
		var toInstall, toUpdate, toPrune []string
		for _, name := range valid {
			switch states[a.ID][name] {
			case NotInstalled, Missing, Unmanaged, Incompatible:
				toInstall = append(toInstall, name)
			case Outdated, Modified:
				toUpdate = append(toUpdate, name)
			}
		}
		if opts.Prune {
			for _, name := range slices.Sorted(maps.Keys(states[a.ID])) {
				if assignedNames[name] {
					continue
				}
				switch states[a.ID][name] {
				case Synced, Outdated, Modified, Orphaned, Missing:
					toPrune = append(toPrune, name)
				}
			}
		}

		one := []config.Agent{a}
		if len(toPrune) > 0 {
			steps = append(steps, func(o Options) ([]Action, error) { return Uninstall(v, one, toPrune, o) })
		}
		if len(toInstall) > 0 {
			steps = append(steps, func(o Options) ([]Action, error) {
				o.SkipIncompatible = true
				return Install(v, one, toInstall, o)
			})
		}
		if len(toUpdate) > 0 {
			steps = append(steps, func(o Options) ([]Action, error) { return Update(v, one, toUpdate, o) })
		}
	}

	for _, id := range unconfigured(v.Assignments, agents) {
		for _, pattern := range v.Assignments[id] {
			plan = append(plan, Action{Op: OpSkip, Skill: pattern, Agent: id, Reason: "not configured here"})
		}
	}

	for _, step := range steps {
		actions, err := step(Options{Force: opts.Force, DryRun: true})
		if err != nil {
			return nil, fmt.Errorf("sync: %w", err)
		}
		plan = append(plan, actions...)
	}
	if opts.DryRun {
		return plan, nil
	}
	for _, step := range steps {
		if _, err := step(Options{Force: opts.Force}); err != nil {
			return nil, fmt.Errorf("sync: %w", err)
		}
	}
	return plan, nil
}

// unconfigured returns the sorted assignment keys that name no agent in agents.
func unconfigured(assignments map[string][]string, agents []config.Agent) []string {
	var ids []string
	for _, id := range slices.Sorted(maps.Keys(assignments)) {
		if !slices.ContainsFunc(agents, func(a config.Agent) bool { return a.ID == id }) {
			ids = append(ids, id)
		}
	}
	return ids
}

// expandAssignment returns the names of the vault skills that patterns select,
// as valid and invalid lists, each sorted. A literal name must exist and be
// valid. A glob that matches nothing selects nothing. A glob that matches an
// invalid skill lists it as invalid, so it is skipped rather than installed.
func expandAssignment(patterns []string, skills []vault.Skill) (valid, invalid []string, err error) {
	validSet := make(map[string]bool)
	invalidSet := make(map[string]bool)
	for _, p := range patterns {
		if _, err := path.Match(p, ""); err != nil {
			return nil, nil, fmt.Errorf("bad skill pattern %q: %w", p, err)
		}
		literal := isLiteral(p)
		matched := false
		for _, s := range skills {
			if ok, _ := path.Match(p, s.Name); !ok {
				continue
			}
			matched = true
			switch {
			case s.Valid:
				validSet[s.Name] = true
			case literal:
				return nil, nil, fmt.Errorf("invalid skills are never installed: %s (%s)", s.Name, strings.Join(s.Problems, "; "))
			default:
				invalidSet[s.Name] = true
			}
		}
		if !matched && literal {
			return nil, nil, fmt.Errorf("no skill %q in vault", p)
		}
	}
	return slices.Sorted(maps.Keys(validSet)), slices.Sorted(maps.Keys(invalidSet)), nil
}
