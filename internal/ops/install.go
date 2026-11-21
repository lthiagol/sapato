package ops

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"sapato/internal/config"
	"sapato/internal/hashdir"
	"sapato/internal/target"
	"sapato/internal/vault"
)

// Operation names, as recorded in Action.Op.
const (
	OpInstall   = "install"
	OpUninstall = "uninstall"
	OpUpdate    = "update"
	OpImport    = "import"
	OpAdopt     = "adopt"
	OpRemove    = "remove"
	OpSkip      = "skip"
)

// Options are the flags of the mutating operations.
type Options struct {
	// Force overwrites an unmanaged or modified folder on install, and removes
	// or overwrites a modified one on uninstall and update. Unmanaged folders
	// are never removed.
	Force bool
	// DryRun returns the plan and changes nothing.
	DryRun bool
}

// Action is one planned or applied change to an agent's skills directory.
type Action struct {
	Op     string
	Skill  string
	Agent  string
	Path   string // the skill folder in the agent's directory; empty for a skip
	Reason string // why a skip was made; empty otherwise
}

// Install copies the vault skills matched by skills into each agent (spec §7,
// §8). A skill named literally must be valid. A glob that matches an invalid
// skill skips it, and the skip is returned as an OpSkip action. A folder that
// is unmanaged or modified is refused unless opts.Force is set. When a check
// fails nothing is changed.
func Install(v *vault.Vault, agents []config.Agent, skills []string, opts Options) ([]Action, error) {
	if err := checkArgs(agents, skills); err != nil {
		return nil, fmt.Errorf("install: %w", err)
	}
	vaultSkills, err := v.Skills()
	if err != nil {
		return nil, fmt.Errorf("install: %w", err)
	}
	byName := make(map[string]vault.Skill, len(vaultSkills))
	names := make([]string, 0, len(vaultSkills))
	for _, s := range vaultSkills {
		byName[s.Name] = s
		names = append(names, s.Name)
	}
	if err := checkMatched(skills, names); err != nil {
		return nil, fmt.Errorf("install: %w", err)
	}

	literal := literalNames(skills)
	var invalid []string
	for _, s := range vaultSkills {
		if !s.Valid && literal[s.Name] {
			invalid = append(invalid, fmt.Sprintf("%s (%s)", s.Name, strings.Join(s.Problems, "; ")))
		}
	}
	if len(invalid) > 0 {
		return nil, fmt.Errorf("install: invalid skills are never installed: %s", strings.Join(invalid, ", "))
	}

	rows, err := Status(v, agents, Filter{Skills: skills})
	if err != nil {
		return nil, fmt.Errorf("install: %w", err)
	}
	dirs := agentDirs(agents)
	var actions []Action
	var refused []string
	for _, r := range rows {
		s, ok := byName[r.Skill]
		if !ok {
			continue
		}
		if !s.Valid {
			actions = append(actions, Action{Op: OpSkip, Skill: r.Skill, Agent: r.Agent, Reason: "invalid"})
			continue
		}
		if (r.Status == Unmanaged || r.Status == Modified) && !opts.Force {
			refused = append(refused, fmt.Sprintf("%s in %s: %s", r.Skill, r.Agent, r.Status))
			continue
		}
		actions = append(actions, Action{Op: OpInstall, Skill: r.Skill, Agent: r.Agent, Path: filepath.Join(dirs[r.Agent], r.Skill)})
	}
	if len(refused) > 0 {
		return nil, fmt.Errorf("install: refusing to overwrite %s (use --force)", strings.Join(refused, "; "))
	}
	if opts.DryRun {
		return actions, nil
	}
	if err := applyActions(actions, byName, dirs); err != nil {
		return nil, err
	}
	return actions, nil
}

