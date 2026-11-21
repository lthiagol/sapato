package vault

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"sapato/internal/testfixtures"
)

func TestSkillDBFixture(t *testing.T) {
	v, err := Open(filepath.Join(testfixtures.Root(), "skill-db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	skills, err := v.Skills()
	if err != nil {
		t.Fatalf("Skills() error = %v", err)
	}

	// Sorted by name; stray.txt and .trash are not skills.
	tests := []struct {
		name        string
		valid       bool
		wantProblem string // substring of the only problem; empty when valid
		wantVersion string
	}{
		{name: "Invalid-Uppercase", wantProblem: "must match"},
		{name: "invalid-long-description", wantProblem: "description is 1025 characters, max 1024"},
		{name: "invalid-missing-description", wantProblem: "frontmatter description must be a non-empty string"},
		{name: "invalid-name-mismatch", wantProblem: `name "something-else" does not match folder "invalid-name-mismatch"`},
		{name: "invalid-no-frontmatter", wantProblem: "frontmatter: missing opening --- line"},
		{name: "invalid-no-skill-md", wantProblem: "SKILL.md is missing"},
		{name: "mocked-skill-00001", valid: true},
		{name: "mocked-skill-00002", valid: true, wantVersion: "1.2.0"},
		{name: "mocked-skill-00003", valid: true},
		{name: "mocked-skill-00004", valid: true},
		{name: "mocked-skill-00005", valid: true},
		{name: "mocked-skill-00006", valid: true},
	}

	var names []string
	for _, tt := range tests {
		names = append(names, tt.name)
	}
	var got []string
	for _, s := range skills {
		got = append(got, s.Name)
	}
	if !reflect.DeepEqual(got, names) {
		t.Fatalf("skills = %v, want %v", got, names)
	}

	for i, tt := range tests {
		s := skills[i]
		t.Run(tt.name, func(t *testing.T) {
			if s.Valid != tt.valid {
				t.Errorf("Valid = %v, want %v (problems %v)", s.Valid, tt.valid, s.Problems)
			}
			if tt.wantProblem == "" {
				if len(s.Problems) != 0 {
					t.Errorf("Problems = %v, want none", s.Problems)
				}
			} else if len(s.Problems) != 1 || !strings.Contains(s.Problems[0], tt.wantProblem) {
				t.Errorf("Problems = %v, want one containing %q", s.Problems, tt.wantProblem)
			}
			if s.Version != tt.wantVersion {
				t.Errorf("Version = %q, want %q", s.Version, tt.wantVersion)
			}
		})
	}
}

func TestSkillMDSymlinkIsNotRegularFile(t *testing.T) {
	dir := testfixtures.CopyTree(t, filepath.Join(testfixtures.Root(), "skill-db"))
	skillMD := filepath.Join(dir, "skills", "mocked-skill-00001", "SKILL.md")
	if err := os.Remove(skillMD); err != nil {
		t.Fatal(err)
	}
	// Point at a real, valid-looking SKILL.md so only rule 1 can fail.
	if err := os.Symlink("../mocked-skill-00002/SKILL.md", skillMD); err != nil {
		t.Fatal(err)
	}

	v, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	skills, err := v.Skills()
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(skills, func(s Skill) bool { return s.Name == "mocked-skill-00001" })
	if i < 0 {
		t.Fatal("mocked-skill-00001 not listed")
	}
	s := skills[i]
	if s.Valid || len(s.Problems) != 1 || !strings.Contains(s.Problems[0], "not a regular file") {
		t.Errorf("Valid = %v, Problems = %v; want one 'not a regular file' problem", s.Valid, s.Problems)
	}
}
