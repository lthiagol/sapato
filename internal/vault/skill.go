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
	skillFileName = "SKILL.md"
	maxNameLen    = 64

	// MaxDescription is the most characters a skill description may have (spec §3 rule 6).
	MaxDescription = 1024
)

var nameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidName reports whether name is a valid skill name (spec §3 rule 5).
func ValidName(name string) bool {
	return nameRe.MatchString(name) && len(name) <= maxNameLen
}

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
		s.validate(data, name)
	}
	s.Valid = len(s.Problems) == 0
	return s, nil
}

// ParseContent validates data as the SKILL.md of a skill that is not in a vault
// yet. The skill takes the name in its frontmatter, so no folder name is checked.
// Dir and Hash are left empty.
func ParseContent(data []byte) Skill {
	return ParseFolder(data, "")
}

// ParseFolder validates data as the SKILL.md of a skill in a folder called folder,
// which the name in the frontmatter must equal. An empty folder has no name to
// match, so only the other rules apply. Name is the frontmatter name. Dir and Hash
// are left empty.
func ParseFolder(data []byte, folder string) Skill {
	s := Skill{Name: contentName(data)}
	s.validate(data, folder)
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

// validate applies the spec §3 rules to the contents of SKILL.md. folder is the
// folder name for rule 4, or "" to skip it.
func (s *Skill) validate(data []byte, folder string) {
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
	case folder != "" && name != folder:
		s.Problems = append(s.Problems, fmt.Sprintf("name %q does not match folder %q", name, folder))
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
	case n > MaxDescription:
		s.Problems = append(s.Problems, fmt.Sprintf("description is %d characters, max %d", n, MaxDescription))
	}
}

// parseFrontmatter returns the YAML mapping between the leading and closing "---" lines.
func parseFrontmatter(data []byte) (map[string]any, error) {
	lines := strings.Split(string(data), "\n")
	if !isDelimiter(lines[0]) {
		return nil, errors.New("missing opening --- line")
	}
	end, err := closingLine(lines)
	if err != nil {
		return nil, err
	}

	var fm map[string]any
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &fm); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	return fm, nil
}

// closingLine returns the index of the "---" line that closes the frontmatter
// whose opening line is lines[0].
func closingLine(lines []string) (int, error) {
	for i := 1; i < len(lines); i++ {
		if isDelimiter(lines[i]) {
			return i, nil
		}
	}
	return 0, errors.New("missing closing --- line")
}

func isDelimiter(line string) bool {
	return strings.TrimRight(line, " \t\r") == "---"
}
