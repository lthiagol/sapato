package ops

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sapato/internal/hashdir"
	"sapato/internal/testfixtures"
)

// writeSkillDir writes a small valid skill called name into dir.
func writeSkillDir(t *testing.T, dir, name string) {
	t.Helper()
	writeTestFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: "+name+"\ndescription: A fresh skill.\n---\n\n# Fresh\n")
	writeTestFile(t, filepath.Join(dir, "notes.md"), "notes\n")
}

// fixtureSkill returns a copy of the fixture skill called name from the shared vault.
func fixtureSkill(t *testing.T, name string) string {
	t.Helper()
	return testfixtures.CopyTree(t, filepath.Join(testfixtures.Root(), "skill-db", "skills", name))
}

func TestImport(t *testing.T) {
	fresh := func(t *testing.T) string {
		dir := filepath.Join(t.TempDir(), "fresh-skill")
		writeSkillDir(t, dir, "fresh-skill")
		return dir
	}
	tests := []struct {
		name     string
		src      func(t *testing.T) string
		opts     ImportOptions
		wantName string
		wantErr  string
	}{
		{
			name:     "folder is imported under its frontmatter name",
			src:      fresh,
			wantName: "fresh-skill",
		},
		{
			name:     "name matching the frontmatter is accepted",
			src:      fresh,
			opts:     ImportOptions{Name: "fresh-skill"},
			wantName: "fresh-skill",
		},
		{
			name:    "name differing from the frontmatter is refused",
			src:     fresh,
			opts:    ImportOptions{Name: "other"},
			wantErr: `name "other" does not match frontmatter name "fresh-skill"`,
		},
		{
			name: "invalid skill is refused",
			src: func(t *testing.T) string {
				return fixtureSkill(t, "invalid-missing-description")
			},
			wantErr: "invalid skill",
		},
		{
			name: "name already in the vault is refused",
			src: func(t *testing.T) string {
				return fixtureSkill(t, "mocked-skill-00004")
			},
			wantErr: "already in the vault (use --force)",
		},
		{
			name: "force replaces the vault skill",
			src: func(t *testing.T) string {
				return fixtureSkill(t, "mocked-skill-00004")
			},
			opts:     ImportOptions{Options: Options{Force: true}},
			wantName: "mocked-skill-00004",
		},
		{
			name:     "dry run plans the copy and changes nothing",
			src:      fresh,
			opts:     ImportOptions{Options: Options{DryRun: true}},
			wantName: "fresh-skill",
		},
		{
			name:    "missing folder is refused",
			src:     func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent") },
			wantErr: "no such file",
		},
		{
			name: "subdir holding the skill is used",
			src: func(t *testing.T) string {
				root := filepath.Join(t.TempDir(), "repo")
				writeSkillDir(t, filepath.Join(root, "skills", "fresh-skill"), "fresh-skill")
				return root
			},
			opts:     ImportOptions{Subdir: "skills/fresh-skill"},
			wantName: "fresh-skill",
		},
		{
			name:    "subdir outside the source is refused",
			src:     fresh,
			opts:    ImportOptions{Subdir: "../elsewhere"},
			wantErr: "must be a path inside the source",
		},
		{
			name:    "subdir that does not exist is refused",
			src:     fresh,
			opts:    ImportOptions{Subdir: "nope"},
			wantErr: "no such file",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			before := snapshot(t, v.Path)
			src := tt.src(t)

			action, err := Import(v, src, tt.opts)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Import() error = %v, want one containing %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("Import() error = %v", err)
			}
			if tt.wantErr != "" || tt.opts.DryRun {
				if after := snapshot(t, v.Path); !reflect.DeepEqual(after, before) {
					t.Error("vault changed on a refused or dry-run import")
				}
			}
			if tt.wantErr != "" {
				return
			}

			wantAction := Action{Op: OpImport, Skill: tt.wantName, Path: v.SkillDir(tt.wantName)}
			if action != wantAction {
				t.Errorf("action = %+v, want %+v", action, wantAction)
			}
			if tt.opts.DryRun {
				return
			}
			got := vaultSkill(t, v, tt.wantName)
			if !got.Valid {
				t.Errorf("imported skill is invalid: %v", got.Problems)
			}
			want, err := hashdir.Hash(filepath.Join(src, tt.opts.Subdir))
			if err != nil {
				t.Fatal(err)
			}
			if got.Hash != want {
				t.Errorf("vault hash = %s, want source hash %s", got.Hash, want)
			}
		})
	}
}

