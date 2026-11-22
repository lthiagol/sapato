package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodSkill = "---\nname: demo\ndescription: A demo.\n---\n"

func TestLoad(t *testing.T) {
	tests := []struct {
		name      string
		folder    string
		content   string
		skillName string
		wantValid bool
		wantProb  string
	}{
		{name: "valid", folder: "demo", content: goodSkill, skillName: "demo", wantValid: true},
		{name: "name differs from folder", folder: "other", content: goodSkill, skillName: "demo", wantValid: true},
		{name: "name differs from frontmatter", folder: "demo", content: goodSkill, skillName: "x", wantProb: "does not match folder"},
		{name: "missing SKILL.md", folder: "demo", content: "", skillName: "demo", wantProb: "SKILL.md is missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), tt.folder)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if tt.content != "" {
				writeFile(t, filepath.Join(dir, "SKILL.md"), tt.content)
			}
			s, err := Load(dir, tt.skillName)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if s.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v (problems %v)", s.Valid, tt.wantValid, s.Problems)
			}
			if s.Name != tt.skillName || s.Dir != dir {
				t.Errorf("Name, Dir = %q, %q; want %q, %q", s.Name, s.Dir, tt.skillName, dir)
			}
			if tt.wantProb != "" && !strings.Contains(strings.Join(s.Problems, "\n"), tt.wantProb) {
				t.Errorf("Problems = %v, want one containing %q", s.Problems, tt.wantProb)
			}
		})
	}
}

func TestLoadMissingFolder(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "absent"), "absent"); err == nil {
		t.Error("Load() of a missing folder returned no error")
	}
}

func TestFrontmatterName(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		wantErr bool
	}{
		{name: "name in frontmatter", content: goodSkill, want: "demo"},
		{name: "name not a string", content: "---\nname: [a]\n---\n", want: ""},
		{name: "name missing", content: "---\ndescription: x\n---\n", want: ""},
		{name: "no frontmatter", content: "# Title\n", wantErr: true},
		{name: "no closing delimiter", content: "---\nname: demo\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "SKILL.md"), tt.content)
			got, err := FrontmatterName(dir)
			if (err != nil) != tt.wantErr {
				t.Fatalf("FrontmatterName() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("FrontmatterName() = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("missing SKILL.md", func(t *testing.T) {
		if _, err := FrontmatterName(t.TempDir()); err == nil {
			t.Error("FrontmatterName() with no SKILL.md returned no error")
		}
	})
}

func TestSkillDir(t *testing.T) {
	v := &Vault{Path: "/vault"}
	tests := []struct {
		name  string
		skill string
		want  string
	}{
		{name: "plain name", skill: "code-review", want: filepath.Join("/vault", "skills", "code-review")},
		{name: "name is not checked", skill: "Bad Name", want: filepath.Join("/vault", "skills", "Bad Name")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := v.SkillDir(tt.skill); got != tt.want {
				t.Errorf("SkillDir(%q) = %q, want %q", tt.skill, got, tt.want)
			}
		})
	}
}

func TestValidName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"demo", true},
		{"code-review-2", true},
		{strings.Repeat("a", maxNameLen), true},
		{strings.Repeat("a", maxNameLen+1), false},
		{"", false},
		{"Demo", false},
		{"my skill", false},
		{"-demo", false},
		{"demo-", false},
		{"../demo", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidName(tt.name); got != tt.want {
				t.Errorf("ValidName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestParseContent(t *testing.T) {
	long := strings.Repeat("d", MaxDescription+1)
	tests := []struct {
		name      string
		content   string
		wantName  string
		wantValid bool
		wantProb  string
	}{
		{name: "valid", content: goodSkill, wantName: "demo", wantValid: true},
		{name: "name comes from frontmatter", content: "---\nname: other-name\ndescription: A demo.\n---\n", wantName: "other-name", wantValid: true},
		{name: "missing name", content: "---\ndescription: A demo.\n---\n", wantProb: "frontmatter name must be a non-empty string"},
		{name: "invalid name", content: "---\nname: Demo\ndescription: A demo.\n---\n", wantName: "Demo", wantProb: "must match"},
		{name: "name with a path part", content: "---\nname: ../demo\ndescription: A demo.\n---\n", wantName: "../demo", wantProb: "must match"},
		{name: "missing description", content: "---\nname: demo\n---\n", wantName: "demo", wantProb: "frontmatter description must be a non-empty string"},
		{name: "long description", content: "---\nname: demo\ndescription: " + long + "\n---\n", wantName: "demo", wantProb: "max 1024"},
		{name: "no frontmatter", content: "# Demo\n", wantProb: "missing opening --- line"},
		{name: "empty content", content: "", wantProb: "missing opening --- line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := ParseContent([]byte(tt.content))
			if s.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", s.Name, tt.wantName)
			}
			if s.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v (problems %v)", s.Valid, tt.wantValid, s.Problems)
			}
			if s.Dir != "" || s.Hash != "" {
				t.Errorf("Dir = %q, Hash = %q, want both empty", s.Dir, s.Hash)
			}
			if tt.wantProb == "" {
				return
			}
			if !containsProblem(s.Problems, tt.wantProb) {
				t.Errorf("Problems = %v, want one containing %q", s.Problems, tt.wantProb)
			}
		})
	}
}

func containsProblem(problems []string, want string) bool {
	for _, p := range problems {
		if strings.Contains(p, want) {
			return true
		}
	}
	return false
}
