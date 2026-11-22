package ops

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

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
	return place(v, s.Name, content, opts, nil)
}

// Scaffold adds a new skill called name to the vault, written from the template of
// spec §7.2 with description as its description. compatible is the compatible list
// (spec §3): nil makes the skill compatible with every agent and removes any table it
// had, so callers pass nil when every configured agent is listed. The result is
// validated as Create validates content, and a name already in the vault is refused
// unless opts.Force is set. When a check fails nothing is changed.
func Scaffold(v *vault.Vault, name, description string, compatible []string, opts Options) (Action, error) {
	content, err := scaffoldContent(name, description)
	if err != nil {
		return Action{}, fmt.Errorf("create %s: %w", name, err)
	}
	s := vault.ParseContent(content)
	if !s.Valid {
		return Action{}, fmt.Errorf("create: invalid skill: %s", strings.Join(s.Problems, "; "))
	}
	var list []string
	if compatible != nil {
		if list, err = compatList(compatible); err != nil {
			return Action{}, fmt.Errorf("create %s: %w", name, err)
		}
	}
	commit := func() error { return v.EditCompat(map[string][]string{name: list}) }
	return place(v, name, content, opts, commit)
}

// scaffoldContent returns the SKILL.md of the template of spec §7.2. The name and the
// description are YAML-quoted as needed.
func scaffoldContent(name, description string) ([]byte, error) {
	type frontmatter struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	var fm bytes.Buffer
	enc := yaml.NewEncoder(&fm)
	enc.SetIndent(2)
	if err := enc.Encode(frontmatter{Name: name, Description: description}); err != nil {
		return nil, fmt.Errorf("write frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("write frontmatter: %w", err)
	}
	return fmt.Appendf(nil, `---
%s---

# %s

<!-- One or two sentences: what this skill does. -->

## When to use

<!-- The situations that should trigger this skill. -->

## Steps

1. <!-- What the agent does, in order. -->

## Notes

<!-- Constraints, gotchas, files in this folder (scripts/, references/, assets/). -->
`, fm.String(), name), nil
}

// place adds content as the new skill folder name in the vault. A name already in
// the vault is refused unless opts.Force is set. When commit is not nil it runs once
// the folder is in place, and a failure puts the old state back (see placeSkillThen).
func place(v *vault.Vault, name string, content []byte, opts Options, commit func() error) (Action, error) {
	dest := v.SkillDir(name)
	present, err := pathExists(dest)
	if err != nil {
		return Action{}, fmt.Errorf("create %s: %w", name, err)
	}
	if present && !opts.Force {
		return Action{}, fmt.Errorf("create %s: %w", name, forceRefusal("already in the vault (use --force)", "already in the vault"))
	}

	action := Action{Op: OpCreate, Skill: name, Path: dest}
	if opts.DryRun {
		return action, nil
	}
	src, cleanup, err := stageSkillFile(name, content)
	if err != nil {
		return Action{}, fmt.Errorf("create %s: %w", name, err)
	}
	defer cleanup()
	if err := placeSkillThen(src, dest, commit); err != nil {
		return Action{}, fmt.Errorf("create %s: %w", name, err)
	}
	return action, nil
}

// EditorCommand returns the command line that opens path in an editor: $VISUAL, else
// $EDITOR, else vi (spec §7). The editor may hold arguments, which stay in front of
// the path.
func EditorCommand(visual, editor, path string) []string {
	for _, e := range []string{visual, editor} {
		if f := strings.Fields(e); len(f) > 0 {
			return append(f, path)
		}
	}
	return []string{"vi", path}
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
