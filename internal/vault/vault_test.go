package vault

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newVault(t *testing.T, manifest string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sapato.toml"), manifest)
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestOpen(t *testing.T) {
	tests := []struct {
		name        string
		manifest    string // empty means no sapato.toml
		noManifest  bool
		wantErr     string
		wantAssigns map[string][]string
	}{
		{
			name:        "format 1 without assignments",
			manifest:    "format = 1\n",
			wantAssigns: nil,
		},
		{
			name:     "format 1 with assignments",
			manifest: "format = 1\n\n[assignments]\nclaude = [\"code-review\", \"obsidian-*\"]\nagents = [\"*\"]\n",
			wantAssigns: map[string][]string{
				"claude": {"code-review", "obsidian-*"},
				"agents": {"*"},
			},
		},
		{
			name:       "missing manifest",
			noManifest: true,
			wantErr:    "open vault",
		},
		{
			name:     "missing format",
			manifest: "[assignments]\n",
			wantErr:  "format is required",
		},
		{
			name:     "format 2 unsupported",
			manifest: "format = 2\n",
			wantErr:  "unsupported format 2",
		},
		{
			name:     "format as string",
			manifest: "format = \"1\"\n",
			wantErr:  "parse",
		},
		{
			name:     "invalid toml",
			manifest: "format = \n",
			wantErr:  "parse",
		},
		{
			name:     "assignments wrong type",
			manifest: "format = 1\n[assignments]\nclaude = \"code-review\"\n",
			wantErr:  "parse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dir string
			if tt.noManifest {
				dir = t.TempDir()
			} else {
				dir = newVault(t, tt.manifest)
			}
			v, err := Open(dir)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Open() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			if v.Path != dir {
				t.Errorf("Path = %s, want %s", v.Path, dir)
			}
			if !reflect.DeepEqual(v.Assignments, tt.wantAssigns) {
				t.Errorf("Assignments = %v, want %v", v.Assignments, tt.wantAssigns)
			}
		})
	}
}

func TestExists(t *testing.T) {
	dir := newVault(t, "format = 1\n")
	if ok, err := Exists(dir); err != nil || !ok {
		t.Errorf("Exists(vault) = %v, %v; want true, nil", ok, err)
	}
	if ok, err := Exists(t.TempDir()); err != nil || ok {
		t.Errorf("Exists(empty) = %v, %v; want false, nil", ok, err)
	}
}

// manifestSpec is the new-vault manifest of spec §3.1 byte for byte. A raw string
// cannot hold a backtick, so the spec's backticks are ^ here.
var manifestSpec = `# Sapato vault manifest. Portable: no machine-specific paths here.

# Vault format version. Only 1 is supported.
format = 1

# Desired installs, applied by ^sapato sync^.
# One line per agent name from your config's [agents]; values are skill
# names or globs (* ? [a-z]). Agents not configured on a machine are skipped there.
# ^sync --prune^ also uninstalls managed skills no longer listed.
# Example:
#   claude = ["code-review", "obsidian-*"]
#   agents = ["*"]
[assignments]

# Per-skill metadata, written by sapato (^sapato compat^, the TUI ^c^ key,
# the NEW wizard). You can edit it by hand too.
# compatible: agent names this skill works with; no table = every agent.
# Example:
#   [skills.code-review]
#   compatible = ["claude"]
`

func TestInit(t *testing.T) {
	t.Run("creates vault", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "new", "vault")
		if err := Init(dir); err != nil {
			t.Fatalf("Init() error = %v", err)
		}
		data, err := os.ReadFile(filepath.Join(dir, "sapato.toml"))
		if err != nil {
			t.Fatal(err)
		}
		if want := strings.ReplaceAll(manifestSpec, "^", "`"); string(data) != want {
			t.Errorf("manifest = %q, want spec §3.1 %q", data, want)
		}
		v, err := Open(dir)
		if err != nil {
			t.Fatalf("Open() after Init error = %v", err)
		}
		if len(v.Assignments) != 0 {
			t.Errorf("Assignments = %v, want none", v.Assignments)
		}
		info, err := os.Stat(filepath.Join(dir, "skills"))
		if err != nil || !info.IsDir() {
			t.Errorf("skills dir missing: %v", err)
		}
	})

	t.Run("refuses existing vault and leaves it untouched", func(t *testing.T) {
		dir := newVault(t, "format = 1\n[assignments]\nclaude = [\"x\"]\n")
		err := Init(dir)
		if err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("Init() error = %v, want already exists", err)
		}
		data, err := os.ReadFile(filepath.Join(dir, "sapato.toml"))
		if err != nil {
			t.Fatal(err)
		}
		if want := "format = 1\n[assignments]\nclaude = [\"x\"]\n"; string(data) != want {
			t.Errorf("manifest changed to %q", data)
		}
	})
}