// requireGit skips the test when git is not installed.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

// runGit runs git in dir with a fixed identity and without the user's git config.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=sapato",
		"GIT_AUTHOR_EMAIL=sapato@example.invalid",
		"GIT_COMMITTER_NAME=sapato",
		"GIT_COMMITTER_EMAIL=sapato@example.invalid",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// fixtureRepo makes a repository named skills.git, with one commit that holds
// the fixture skill mocked-skill-00004 at subdir. The name ends in .git, so
// Import reads it as a git URL, and git clones a local path without the network.
func fixtureRepo(t *testing.T, subdir string) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "skills.git")
	at := filepath.Join(repo, filepath.FromSlash(subdir))
	if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(fixtureSkill(t, "mocked-skill-00004"), at); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "add skill")
	return repo
}

func TestImportGit(t *testing.T) {
	requireGit(t)
	tests := []struct {
		name       string
		repoSubdir string // where the skill sits in the repository
		missing    bool   // the URL names a repository that does not exist
		opts       ImportOptions
		wantName   string
		wantErr    string
	}{
		{
			name:       "skill at the repository root",
			repoSubdir: "",
			wantName:   "mocked-skill-00004",
		},
		{
			name:       "skill in a subdir",
			repoSubdir: "skills/mocked-skill-00004",
			opts:       ImportOptions{Subdir: "skills/mocked-skill-00004"},
			wantName:   "mocked-skill-00004",
		},
		{
			name:       "subdir missing from the repository is refused",
			repoSubdir: "skills/mocked-skill-00004",
			opts:       ImportOptions{Subdir: "skills/other"},
			wantErr:    "no such file",
		},
		{
			name:       "name must match the frontmatter",
			repoSubdir: "",
			opts:       ImportOptions{Name: "other"},
			wantErr:    "does not match frontmatter name",
		},
		{
			name:    "missing repository is refused",
			missing: true,
			wantErr: "git clone",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("TMPDIR", tmp)
			v := fixtureVault(t)
			// The fixture vault already holds the skill the repository carries.
			if err := os.RemoveAll(v.SkillDir("mocked-skill-00004")); err != nil {
				t.Fatal(err)
			}

			src := filepath.Join(t.TempDir(), "absent.git")
			if !tt.missing {
				src = fixtureRepo(t, tt.repoSubdir)
			}
			_, err := Import(v, src, tt.opts)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Import() error = %v, want one containing %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("Import() error = %v", err)
			}

			left, err := os.ReadDir(tmp)
			if err != nil {
				t.Fatal(err)
			}
			if len(left) != 0 {
				t.Errorf("clone temp dir not removed: %d entries left", len(left))
			}
			if tt.wantErr != "" {
				return
			}

			got := vaultSkill(t, v, tt.wantName)
			if !got.Valid {
				t.Errorf("imported skill is invalid: %v", got.Problems)
			}
			want, err := hashdir.Hash(fixtureSkill(t, "mocked-skill-00004"))
			if err != nil {
				t.Fatal(err)
			}
			if got.Hash != want {
				t.Errorf("vault hash = %s, want fixture hash %s", got.Hash, want)
			}
		})
	}
}

func TestIsGitURL(t *testing.T) {
	tests := []struct {
		src  string
		want bool
	}{
		{"https://example.com/skill.git", true},
		{"https://example.com/skill", true},
		{"git@example.com:user/skill.git", true},
		{"git@example.com:user/skill", true},
		{"/home/u/skill.git", true},
		{"/home/u/skill", false},
		{"./skill", false},
		{"http://example.com/skill", false},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			if got := isGitURL(tt.src); got != tt.want {
				t.Errorf("isGitURL(%q) = %v, want %v", tt.src, got, tt.want)
			}
		})
	}
}