// applyActions installs the skill of each action that is not a skip. The
// actions come from a plan that already passed its checks.
func applyActions(actions []Action, byName map[string]vault.Skill, dirs map[string]string) error {
	now := time.Now().UTC()
	for _, a := range actions {
		if a.Op == OpSkip {
			continue
		}
		if err := installSkill(dirs[a.Agent], byName[a.Skill], now); err != nil {
			return fmt.Errorf("%s %s in %s: %w", a.Op, a.Skill, a.Agent, err)
		}
	}
	return nil
}

// installSkill copies s into the agent directory dir and records it in the lock.
// The copy is staged in a hidden folder and renamed into place (spec §8).
func installSkill(dir string, s vault.Skill, now time.Time) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := placeSkill(s.Dir, filepath.Join(dir, s.Name)); err != nil {
		return err
	}
	return recordLock(dir, s, now)
}

// recordLock adds s to the lock of the agent directory dir, as installed at now.
func recordLock(dir string, s vault.Skill, now time.Time) error {
	lock, err := target.ReadLock(dir)
	if err != nil {
		return err
	}
	lock.Skills[s.Name] = target.LockEntry{Hash: s.Hash, Version: s.Version, InstalledAt: now}
	return target.WriteLock(dir, lock)
}

// placeSkill copies the skill folder src to dest, replacing what dest holds.
// The copy is staged in a hidden sibling folder and renamed into place.
func placeSkill(src, dest string) error {
	tmp := filepath.Join(filepath.Dir(dest), "."+filepath.Base(dest)+".sapato-tmp")
	if err := stageSkill(src, tmp); err != nil {
		return err
	}
	if err := os.RemoveAll(dest); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("remove old copy: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("rename into place: %w", err)
	}
	return nil
}

// stageSkill copies the skill folder src into a new folder at tmp. On failure
// tmp is removed.
func stageSkill(src, tmp string) (err error) {
	defer func() {
		if err != nil {
			_ = os.RemoveAll(tmp)
		}
	}()
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	files, err := hashdir.Files(src)
	if err != nil {
		return err
	}
	if err := os.Mkdir(tmp, 0o755); err != nil {
		return err
	}
	for _, f := range files {
		dst := filepath.Join(tmp, filepath.FromSlash(f.Rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := copyFile(f.Path, dst, f.Mode); err != nil {
			return fmt.Errorf("copy %s: %w", f.Path, err)
		}
	}
	return nil
}

// copyFile is copyRegular, held in a variable so tests can fail a copy midway.
var copyFile = copyRegular

func copyRegular(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	// The umask may have stripped bits at create time.
	return os.Chmod(dst, mode)
}

// checkAgents rejects a call with no agents.
func checkAgents(agents []config.Agent) error {
	if len(agents) == 0 {
		return errors.New("no agents selected")
	}
	return nil
}

// checkArgs rejects a call with no agents or no skill arguments.
func checkArgs(agents []config.Agent, skills []string) error {
	if err := checkAgents(agents); err != nil {
		return err
	}
	if len(skills) == 0 {
		return errors.New("no skills given")
	}
	return nil
}

// literalNames returns the args that name a skill exactly, rather than by a glob.
func literalNames(args []string) map[string]bool {
	literal := make(map[string]bool, len(args))
	for _, a := range args {
		if !strings.ContainsAny(a, `*?[\`) {
			literal[a] = true
		}
	}
	return literal
}

// checkMatched fails when a glob is malformed or matches none of names.
func checkMatched(globs, names []string) error {
	for _, g := range globs {
		if _, err := path.Match(g, ""); err != nil {
			return fmt.Errorf("bad skill pattern %q: %w", g, err)
		}
		if !slices.ContainsFunc(names, func(n string) bool { return matchesAny([]string{g}, n) }) {
			return fmt.Errorf("no skill matches %q", g)
		}
	}
	return nil
}

// agentDirs maps each agent id to its skills directory.
func agentDirs(agents []config.Agent) map[string]string {
	dirs := make(map[string]string, len(agents))
	for _, a := range agents {
		dirs[a.ID] = a.Path
	}
	return dirs
}
