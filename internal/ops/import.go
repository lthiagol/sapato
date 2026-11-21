package ops

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"sapato/internal/vault"
)

// ImportOptions are the flags of Import.
type ImportOptions struct {
	Options
	// Name is the skill name. For a folder or git source it must match the
	// frontmatter name. For a tarball it is allowed only with a single-skill archive.
	Name string
	// Subdir is the folder of the source that holds the skill. It does not apply to a tarball.
	Subdir string
}

// Import copies skills from src into the vault (spec §7, §11). src is a gzip
// tarball, a local folder, or a git URL that is shallow-cloned to a temp dir first.
// A skill takes the name in its frontmatter. It must be valid, and a name already in
// the vault is refused unless opts.Force is set. When a check fails nothing is changed.
func Import(v *vault.Vault, src string, opts ImportOptions) ([]Action, error) {
	if isArchive(src) {
		actions, err := importArchive(v, src, opts)
		if err != nil {
			return nil, fmt.Errorf("import %s: %w", src, err)
		}
		return actions, nil
	}
	action, err := importFolder(v, src, opts)
	if err != nil {
		return nil, err
	}
	return []Action{action}, nil
}

// importFolder copies the skill folder at src, or in a git clone of src, into the vault.
func importFolder(v *vault.Vault, src string, opts ImportOptions) (Action, error) {
	root, cleanup, err := fetchSource(src)
	if err != nil {
		return Action{}, fmt.Errorf("import %s: %w", src, err)
	}
	defer cleanup()

	dir, err := skillDir(root, opts.Subdir)
	if err != nil {
		return Action{}, fmt.Errorf("import %s: %w", src, err)
	}
	name, err := vault.FrontmatterName(dir)
	if err != nil {
		return Action{}, fmt.Errorf("import %s: %w", src, err)
	}
	if opts.Name != "" && opts.Name != name {
		return Action{}, fmt.Errorf("import %s: name %q does not match frontmatter name %q", src, opts.Name, name)
	}
	s, err := vault.Load(dir, name)
	if err != nil {
		return Action{}, fmt.Errorf("import %s: %w", src, err)
	}
	if !s.Valid {
		return Action{}, fmt.Errorf("import %s: invalid skill: %s", src, strings.Join(s.Problems, "; "))
	}

	dest := v.SkillDir(name)
	present, err := pathExists(dest)
	if err != nil {
		return Action{}, fmt.Errorf("import %s: %w", src, err)
	}
	if present && !opts.Force {
		return Action{}, fmt.Errorf("import %s: %w", src, forceRefusal(name+" is already in the vault (use --force)"))
	}

	action := Action{Op: OpImport, Skill: name, Path: dest}
	if opts.DryRun {
		return action, nil
	}
	if err := placeSkillThen(dir, dest, nil); err != nil {
		return Action{}, fmt.Errorf("import %s: %w", src, err)
	}
	return action, nil
}

// isGitURL reports whether src names a git repository rather than a local folder.
func isGitURL(src string) bool {
	return strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "git@") || strings.HasSuffix(src, ".git")
}

// fetchSource returns the folder to read the skill from. A local src is resolved
// through its symlinks. A git URL is a shallow clone in a new temp dir, and
// cleanup removes that dir. The clone never prompts for credentials and never
// runs an ext:: helper.
func fetchSource(src string) (root string, cleanup func(), err error) {
	if !isGitURL(src) {
		resolved, err := filepath.EvalSymlinks(src)
		if err != nil {
			return "", nil, err
		}
		return resolved, func() {}, nil
	}
	tmp, err := os.MkdirTemp("", "sapato-import-")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(tmp) }
	root = filepath.Join(tmp, "repo")
	// -c wins over the user's git config, so ext:: stays refused even where
	// the config allows it.
	cmd := exec.Command("git", "-c", "protocol.ext.allow=never", "clone", "--depth", "1", "--", src, root)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("git clone: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return root, cleanup, nil
}

// skillDir returns the folder of the skill inside root. subdir must stay inside root.
func skillDir(root, subdir string) (string, error) {
	if subdir == "" {
		return root, nil
	}
	if !filepath.IsLocal(subdir) {
		return "", fmt.Errorf("subdir %q must be a path inside the source", subdir)
	}
	return filepath.Join(root, subdir), nil
}

// pathExists reports whether anything, even a broken symlink, is at path.
func pathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}