const validSkill = "---\nname: code-review\ndescription: Reviews code.\n---\n# Body\n"

func TestSkillsValidation(t *testing.T) {
	long := func(c string, n int) string { return strings.Repeat(c, n) }

	tests := []struct {
		name        string
		folder      string
		skillMD     string // empty means no SKILL.md
		skillIsDir  bool
		wantProblem string // substring; empty means valid
		wantVersion string
		wantDesc    string
	}{
		{
			name:     "valid",
			folder:   "code-review",
			skillMD:  validSkill,
			wantDesc: "Reviews code.",
		},
		{
			name:        "valid with metadata version",
			folder:      "code-review",
			skillMD:     "---\nname: code-review\ndescription: d\nmetadata:\n  version: 1.2.0\n---\n",
			wantVersion: "1.2.0",
			wantDesc:    "d",
		},
		{
			name:     "non-string version is ignored",
			folder:   "code-review",
			skillMD:  "---\nname: code-review\ndescription: d\nmetadata:\n  version: 2\n---\n",
			wantDesc: "d",
		},
		{
			name:     "CRLF delimiters accepted",
			folder:   "code-review",
			skillMD:  "---\r\nname: code-review\r\ndescription: d\r\n---\r\n",
			wantDesc: "d",
		},
		{
			name:        "rule 1: SKILL.md missing",
			folder:      "code-review",
			wantProblem: "SKILL.md is missing",
		},
		{
			name:        "rule 1: SKILL.md is a directory",
			folder:      "code-review",
			skillIsDir:  true,
			wantProblem: "not a regular file",
		},
		{
			name:        "rule 2: no opening delimiter",
			folder:      "code-review",
			skillMD:     "name: code-review\ndescription: d\n---\n",
			wantProblem: "missing opening --- line",
		},
		{
			name:        "rule 2: no closing delimiter",
			folder:      "code-review",
			skillMD:     "---\nname: code-review\ndescription: d\n",
			wantProblem: "missing closing --- line",
		},
		{
			name:        "rule 2: invalid YAML",
			folder:      "code-review",
			skillMD:     "---\nname: [code-review\n---\n",
			wantProblem: "invalid YAML",
		},
		{
			name:        "rule 3: name missing",
			folder:      "code-review",
			skillMD:     "---\ndescription: d\n---\n",
			wantProblem: "frontmatter name must be a non-empty string",
			wantDesc:    "d",
		},
		{
			name:        "rule 3: name empty",
			folder:      "code-review",
			skillMD:     "---\nname: \"\"\ndescription: d\n---\n",
			wantProblem: "frontmatter name must be a non-empty string",
			wantDesc:    "d",
		},
		{
			name:        "rule 3: name not a string",
			folder:      "42",
			skillMD:     "---\nname: 42\ndescription: d\n---\n",
			wantProblem: "frontmatter name must be a non-empty string",
			wantDesc:    "d",
		},
		{
			name:        "rule 3: description missing",
			folder:      "code-review",
			skillMD:     "---\nname: code-review\n---\n",
			wantProblem: "frontmatter description must be a non-empty string",
		},
		{
			name:        "rule 3: description empty",
			folder:      "code-review",
			skillMD:     "---\nname: code-review\ndescription: \"\"\n---\n",
			wantProblem: "frontmatter description must be a non-empty string",
		},
		{
			name:        "rule 4: name differs from folder",
			folder:      "code-rev",
			skillMD:     validSkill,
			wantProblem: `name "code-review" does not match folder "code-rev"`,
			wantDesc:    "Reviews code.",
		},
		{
			name:        "rule 5: uppercase name",
			folder:      "Code-Review",
			skillMD:     "---\nname: Code-Review\ndescription: d\n---\n",
			wantProblem: "must match",
			wantDesc:    "d",
		},
		{
			name:        "rule 5: underscore in name",
			folder:      "code_review",
			skillMD:     "---\nname: code_review\ndescription: d\n---\n",
			wantProblem: "must match",
			wantDesc:    "d",
		},
		{
			name:        "rule 5: double hyphen",
			folder:      "code--review",
			skillMD:     "---\nname: code--review\ndescription: d\n---\n",
			wantProblem: "must match",
			wantDesc:    "d",
		},
		{
			name:        "rule 5: trailing hyphen",
			folder:      "code-",
			skillMD:     "---\nname: code-\ndescription: d\n---\n",
			wantProblem: "must match",
			wantDesc:    "d",
		},
		{
			name:     "rule 5: 64 chars accepted",
			folder:   long("a", 64),
			skillMD:  "---\nname: " + long("a", 64) + "\ndescription: d\n---\n",
			wantDesc: "d",
		},
		{
			name:        "rule 5: 65 chars rejected",
			folder:      long("a", 65),
			skillMD:     "---\nname: " + long("a", 65) + "\ndescription: d\n---\n",
			wantProblem: "name is 65 characters, max 64",
			wantDesc:    "d",
		},
		{
			name:     "rule 6: 1024 chars accepted",
			folder:   "code-review",
			skillMD:  "---\nname: code-review\ndescription: " + long("d", 1024) + "\n---\n",
			wantDesc: long("d", 1024),
		},
		{
			name:        "rule 6: 1025 chars rejected",
			folder:      "code-review",
			skillMD:     "---\nname: code-review\ndescription: " + long("d", 1025) + "\n---\n",
			wantProblem: "description is 1025 characters, max 1024",
			wantDesc:    long("d", 1025),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newVault(t, "format = 1\n")
			skillDir := filepath.Join(dir, "skills", tt.folder)
			if err := os.MkdirAll(skillDir, 0o755); err != nil {
				t.Fatal(err)
			}
			switch {
			case tt.skillIsDir:
				if err := os.MkdirAll(filepath.Join(skillDir, "SKILL.md"), 0o755); err != nil {
					t.Fatal(err)
				}
			case tt.skillMD != "":
				writeFile(t, filepath.Join(skillDir, "SKILL.md"), tt.skillMD)
			}

			v, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			skills, err := v.Skills()
			if err != nil {
				t.Fatalf("Skills() error = %v", err)
			}
			if len(skills) != 1 {
				t.Fatalf("got %d skills, want 1", len(skills))
			}
			s := skills[0]

			if tt.wantProblem == "" {
				if !s.Valid || len(s.Problems) != 0 {
					t.Errorf("Valid = %v, Problems = %v; want valid", s.Valid, s.Problems)
				}
			} else {
				if s.Valid {
					t.Errorf("Valid = true, want false")
				}
				found := false
				for _, p := range s.Problems {
					if strings.Contains(p, tt.wantProblem) {
						found = true
					}
				}
				if !found {
					t.Errorf("Problems = %v, want one containing %q", s.Problems, tt.wantProblem)
				}
			}
			if s.Version != tt.wantVersion {
				t.Errorf("Version = %q, want %q", s.Version, tt.wantVersion)
			}
			if s.Description != tt.wantDesc {
				t.Errorf("Description = %q, want %q", s.Description, tt.wantDesc)
			}
			if s.Name != tt.folder || s.Dir != skillDir {
				t.Errorf("Name, Dir = %q, %q; want %q, %q", s.Name, s.Dir, tt.folder, skillDir)
			}
			if !strings.HasPrefix(s.Hash, "sha256:") {
				t.Errorf("Hash = %q, want sha256:...", s.Hash)
			}
		})
	}
}

