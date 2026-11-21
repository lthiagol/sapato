package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sapato/internal/config"
	"sapato/internal/target"
	"sapato/internal/vault"
)

// Uninstall removes the copies of the skills matched by skills from each agent,
// together with their lock entries (spec §7). An unmanaged folder named literally
// is refused, and one matched only by a glob is skipped; unmanaged folders are
// never removed. Modified ones are refused unless opts.Force is set. A skill
// that is not installed in an agent is skipped. When a check fails nothing is
// changed.
func Uninstall(v *vault.Vault, agents []config.Agent, skills []string, opts Options) ([]Action, error) {
	if err := checkArgs(agents, skills); err != nil {
		return nil, fmt.Errorf("uninstall: %w", err)
	}
	rows, err := Status(v, agents, Filter{Skills: skills})
	if err != nil {
		return nil, fmt.Errorf("uninstall: %w", err)
	}
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		names = append(names, r.Skill)
	}
	if err := checkMatched(skills, names); err != nil {
		return nil, fmt.Errorf("uninstall: %w", err)
	}

	literal := literalNames(skills)
	dirs := agentDirs(agents)
	var actions []Action
	var unmanaged, modified, reasons []string
	for _, r := range rows {
		switch {
		case r.Status == NotInstalled || r.Status == Invalid:
			continue
		case r.Status == Unmanaged && literal[r.Skill]:
			unmanaged = append(unmanaged, fmt.Sprintf("%s in %s: unmanaged", r.Skill, r.Agent))
			continue
		case r.Status == Unmanaged:
			actions = append(actions, Action{Op: OpSkip, Skill: r.Skill, Agent: r.Agent, Reason: "unmanaged"})
			continue
		case r.Status == Modified && !opts.Force:
			modified = append(modified, fmt.Sprintf("%s in %s: modified", r.Skill, r.Agent))
			reasons = addReason(reasons, r.Agent+" has local edits (modified)")
			continue
		}
		// Lock keys come from disk, so check them before anything is removed.
		if !managedName(r.Skill) {
			return nil, fmt.Errorf("uninstall: refusing to remove %q in %s", r.Skill, r.Agent)
		}
		actions = append(actions, Action{Op: OpUninstall, Skill: r.Skill, Agent: r.Agent, Path: filepath.Join(dirs[r.Agent], r.Skill)})
	}
	// --force cannot clear an unmanaged refusal, so it is reported alone.
	if len(unmanaged) > 0 {
		return nil, fmt.Errorf("uninstall: refusing to remove %s", strings.Join(unmanaged, "; "))
	}
	if len(modified) > 0 {
		return nil, fmt.Errorf("uninstall: %w", forceRefusal(fmt.Sprintf("refusing to remove %s (use --force)", strings.Join(modified, "; ")), reasons...))
	}
	if opts.DryRun {
		return actions, nil
	}

	for _, a := range actions {
		if a.Op == OpSkip {
			continue
		}
		if err := uninstallSkill(dirs[a.Agent], a.Skill); err != nil {
			return nil, fmt.Errorf("uninstall %s from %s: %w", a.Skill, a.Agent, err)
		}
	}
	return actions, nil
}

// uninstallSkill removes the folder name from the agent directory dir and drops
// its lock entry.
func uninstallSkill(dir, name string) error {
	lock, err := target.ReadLock(dir)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(dir, name)); err != nil {
		return err
	}
	delete(lock.Skills, name)
	return target.WriteLock(dir, lock)
}

// managedName reports whether name can be a skill folder sapato installs: a
// plain name, not hidden. Such a name cannot reach outside the agent directory.
func managedName(name string) bool {
	return filepath.Base(name) == name && !strings.HasPrefix(name, ".")
}
