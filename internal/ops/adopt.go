package ops

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"sapato/internal/config"
	"sapato/internal/hashdir"
	"sapato/internal/target"
	"sapato/internal/vault"
)

// Adopt copies the unmanaged skill folders named by skills in agent from into the
// vault, and records each in from's lock, so it becomes synced (spec §7). A name is
// a folder name or a glob over from's unmanaged skills. A folder name must be a
// valid unmanaged skill; a glob skips an invalid one. A name already in the vault
// is refused unless opts.Force is set. Every other agent in agents whose unmanaged
// copy of the name has the identical hash is recorded in its lock too. When a check
// fails nothing is changed.
func Adopt(v *vault.Vault, agents []config.Agent, from config.Agent, skills []string, opts Options) ([]Action, error) {
	if err := checkArgs(agents, skills); err != nil {
		return nil, fmt.Errorf("adopt: %w", err)
	}
	if !slices.ContainsFunc(agents, func(a config.Agent) bool { return a.ID == from.ID }) {
		return nil, fmt.Errorf("adopt: agent %q is not configured", from.ID)
	}
	for _, s := range skills {
		if isLiteral(s) && !managedName(s) {
			return nil, fmt.Errorf("adopt %s in %s: not a skill folder name", s, from.ID)
		}
	}
	literal := literalNames(skills)
	var globs []string
	for _, s := range skills {
		if !literal[s] {
			globs = append(globs, s)
		}
	}

	rows, err := Status(v, agents, Filter{Skills: skills})
	if err != nil {
		return nil, fmt.Errorf("adopt: %w", err)
	}
	state := make(map[string]State)
	var unmanaged []string
	for _, r := range rows {
		if r.Agent != from.ID {
			continue
		}
		state[r.Skill] = r.Status
		if r.Status == Unmanaged {
			unmanaged = append(unmanaged, r.Skill)
		}
	}
	for _, s := range skills {
		if !literal[s] {
			continue
		}
		st, ok := state[s]
		if !ok {
			st = NotInstalled
		}
		if st != Unmanaged {
			return nil, fmt.Errorf("adopt %s in %s: %s, want unmanaged", s, from.ID, st)
		}
	}
	for _, g := range globs {
		if !slices.ContainsFunc(unmanaged, func(n string) bool { return matchesAny([]string{g}, n) }) {
			return nil, fmt.Errorf("adopt: no unmanaged skill in %s matches %q", from.ID, g)
		}
	}

	dirs := agentDirs(agents)
	var actions []Action
	var adopts []vault.Skill
	var refused, reasons []string
	for _, r := range rows {
		if r.Agent != from.ID || r.Status != Unmanaged {
			continue
		}
		dir := filepath.Join(dirs[from.ID], r.Skill)
		s, err := vault.Load(dir, r.Skill)
		if err != nil {
			return nil, fmt.Errorf("adopt %s in %s: %w", r.Skill, from.ID, err)
		}
		if !s.Valid {
			if !literal[r.Skill] {
				actions = append(actions, Action{Op: OpSkip, Skill: r.Skill, Agent: from.ID, Reason: "invalid"})
				continue
			}
			return nil, fmt.Errorf("adopt %s in %s: invalid skill: %s", r.Skill, from.ID, strings.Join(s.Problems, "; "))
		}
		present, err := pathExists(v.SkillDir(r.Skill))
		if err != nil {
			return nil, fmt.Errorf("adopt %s in %s: %w", r.Skill, from.ID, err)
		}
		if present && !opts.Force {
			refused = append(refused, r.Skill+" in "+from.ID+": already in the vault")
			reasons = addReason(reasons, "already in the vault")
			continue
		}
		adopts = append(adopts, s)
		actions = append(actions, Action{Op: OpAdopt, Skill: r.Skill, Agent: from.ID, Path: dir})
	}
	if len(refused) > 0 {
		return nil, fmt.Errorf("adopt: %w", forceRefusal("refusing to adopt "+strings.Join(refused, "; ")+" (use --force)", reasons...))
	}

	locks := make([]lockWrite, 0, len(adopts))
	for _, s := range adopts {
		locks = append(locks, lockWrite{dir: dirs[from.ID], skill: s})
	}
	for _, s := range adopts {
		for _, r := range rows {
			if r.Skill != s.Name || r.Agent == from.ID || r.Status != Unmanaged {
				continue
			}
			dir := filepath.Join(dirs[r.Agent], s.Name)
			h, err := hashdir.Hash(dir)
			if err != nil {
				return nil, fmt.Errorf("adopt %s in %s: %w", s.Name, r.Agent, err)
			}
			if h != s.Hash {
				continue
			}
			actions = append(actions, Action{Op: OpAdopt, Skill: s.Name, Agent: r.Agent, Path: dir})
			locks = append(locks, lockWrite{dir: dirs[r.Agent], skill: s})
		}
	}
	if opts.DryRun {
		return actions, nil
	}

	copies := make([]skillCopy, 0, len(adopts))
	for _, s := range adopts {
		copies = append(copies, skillCopy{src: s.Dir, dest: v.SkillDir(s.Name)})
	}
	now := time.Now().UTC()
	if err := placeSkills(copies, func() error { return recordLocks(locks, now) }); err != nil {
		return nil, fmt.Errorf("adopt: %w", err)
	}
	return actions, nil
}