func TestSkillsListing(t *testing.T) {
	dir := newVault(t, "format = 1\n")
	writeFile(t, filepath.Join(dir, "skills", "zeta", "SKILL.md"), "---\nname: zeta\ndescription: z\n---\n")
	writeFile(t, filepath.Join(dir, "skills", "alpha", "SKILL.md"), "---\nname: alpha\ndescription: a\n---\n")
	writeFile(t, filepath.Join(dir, "skills", ".hidden", "SKILL.md"), validSkill)
	writeFile(t, filepath.Join(dir, "skills", "README.md"), "not a skill")
	if err := os.MkdirAll(filepath.Join(dir, "skills", "broken"), 0o755); err != nil {
		t.Fatal(err)
	}

	v, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	skills, err := v.Skills()
	if err != nil {
		t.Fatalf("Skills() error = %v", err)
	}

	var names []string
	for _, s := range skills {
		names = append(names, s.Name)
	}
	if want := []string{"alpha", "broken", "zeta"}; !reflect.DeepEqual(names, want) {
		t.Errorf("names = %v, want %v (sorted, dot entries and files ignored)", names, want)
	}
	if skills[1].Valid {
		t.Errorf("broken skill reported valid")
	}
}

func TestSkillsMissingSkillsDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sapato.toml"), "format = 1\n")
	v, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Skills(); err == nil {
		t.Error("Skills() with no skills dir returned nil error")
	}
}
