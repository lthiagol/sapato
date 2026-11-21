package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sapato/internal/vault"
)

// Create adds content, a complete SKILL.md, to the vault as a new skill (spec §7).
// The skill takes the name in its frontmatter and must be valid. A name already in
// the vault is refused unless opts.Force is set. When a check fails nothing is changed.
func Create(v *vault.Vault, content []byte, opts Options) (Action, error) {
	s := vault.ParseContent(content)
	if !s.Valid {
		return Action{}, fmt.Errorf("create: invalid skill: %s", strings.Join(s.Problems, "; "))
	}

	dest := v.SkillDir(s.Name)
	present, err := pathExists(dest)
	if err != nil {
		return Action{}, fmt.Errorf("create %s: %w", s.Name, err)
	}
	if present && !opts.Force {
		return Action{}, fmt.Errorf("create %s: %w", s.Name, forceRefusal("already in the vault (use --force)"))
	}

	action := Action{Op: OpCreate, Skill: s.Name, Path: dest}
	if opts.DryRun {
		return action, nil
	}
	src, cleanup, err := stageSkillFile(s.Name, content)
	if err != nil {
		return Action{}, fmt.Errorf("create %s: %w", s.Name, err)
	}
	defer cleanup()
	if err := placeSkillThen(src, dest, nil); err != nil {
		return Action{}, fmt.Errorf("create %s: %w", s.Name, err)
	}
	return action, nil
}

// stageSkillFile writes content as the SKILL.md of a new folder called name in a
// temp dir. It returns the folder and a cleanup func that removes the temp dir.
// name must be a valid skill name, so it is safe as a path component.
func stageSkillFile(name string, content []byte) (dir string, cleanup func(), err error) {
	tmp, err := os.MkdirTemp("", "sapato-new-")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(tmp) }
	dir = filepath.Join(tmp, name)
	if err := os.Mkdir(dir, 0o755); err != nil {
		cleanup()
		return "", nil, err
	}
	file := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(file, content, 0o644); err != nil {
		cleanup()
		return "", nil, err
	}
	// The umask may have stripped bits at create time.
	if err := os.Chmod(file, 0o644); err != nil {
		cleanup()
		return "", nil, err
	}
	return dir, cleanup, nil
}
