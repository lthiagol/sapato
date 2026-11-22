// Package vault opens and creates skill vaults and validates the skills in them.
package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	manifestName = "sapato.toml"
	skillsDir    = "skills"
	formatV1     = 1
)

// Vault is an opened vault.
type Vault struct {
	Path        string
	Assignments map[string][]string
	// compat holds the compatible list of each skill that has a [skills.<name>]
	// table (spec §3). A skill without one is compatible with every agent.
	compat map[string][]string
}

type manifest struct {
	Format      *int                      `toml:"format"`
	Assignments map[string][]string       `toml:"assignments"`
	Skills      map[string]map[string]any `toml:"skills"`
}

// Open reads and checks the vault manifest at path.
func Open(path string) (*Vault, error) {
	data, err := os.ReadFile(filepath.Join(path, manifestName))
	if err != nil {
		return nil, fmt.Errorf("open vault %s: %w", path, err)
	}
	assignments, compat, err := parseManifest(path, data)
	if err != nil {
		return nil, err
	}
	return &Vault{Path: path, Assignments: assignments, compat: compat}, nil
}

// parseManifest checks the contents data of the manifest of the vault at path and
// returns its assignments and compatible lists.
func parseManifest(path string, data []byte) (map[string][]string, map[string][]string, error) {
	var m manifest
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", filepath.Join(path, manifestName), err)
	}
	if m.Format == nil {
		return nil, nil, fmt.Errorf("vault %s: format is required", path)
	}
	if *m.Format != formatV1 {
		return nil, nil, fmt.Errorf("vault %s: unsupported format %d", path, *m.Format)
	}
	compat, err := parseCompat(path, m.Skills)
	if err != nil {
		return nil, nil, err
	}
	return m.Assignments, compat, nil
}

// Exists reports whether a vault manifest is present at path.
func Exists(path string) (bool, error) {
	_, err := os.Stat(filepath.Join(path, manifestName))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("check vault %s: %w", path, err)
	}
}

// Init creates a vault at path. It fails if a vault manifest already exists.
func Init(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("init vault %s: %w", path, err)
	}
	f, err := os.OpenFile(filepath.Join(path, manifestName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("init vault %s: %s already exists", path, manifestName)
	}
	if err != nil {
		return fmt.Errorf("init vault %s: %w", path, err)
	}
	if _, err := fmt.Fprintf(f, "format = %d\n", formatV1); err != nil {
		f.Close()
		return fmt.Errorf("init vault %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("init vault %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Join(path, skillsDir), 0o755); err != nil {
		return fmt.Errorf("init vault %s: %w", path, err)
	}
	return nil
}

// SkillDir returns the folder of the skill called name. It does not check that
// the folder exists.
func (v *Vault) SkillDir(name string) string {
	return filepath.Join(v.Path, skillsDir, name)
}

// Skills returns every skill directory in the vault, sorted by name.
// Invalid skills are included with Valid false and their Problems set.
func (v *Vault) Skills() ([]Skill, error) {
	root := filepath.Join(v.Path, skillsDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read skills in %s: %w", v.Path, err)
	}
	var skills []Skill
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		s, err := Load(filepath.Join(root, e.Name()), e.Name())
		if err != nil {
			return nil, err
		}
		skills = append(skills, s)
	}
	slices.SortFunc(skills, func(a, b Skill) int { return strings.Compare(a.Name, b.Name) })
	return skills, nil
}
