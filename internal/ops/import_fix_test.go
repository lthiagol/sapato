package ops

import (
	"archive/tar"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"sapato/internal/vault"
)

// skillMD returns a valid SKILL.md called name.
func skillMD(name string) string {
	return "---\nname: " + name + "\ndescription: A " + name + " skill.\n---\n"
}

func TestImportAll(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		opts      ImportOptions
		wantNames []string
		wantErr   string
	}{
		{
			name:      "every skill is imported",
			files:     map[string]string{"a/alpha/SKILL.md": skillMD("alpha"), "b/beta/SKILL.md": skillMD("beta")},
			opts:      ImportOptions{All: true},
			wantNames: []string{"alpha", "beta"},
		},
		{
			name:    "one invalid skill imports nothing",
			files:   map[string]string{"a/alpha/SKILL.md": skillMD("alpha"), "b/beta/SKILL.md": "---\nname: beta\n---\n"},
			opts:    ImportOptions{All: true},
			wantErr: "invalid skill",
		},
		{
			name:    "two skills with one name are refused",
			files:   map[string]string{"a/fresh-skill/SKILL.md": skillMD("fresh-skill"), "b/fresh-skill/SKILL.md": skillMD("fresh-skill")},
			opts:    ImportOptions{All: true},
			wantErr: `two skills take the name "fresh-skill"`,
		},
		{
			name:    "several skills without --all list their paths",
			files:   map[string]string{"a/alpha/SKILL.md": skillMD("alpha"), "b/beta/SKILL.md": skillMD("beta")},
			wantErr: "several skills found (a/alpha, b/beta)",
		},
		{
			name:    "name with all is refused",
			files:   map[string]string{"a/alpha/SKILL.md": skillMD("alpha")},
			opts:    ImportOptions{All: true, Name: "alpha"},
			wantErr: "--name does not apply with --all",
		},
		{
			name: "all with --fix repairs each skill by its folder",
			files: map[string]string{
				"a/alpha/SKILL.md":   "---\ndescription: One.\n---\n",
				"b/My Beta/SKILL.md": "# Beta\n",
			},
			opts:      ImportOptions{All: true, Fix: true, Description: "Filled in."},
			wantNames: []string{"alpha", "my-beta"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			before := snapshot(t, v.Path)
			root := filepath.Join(t.TempDir(), "pack")
			writeFiles(t, root, tt.files)

			actions, err := Import(v, root, tt.opts)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Import() error = %v, want one containing %q", err, tt.wantErr)
				}
				if after := snapshot(t, v.Path); !reflect.DeepEqual(after, before) {
					t.Error("refused import changed the vault")
				}
				return
			}
			if err != nil {
				t.Fatalf("Import() error = %v", err)
			}
			if len(actions) != len(tt.wantNames) {
				t.Fatalf("actions = %+v, want %d", actions, len(tt.wantNames))
			}
			for _, name := range tt.wantNames {
				if got := vaultSkill(t, v, name); !got.Valid {
					t.Errorf("imported %s is invalid: %v", name, got.Problems)
				}
			}
		})
	}
}