// lockWrite is a skill to record in the lock of the agent directory dir.
type lockWrite struct {
	dir   string
	skill vault.Skill
}

// recordLocks records each skill in the lock of its agent directory, as installed at
// now. When a write fails, the locks already written get their old contents back, and
// a lock file the call created is removed, so the records are all or nothing.
func recordLocks(writes []lockWrite, now time.Time) error {
	type saved struct {
		dir     string
		existed bool
		lock    target.Lock
	}
	var done []saved
	restore := func(err error) error {
		for _, s := range slices.Backward(done) {
			if !s.existed {
				_ = target.RemoveLock(s.dir)
				continue
			}
			_ = target.WriteLock(s.dir, s.lock)
		}
		return err
	}
	for _, w := range writes {
		existed, err := target.LockExists(w.dir)
		if err != nil {
			return restore(err)
		}
		prev, err := target.ReadLock(w.dir)
		if err != nil {
			return restore(err)
		}
		done = append(done, saved{dir: w.dir, existed: existed, lock: target.Lock{Version: prev.Version, Skills: maps.Clone(prev.Skills)}})
		if err := recordLock(w.dir, w.skill, now); err != nil {
			return restore(err)
		}
	}
	return nil
}

// UnmanagedCopy is an unmanaged skill folder in an agent, as the ADOPT picker lists
// it. Conflict is set on every copy of a name that is unmanaged in several agents
// with different hashes.
type UnmanagedCopy struct {
	Skill    string
	Agent    string
	Path     string
	Hash     string
	Conflict bool
}

// UnmanagedCopies lists the unmanaged skill folders of agents, by skill name and then
// agent order, each with its hash.
func UnmanagedCopies(v *vault.Vault, agents []config.Agent) ([]UnmanagedCopy, error) {
	rows, err := Status(v, agents, Filter{})
	if err != nil {
		return nil, fmt.Errorf("unmanaged: %w", err)
	}
	dirs := agentDirs(agents)
	var copies []UnmanagedCopy
	hashes := make(map[string]map[string]bool)
	for _, r := range rows {
		if r.Status != Unmanaged {
			continue
		}
		dir := filepath.Join(dirs[r.Agent], r.Skill)
		h, err := hashdir.Hash(dir)
		if err != nil {
			return nil, fmt.Errorf("unmanaged %s in %s: %w", r.Skill, r.Agent, err)
		}
		copies = append(copies, UnmanagedCopy{Skill: r.Skill, Agent: r.Agent, Path: dir, Hash: h})
		if hashes[r.Skill] == nil {
			hashes[r.Skill] = make(map[string]bool)
		}
		hashes[r.Skill][h] = true
	}
	for i := range copies {
		copies[i].Conflict = len(hashes[copies[i].Skill]) > 1
	}
	return copies, nil
}
