package ops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"sapato/internal/vault"
)

// ImportOptions are the flags of Import.
type ImportOptions struct {
	Options
	// Name is the skill name. Without Fix it must match the frontmatter name; with
	// Fix it renames the skill (spec §7.1). It does not apply with All.
	Name string
	// Subdir is the folder of the source that holds the skill. It does not apply to a tarball.
	Subdir string
	// All imports every skill the source holds, all or nothing.
	All bool
	// Fix repairs the frontmatter of an invalid skill when --fix can (spec §7.1).
	// It does not apply to a tarball.
	Fix bool
	// Description is the description a repaired skill takes when it has none.
	Description string
}

// importItem is a skill to copy into the vault.
type importItem struct {
	name string
	src  string // the folder to copy
}

// Import copies skills from src into the vault (spec §7, §7.1, §11). A skill takes
// the name in its frontmatter. It must be valid, or repairable with Fix, and a name
// already in the vault is refused unless opts.Force is set. When a check fails
// nothing is changed.
func Import(v *vault.Vault, src string, opts ImportOptions) ([]Action, error) {
	if opts.All && opts.Name != "" {
		return nil, fmt.Errorf("import %s: --name does not apply with --all", src)
	}
	if isArchive(src) {
		if opts.Fix || opts.Description != "" {
			return nil, fmt.Errorf("import %s: --fix and --description do not apply to a tarball", src)
		}
		actions, err := importArchive(v, src, opts)
		if err != nil {
			return nil, fmt.Errorf("import %s: %w", src, err)
		}
		return actions, nil
	}

	cands, cleanup, err := Discover(src, opts.Subdir)
	defer cleanup()
	if err != nil {
		return nil, fmt.Errorf("import %s: %w", src, err)
	}
	chosen, err := chooseCandidates(cands, opts.All)
	if err != nil {
		return nil, fmt.Errorf("import %s: %w", src, err)
	}

	work, err := os.MkdirTemp("", "sapato-import-")
	if err != nil {
		return nil, fmt.Errorf("import %s: %w", src, err)
	}
	defer func() { _ = os.RemoveAll(work) }()

	var items []importItem
	for _, c := range chosen {
		it, err := prepareItem(c, opts, work)
		if err != nil {
			return nil, fmt.Errorf("import %s: %w", src, err)
		}
		if slices.ContainsFunc(items, func(o importItem) bool { return o.name == it.name }) {
			return nil, fmt.Errorf("import %s: two skills take the name %q", src, it.name)
		}
		items = append(items, it)
	}

	var copies []skillCopy
	var actions []Action
	var present []string
	for _, it := range items {
		dest := v.SkillDir(it.name)
		exists, err := pathExists(dest)
		if err != nil {
			return nil, fmt.Errorf("import %s: %w", src, err)
		}
		if exists {
			present = append(present, it.name)
		}
		copies = append(copies, skillCopy{src: it.src, dest: dest})
		actions = append(actions, Action{Op: OpImport, Skill: it.name, Path: dest})
	}
	if len(present) > 0 && !opts.Force {
		return nil, fmt.Errorf("import %s: %w", src, presentRefusal(present))
	}
	if opts.DryRun {
		return actions, nil
	}
	if err := placeSkills(copies, nil); err != nil {
		return nil, fmt.Errorf("import %s: %w", src, err)
	}
	return actions, nil
}

// candidateWhere names the folder of c in an error, or nothing when c is the root.
func candidateWhere(c Candidate) string {
	if c.Rel == "." {
		return ""
	}
	return " for " + c.Rel
}

// presentRefusal refuses to replace the skills called names, which are in the vault.
func presentRefusal(names []string) error {
	if len(names) == 1 {
		return forceRefusal(names[0]+" is already in the vault (use --force)", "already in the vault")
	}
	return forceRefusal("already in the vault (use --force): "+strings.Join(names, ", "), "already in the vault: "+strings.Join(names, ", "))
}

// chooseCandidates returns the candidates to import. With all, every candidate.
// Otherwise the source must hold exactly one.
func chooseCandidates(cands []Candidate, all bool) ([]Candidate, error) {
	switch {
	case len(cands) == 0:
		return nil, errors.New("no skill found")
	case all || len(cands) == 1:
		return cands, nil
	}
	rels := make([]string, len(cands))
	for i, c := range cands {
		rels[i] = c.Rel
	}
	return nil, fmt.Errorf("several skills found (%s): pick one with --subdir or import all with --all", strings.Join(rels, ", "))
}

// prepareItem checks candidate c for import. A skill whose name differs from --name
// is refused unless opts.Fix renames it, and an invalid skill is refused unless
// opts.Fix repairs it. A repaired copy is staged in a new folder under work, and the
// source is never changed.
func prepareItem(c Candidate, opts ImportOptions, work string) (importItem, error) {
	renamed := opts.Name != "" && opts.Name != c.Name
	if c.Valid && !renamed {
		return importItem{name: c.Name, src: c.dir}, nil
	}
	if !opts.Fix {
		if c.Valid {
			return importItem{}, fmt.Errorf("name %q does not match frontmatter name %q; --fix renames the skill", opts.Name, c.Name)
		}
		msg := "invalid skill: " + strings.Join(c.Problems, "; ")
		if c.Fixable {
			msg += "; --fix can repair it"
		}
		return importItem{}, errors.New(msg)
	}
	if !c.Valid && !c.Fixable {
		return importItem{}, errors.New("invalid skill: " + strings.Join(c.Problems, "; "))
	}

	name := opts.Name
	if name == "" {
		name = c.Name
		if !vault.ValidName(name) {
			name = NormalizeName(c.Hint)
		}
	}
	if name == "" {
		return importItem{}, fmt.Errorf("no name can be derived%s: pass --name", candidateWhere(c))
	}
	data, err := os.ReadFile(filepath.Join(c.dir, skillFileName))
	if err != nil {
		return importItem{}, err
	}
	if vault.ParseContent(data).Description == "" && opts.Description == "" {
		return importItem{}, fmt.Errorf("description is missing%s: pass --description", candidateWhere(c))
	}
	repaired, err := vault.Repair(data, name, opts.Description)
	if err != nil {
		return importItem{}, err
	}
	if s := vault.ParseFolder(repaired, name); !s.Valid {
		return importItem{}, fmt.Errorf("invalid skill after --fix: %s", strings.Join(s.Problems, "; "))
	}

	stage, err := os.MkdirTemp(work, "skill-")
	if err != nil {
		return importItem{}, err
	}
	dir := filepath.Join(stage, name)
	if err := stageSkill(c.dir, dir); err != nil {
		return importItem{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, skillFileName), repaired, 0o644); err != nil {
		return importItem{}, err
	}
	return importItem{name: name, src: dir}, nil
}