func TestImportFix(t *testing.T) {
	tooLong := "---\nname: fresh-skill\ndescription: " + strings.Repeat("x", 1025) + "\n---\n"
	tests := []struct {
		name     string
		folder   string // the source folder, which is the name hint
		content  string // the source SKILL.md
		opts     ImportOptions
		wantName string
		wantFile string // the vault SKILL.md after the import
		wantErr  string
		noFixTip bool // the error must not suggest --fix
	}{
		{
			name:     "no frontmatter gets a new one",
			folder:   "fresh-skill",
			content:  "# Fresh\n",
			opts:     ImportOptions{Options: Options{}, Fix: true, Description: "A fresh skill."},
			wantName: "fresh-skill",
			wantFile: "---\nname: fresh-skill\ndescription: A fresh skill.\n---\n# Fresh\n",
		},
		{
			name:     "missing name takes the folder name",
			folder:   "fresh-skill",
			content:  "---\ndescription: A fresh skill.\n---\n\n# Fresh\n",
			opts:     ImportOptions{Fix: true},
			wantName: "fresh-skill",
			wantFile: "---\ndescription: A fresh skill.\nname: fresh-skill\n---\n\n# Fresh\n",
		},
		{
			name:     "valid name that is not the folder keeps the frontmatter name",
			folder:   "repo-checkout",
			content:  "---\nname: code-review\n---\n",
			opts:     ImportOptions{Fix: true, Description: "A fresh skill."},
			wantName: "code-review",
			wantFile: "---\nname: code-review\ndescription: A fresh skill.\n---\n",
		},
		{
			name:    "--name differing from the frontmatter is refused without --fix",
			folder:  "fresh-skill",
			content: "---\nname: other\ndescription: A fresh skill.\n---\n",
			opts:    ImportOptions{Name: "renamed"},
			wantErr: `name "renamed" does not match frontmatter name "other"; --fix renames the skill`,
		},
		{
			name:     "--name is the name a repaired skill takes",
			folder:   "fresh-skill",
			content:  "---\nname: other\ndescription: A fresh skill.\n---\n",
			opts:     ImportOptions{Name: "renamed", Fix: true},
			wantName: "renamed",
			wantFile: "---\nname: renamed\ndescription: A fresh skill.\n---\n",
		},
		{
			name:     "folder with spaces and capitals gives a normalized name",
			folder:   "My Skill",
			content:  "---\ndescription: A fresh skill.\n---\n",
			opts:     ImportOptions{Fix: true},
			wantName: "my-skill",
			wantFile: "---\ndescription: A fresh skill.\nname: my-skill\n---\n",
		},
		{
			name:     "missing description takes --description",
			folder:   "fresh-skill",
			content:  "---\nname: fresh-skill\n---\n",
			opts:     ImportOptions{Fix: true, Description: "Filled in."},
			wantName: "fresh-skill",
			wantFile: "---\nname: fresh-skill\ndescription: Filled in.\n---\n",
		},
		{
			name:    "missing description without --description is refused",
			folder:  "fresh-skill",
			content: "---\nname: fresh-skill\n---\n",
			opts:    ImportOptions{Fix: true},
			wantErr: "description is missing: pass --description",
		},
		{
			name:     "extra keys keep their order and the body is kept",
			folder:   "fresh-skill",
			content:  "---\nlicense: MIT\nname: other\ndescription: A fresh skill.\nmetadata:\n  version: 1.0.0\n---\n\n# Body\n",
			opts:     ImportOptions{Name: "fresh-skill", Fix: true},
			wantName: "fresh-skill",
			wantFile: "---\nlicense: MIT\nname: fresh-skill\ndescription: A fresh skill.\nmetadata:\n  version: 1.0.0\n---\n\n# Body\n",
		},
		{
			name:     "dry run plans the repair and changes nothing",
			folder:   "fresh-skill",
			content:  "# Fresh\n",
			opts:     ImportOptions{Options: Options{DryRun: true}, Fix: true, Description: "A fresh skill."},
			wantName: "fresh-skill",
		},
		{
			name:    "too long description is refused even with --fix",
			folder:  "fresh-skill",
			content: tooLong,
			opts:    ImportOptions{Fix: true, Description: "Short."},
			wantErr: "invalid skill: description is 1025 characters, max 1024",
		},
		{
			name:     "missing name without --fix names the fix",
			folder:   "fresh-skill",
			content:  "---\ndescription: A fresh skill.\n---\n",
			wantErr:  "invalid skill: frontmatter name must be a non-empty string; --fix can repair it",
			wantName: "fresh-skill",
		},
		{
			name:     "too long description without --fix gets no fix tip",
			folder:   "fresh-skill",
			content:  tooLong,
			wantErr:  "invalid skill",
			noFixTip: true,
		},
		{
			name:    "no name can be derived without --name",
			folder:  "!!!",
			content: "---\ndescription: A fresh skill.\n---\n",
			opts:    ImportOptions{Fix: true},
			wantErr: "pass --name",
		},
		{
			name:    "--name must match a valid frontmatter name",
			folder:  "fresh-skill",
			content: "---\nname: fresh-skill\ndescription: A fresh skill.\n---\n",
			opts:    ImportOptions{Name: "other"},
			wantErr: `name "other" does not match frontmatter name "fresh-skill"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			before := snapshot(t, v.Path)
			src := filepath.Join(t.TempDir(), tt.folder)
			writeTestFile(t, filepath.Join(src, "SKILL.md"), tt.content)
			writeTestFile(t, filepath.Join(src, "notes.md"), "notes\n")
			srcBefore := snapshot(t, src)

			actions, err := Import(v, src, tt.opts)
			if after := snapshot(t, src); !reflect.DeepEqual(after, srcBefore) {
				t.Error("source changed")
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Import() error = %v, want one containing %q", err, tt.wantErr)
				}
				if strings.Contains(err.Error(), "--fix can repair") && tt.noFixTip {
					t.Errorf("error %q suggests --fix, want no suggestion", err)
				}
				if after := snapshot(t, v.Path); !reflect.DeepEqual(after, before) {
					t.Error("refused import changed the vault")
				}
				return
			}
			if err != nil {
				t.Fatalf("Import() error = %v", err)
			}
			if len(actions) != 1 || actions[0].Skill != tt.wantName {
				t.Fatalf("actions = %+v, want one for %s", actions, tt.wantName)
			}
			if tt.opts.DryRun {
				if after := snapshot(t, v.Path); !reflect.DeepEqual(after, before) {
					t.Error("dry run changed the vault")
				}
				return
			}
			got, err := os.ReadFile(filepath.Join(v.SkillDir(tt.wantName), "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.wantFile {
				t.Errorf("vault SKILL.md =\n%q\nwant\n%q", got, tt.wantFile)
			}
			if _, err := os.Stat(filepath.Join(v.SkillDir(tt.wantName), "notes.md")); err != nil {
				t.Errorf("other files are not copied: %v", err)
			}
			if got := vaultSkill(t, v, tt.wantName); !got.Valid {
				t.Errorf("repaired skill is invalid: %v", got.Problems)
			}
		})
	}
}

func TestImportFixRefusesTarball(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	_, err := Import(fixtureVault(t), makeTarball(t, skillEntries("tar-skill")...), ImportOptions{Fix: true})
	if err == nil || !strings.Contains(err.Error(), "do not apply to a tarball") {
		t.Fatalf("Import() error = %v, want the tarball refusal", err)
	}
}

func TestImportSourceKinds(t *testing.T) {
	t.Run("local file is one skill named after its folder", func(t *testing.T) {
		v := fixtureVault(t)
		path := filepath.Join(t.TempDir(), "fresh-skill", "notes.md")
		writeTestFile(t, path, freshSkillMD)
		if _, err := Import(v, path, ImportOptions{}); err != nil {
			t.Fatalf("Import() error = %v", err)
		}
		if got := vaultSkill(t, v, "fresh-skill"); !got.Valid {
			t.Errorf("imported skill is invalid: %v", got.Problems)
		}
	})

	t.Run("other local file is refused and changes nothing", func(t *testing.T) {
		v := fixtureVault(t)
		before := snapshot(t, v.Path)
		path := filepath.Join(t.TempDir(), "fresh-skill", "notes.txt")
		writeTestFile(t, path, freshSkillMD)
		_, err := Import(v, path, ImportOptions{})
		if err == nil || !strings.Contains(err.Error(), "is not a .md file or a tarball") {
			t.Fatalf("Import() error = %v, want the file type refusal", err)
		}
		if after := snapshot(t, v.Path); !reflect.DeepEqual(after, before) {
			t.Error("refused import changed the vault")
		}
	})

	t.Run("raw URL is one skill named after its folder", func(t *testing.T) {
		t.Setenv("TMPDIR", t.TempDir())
		v := fixtureVault(t)
		srv := serveRaw(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/skills/fresh-skill/SKILL.md" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(freshSkillMD))
		}))
		if _, err := Import(v, srv.URL+"/skills/fresh-skill/SKILL.md", ImportOptions{}); err != nil {
			t.Fatalf("Import() error = %v", err)
		}
		if got := vaultSkill(t, v, "fresh-skill"); !got.Valid {
			t.Errorf("imported skill is invalid: %v", got.Problems)
		}
	})

	t.Run("GitHub blob URL is read from the raw file", func(t *testing.T) {
		t.Setenv("TMPDIR", t.TempDir())
		v := fixtureVault(t)
		srv := serveRaw(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/o/r/main/skills/fresh-skill/SKILL.md" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(freshSkillMD))
		}))
		_ = srv
		if _, err := Import(v, "https://github.com/o/r/blob/main/skills/fresh-skill/SKILL.md", ImportOptions{}); err != nil {
			t.Fatalf("Import() error = %v", err)
		}
		if got := vaultSkill(t, v, "fresh-skill"); !got.Valid {
			t.Errorf("imported skill is invalid: %v", got.Problems)
		}
	})

	t.Run("raw URL with a bad status changes nothing and cleans its temp dir", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("TMPDIR", tmp)
		v := fixtureVault(t)
		before := snapshot(t, v.Path)
		srv := serveRaw(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		_, err := Import(v, srv.URL+"/skills/fresh-skill/SKILL.md", ImportOptions{})
		if err == nil || !strings.Contains(err.Error(), "status 404") {
			t.Fatalf("Import() error = %v, want the status refusal", err)
		}
		if after := snapshot(t, v.Path); !reflect.DeepEqual(after, before) {
			t.Error("failed fetch changed the vault")
		}
		if left, _ := os.ReadDir(tmp); len(left) != 0 {
			t.Errorf("temp dir not removed: %d entries left", len(left))
		}
	})
}

func TestImportPick(t *testing.T) {
	three := map[string]string{
		"a/alpha/SKILL.md": skillMD("alpha"),
		"b/beta/SKILL.md":  skillMD("beta"),
		"c/gamma/SKILL.md": skillMD("gamma"),
	}
	tests := []struct {
		name      string
		files     map[string]string
		pick      []string
		all       bool
		wantNames []string
		wantErr   string
	}{
		{
			name:      "only the picked folders are imported",
			files:     three,
			pick:      []string{"a/alpha", "c/gamma"},
			wantNames: []string{"alpha", "gamma"},
		},
		{
			name:    "a folder that is not in the source is refused",
			files:   three,
			pick:    []string{"a/alpha", "d/delta"},
			wantErr: `no skill at "d/delta" in the source`,
		},
		{
			name:    "one invalid pick imports nothing",
			files:   map[string]string{"a/alpha/SKILL.md": skillMD("alpha"), "b/beta/SKILL.md": "---\nname: beta\n---\n"},
			pick:    []string{"a/alpha", "b/beta"},
			wantErr: "invalid skill",
		},
		{
			name:    "pick with all is refused",
			files:   three,
			pick:    []string{"a/alpha"},
			all:     true,
			wantErr: "--pick does not apply with --all",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			before := snapshot(t, v.Path)
			root := filepath.Join(t.TempDir(), "pack")
			writeFiles(t, root, tt.files)

			actions, err := Import(v, root, ImportOptions{Pick: tt.pick, All: tt.all})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Import() error = %v, want one containing %q", err, tt.wantErr)
				}
				if after := snapshot(t, v.Path); !reflect.DeepEqual(after, before) {
					t.Error("refused import changed the vault")
				}
				return
			}
			if err != nil {
				t.Fatalf("Import() error = %v", err)
			}
			if len(actions) != len(tt.wantNames) {
				t.Fatalf("actions = %+v, want %d", actions, len(tt.wantNames))
			}
			for _, name := range tt.wantNames {
				if got := vaultSkill(t, v, name); !got.Valid {
					t.Errorf("imported %s is invalid: %v", name, got.Problems)
				}
			}
			if inVault(t, v, "beta") {
				t.Error("beta was imported but not picked")
			}
		})
	}
}

func TestImportPickTarball(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	v := fixtureVault(t)
	before := snapshot(t, v.Path)
	archive := makeTarball(t, append(skillEntries("tar-one"), skillEntries("tar-two")...)...)

	if _, err := Import(v, archive, ImportOptions{Pick: []string{"tar-one", "tar-missing"}}); err == nil || !strings.Contains(err.Error(), `no skill "tar-missing" in the archive`) {
		t.Fatalf("missing pick: error = %v", err)
	}
	if after := snapshot(t, v.Path); !reflect.DeepEqual(after, before) {
		t.Fatal("refused tarball pick changed the vault")
	}

	actions, err := Import(v, archive, ImportOptions{Pick: []string{"tar-two"}})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if len(actions) != 1 || actions[0].Skill != "tar-two" {
		t.Fatalf("actions = %+v, want only tar-two", actions)
	}
	if inVault(t, v, "tar-one") {
		t.Error("tar-one was imported but not picked")
	}

	bad := append(skillEntries("tar-ok"), []tarEntry{
		{name: "tar-bad/", typeflag: tar.TypeDir},
		{name: "tar-bad/SKILL.md", body: []byte("---\nname: tar-bad\n---\n")},
	}...)
	badArchive := makeTarball(t, bad...)
	before = snapshot(t, v.Path)
	if _, err := Import(v, badArchive, ImportOptions{Pick: []string{"tar-bad"}}); err == nil || !strings.Contains(err.Error(), "invalid skill tar-bad") {
		t.Fatalf("picked invalid skill: error = %v", err)
	}
	if after := snapshot(t, v.Path); !reflect.DeepEqual(after, before) {
		t.Fatal("refused invalid pick changed the vault")
	}
	actions, err = Import(v, badArchive, ImportOptions{Pick: []string{"tar-ok"}})
	if err != nil {
		t.Fatalf("unpicked invalid skill blocked a picked valid one: %v", err)
	}
	if len(actions) != 1 || actions[0].Skill != "tar-ok" || inVault(t, v, "tar-bad") {
		t.Errorf("actions = %+v, want only tar-ok", actions)
	}
}

// inVault reports whether the vault lists a skill called name, valid or not.
func inVault(t *testing.T, v *vault.Vault, name string) bool {
	t.Helper()
	skills, err := v.Skills()
	if err != nil {
		t.Fatal(err)
	}
	return slices.ContainsFunc(skills, func(s vault.Skill) bool { return s.Name == name })
}
