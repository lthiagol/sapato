package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"sapato/internal/hashdir"
)

const (
	skillFileName  = "SKILL.md"
	maxNameLen     = 64
	maxDescription = 1024
)

var nameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Skill is a skill folder in the vault.
type Skill struct {
	Name        string
	Dir         string
	Description string
	Version     string
	Hash        string
	Valid       bool
	Problems    []string
	// Compat is the compatible list of the skill, nil when it is compatible with
	// every agent. Vault.Skills sets it; Load does not.
	Compat []string
}

// Load reads and validates the skill folder dir as a skill called name. The
// name is checked against the frontmatter, not against the folder name of dir.
func Load(dir, name string) (Skill, error) {
	hash, err := hashdir.Hash(dir)
	if err != nil {
		return Skill{}, err
	}
	s := Skill{Name: name, Dir: dir, Hash: hash}

	path := filepath.Join(dir, skillFileName)
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		s.Problems = append(s.Problems, skillFileName+" is missing")
	case err != nil:
		return Skill{}, fmt.Errorf("stat %s: %w", path, err)
	case !info.Mode().IsRegular():
		s.Problems = append(s.Problems, skillFileName+" is not a regular file")
	default:
		data, err := os.ReadFile(path)
		if err != nil {
			return Skill{}, fmt.Errorf("read %s: %w", path, err)
		}
		s.validate(data)
	}
	s.Valid = len(s.Problems) == 0
	return s, nil
}

// ParseContent validates data as the SKILL.md of a skill that is not in a vault
// yet. The skill takes the name in its frontmatter, so no folder name is checked.
// Dir and Hash are left empty.
func ParseContent(data []byte) Skill {
	s := Skill{Name: contentName(data)}
	s.validate(data)
	s.Valid = len(s.Problems) == 0
	return s
}

// contentName returns the name in the frontmatter of data, or "" if there is none.
func contentName(data []byte) string {
	fm, err := parseFrontmatter(data)
	if err != nil {
		return ""
	}
	name, _ := fm["name"].(string)
	return name
}

// FrontmatterName returns the name in the frontmatter of the SKILL.md in dir.
// It returns "" when the name is missing or not a string.
func FrontmatterName(dir string) (string, error) {
	path := filepath.Join(dir, skillFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	fm, err := parseFrontmatter(data)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	name, _ := fm["name"].(string)
	return name, nil
}

// validate applies the spec §3 rules to the contents of SKILL.md.
func (s *Skill) validate(data []byte) {
	fm, err := parseFrontmatter(data)
	if err != nil {
		s.Problems = append(s.Problems, "frontmatter: "+err.Error())
		return
	}

	name, _ := fm["name"].(string)
	description, _ := fm["description"].(string)
	meta, _ := fm["metadata"].(map[string]any)
	s.Description = description
	s.Version, _ = meta["version"].(string)

	switch {
	case name == "":
		s.Problems = append(s.Problems, "frontmatter name must be a non-empty string")
	case name != s.Name:
		s.Problems = append(s.Problems, fmt.Sprintf("name %q does not match folder %q", name, s.Name))
	}
	if name != "" {
		if !nameRe.MatchString(name) {
			s.Problems = append(s.Problems, fmt.Sprintf("name %q must match %s", name, nameRe))
		}
		if n := utf8.RuneCountInString(name); n > maxNameLen {
			s.Problems = append(s.Problems, fmt.Sprintf("name is %d characters, max %d", n, maxNameLen))
		}
	}

	switch n := utf8.RuneCountInString(description); {
	case description == "":
		s.Problems = append(s.Problems, "frontmatter description must be a non-empty string")
	case n > maxDescription:
		s.Problems = append(s.Problems, fmt.Sprintf("description is %d characters, max %d", n, maxDescription))
	}
}

// parseFrontmatter returns the YAML mapping between the leading and closing "---" lines.
func parseFrontmatter(data []byte) (map[string]any, error) {
	lines := strings.Split(string(data), "\n")
	if !isDelimiter(lines[0]) {
		return nil, errors.New("missing opening --- line")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if isDelimiter(lines[i]) {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, errors.New("missing closing --- line")
	}

	var fm map[string]any
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &fm); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	return fm, nil
}

func isDelimiter(line string) bool {
	return strings.TrimRight(line, " \t\r") == "---"
}
