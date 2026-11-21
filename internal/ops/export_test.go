package ops

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"sapato/internal/hashdir"
	"sapato/internal/vault"
)

// fixtureNames are the valid skills of the shared vault fixture.
var fixtureNames = []string{
	"mocked-skill-00001", "mocked-skill-00002", "mocked-skill-00003",
	"mocked-skill-00004", "mocked-skill-00005", "mocked-skill-00006",
}

// emptyVault creates and opens a vault with no skills.
func emptyVault(t *testing.T) *vault.Vault {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "vault")
	if err := vault.Init(dir); err != nil {
		t.Fatal(err)
	}
	v, err := vault.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// exportTo exports skills from v into a new file and returns the file's path.
func exportTo(t *testing.T, v *vault.Vault, skills ...string) string {
	t.Helper()
	var buf bytes.Buffer
	if _, err := Export(v, skills, &buf); err != nil {
		t.Fatalf("Export(%v) error = %v", skills, err)
	}
	path := filepath.Join(t.TempDir(), "export.skill.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// execFiles maps each file that counts toward the hash of dir to its exec bit.
func execFiles(t *testing.T, dir string) map[string]bool {
	t.Helper()
	files, err := hashdir.Files(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := make(map[string]bool, len(files))
	for _, f := range files {
		m[f.Rel] = hashdir.Exec(f.Mode)
	}
	return m
}

func TestExportImportRoundTrip(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	for _, name := range fixtureNames {
		t.Run(name, func(t *testing.T) {
			src := fixtureVault(t)
			dst := emptyVault(t)

			actions, err := Import(dst, exportTo(t, src, name), ImportOptions{})
			if err != nil {
				t.Fatalf("Import() error = %v", err)
			}
			if want := []Action{{Op: OpImport, Skill: name, Path: dst.SkillDir(name)}}; !reflect.DeepEqual(actions, want) {
				t.Errorf("actions = %+v, want %+v", actions, want)
			}

			want := vaultSkill(t, src, name)
			got := vaultSkill(t, dst, name)
			if !got.Valid {
				t.Errorf("imported skill is invalid: %v", got.Problems)
			}
			if got.Hash != want.Hash {
				t.Errorf("hash = %s, want %s", got.Hash, want.Hash)
			}
			if wantExec := execFiles(t, want.Dir); !reflect.DeepEqual(execFiles(t, got.Dir), wantExec) {
				t.Errorf("file exec bits = %v, want %v", execFiles(t, got.Dir), wantExec)
			}
		})
	}
}

func TestExportImportRoundTripAllSkills(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	src := fixtureVault(t)
	dst := emptyVault(t)

	actions, err := Import(dst, exportTo(t, src, fixtureNames...), ImportOptions{})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if len(actions) != len(fixtureNames) {
		t.Fatalf("got %d actions, want %d", len(actions), len(fixtureNames))
	}
	for _, name := range fixtureNames {
		if got, want := vaultSkill(t, dst, name).Hash, vaultSkill(t, src, name).Hash; got != want {
			t.Errorf("%s hash = %s, want %s", name, got, want)
		}
	}
}

func TestExportIsDeterministic(t *testing.T) {
	v := fixtureVault(t)
	export := func(skills ...string) []byte {
		var buf bytes.Buffer
		if _, err := Export(v, skills, &buf); err != nil {
			t.Fatalf("Export() error = %v", err)
		}
		return buf.Bytes()
	}
	for _, skills := range [][]string{{"mocked-skill-00006"}, fixtureNames} {
		first := export(skills...)
		// Move every file's mtime, so the export must ignore mtimes to match.
		later := time.Now().Add(72 * time.Hour)
		err := filepath.WalkDir(v.Path, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			return os.Chtimes(p, later, later)
		})
		if err != nil {
			t.Fatal(err)
		}
		second := export(skills...)
		if !bytes.Equal(first, second) {
			t.Errorf("export of %v differs between two runs", skills)
		}
	}
}

func TestExportSelection(t *testing.T) {
	invalid := []string{
		"Invalid-Uppercase", "invalid-long-description", "invalid-missing-description",
		"invalid-name-mismatch", "invalid-no-frontmatter", "invalid-no-skill-md",
	}
	tests := []struct {
		name      string
		skills    []string
		wantErr   string
		wantSkips []string
		wantFiles []string // skills in the manifest of the archive
	}{
		{name: "no skills given is refused", wantErr: "no skills given"},
		{name: "name matching no skill is refused", skills: []string{"nope"}, wantErr: "no skill matches"},
		{name: "literal invalid skill is refused", skills: []string{"invalid-missing-description"}, wantErr: "invalid skill invalid-missing-description"},
		{name: "glob matching only invalid skills exports nothing", skills: []string{"invalid-*"}, wantErr: "no valid skill to export"},
		{
			name:      "glob skips invalid skills and reports them",
			skills:    []string{"*"},
			wantSkips: invalid,
			wantFiles: fixtureNames,
		},
		{
			name:      "literal and glob names select the same skill once",
			skills:    []string{"mocked-skill-00002", "mocked-skill-0000*"},
			wantFiles: fixtureNames,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			var buf bytes.Buffer
			skips, err := Export(v, tt.skills, &buf)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Export() error = %v, want one containing %q", err, tt.wantErr)
				}
				if buf.Len() != 0 {
					t.Errorf("refused export wrote %d bytes", buf.Len())
				}
				return
			}
			if err != nil {
				t.Fatalf("Export() error = %v", err)
			}

			var gotSkips []string
			for _, s := range skips {
				if s.Op != OpSkip || s.Reason != "invalid" {
					t.Errorf("skip = %+v, want an invalid OpSkip", s)
				}
				gotSkips = append(gotSkips, s.Skill)
			}
			slices.Sort(gotSkips)
			wantSkips := slices.Sorted(slices.Values(tt.wantSkips))
			if !slices.Equal(gotSkips, wantSkips) {
				t.Errorf("skips = %v, want %v", gotSkips, wantSkips)
			}

			m := archiveManifest(t, buf.Bytes())
			var gotFiles []string
			for name := range m.Skills {
				gotFiles = append(gotFiles, name)
			}
			slices.Sort(gotFiles)
			if !slices.Equal(gotFiles, tt.wantFiles) {
				t.Errorf("manifest skills = %v, want %v", gotFiles, tt.wantFiles)
			}
		})
	}
}

// archiveManifest reads the manifest of the tarball in data.
func archiveManifest(t *testing.T, data []byte) *manifest {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.skill.tar.gz")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := extractArchive(path, t.TempDir())
	if err != nil {
		t.Fatalf("extractArchive() error = %v", err)
	}
	if m == nil {
		t.Fatal("archive has no manifest")
	}
	return m
}

func TestExportName(t *testing.T) {
	now := time.Date(2026, 11, 21, 15, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		skills  []string
		now     time.Time
		want    string // the file name, or the skill name when hashed is set
		hashed  bool   // the name ends in the hash, not the version
		wantErr string
	}{
		{name: "one skill with a version uses it", skills: []string{"mocked-skill-00002"}, now: now, want: "mocked-skill-00002-1.2.0.skill.tar.gz"},
		{name: "one skill without a version uses the hash", skills: []string{"mocked-skill-00001"}, now: now, want: "mocked-skill-00001", hashed: true},
		{name: "several skills use the date", skills: []string{"mocked-skill-0000*"}, now: now, want: "skills-2026-11-21.skill.tar.gz"},
		{
			name:   "date is the UTC day",
			skills: []string{"mocked-skill-0000*"},
			now:    time.Date(2026, 11, 21, 23, 0, 0, 0, time.FixedZone("UTC-5", -5*3600)),
			want:   "skills-2026-11-22.skill.tar.gz",
		},
		{name: "no skills is refused", now: now, wantErr: "no skills given"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			got, err := ExportName(v, tt.skills, tt.now)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ExportName() error = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ExportName() error = %v", err)
			}
			want := tt.want
			if tt.hashed {
				want = tt.want + "-" + hashPrefix(vaultSkill(t, v, tt.want).Hash) + ".skill.tar.gz"
			}
			if got != want {
				t.Errorf("ExportName() = %q, want %q", got, want)
			}
		})
	}
}

func TestExportNameVersionWithSeparatorUsesHash(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	v := fixtureVault(t)
	content := "---\nname: slashed\ndescription: A skill.\nmetadata:\n  version: \"1/2\"\n---\n"
	if _, err := Create(v, []byte(content), Options{}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := ExportName(v, []string{"slashed"}, time.Now())
	if err != nil {
		t.Fatalf("ExportName() error = %v", err)
	}
	want := "slashed-" + hashPrefix(vaultSkill(t, v, "slashed").Hash) + ".skill.tar.gz"
	if got != want {
		t.Errorf("ExportName() = %q, want %q", got, want)
	}
}

// hashPrefix returns the first 8 hex digits of a "sha256:<hex>" hash.
func hashPrefix(hash string) string {
	_, hex, _ := strings.Cut(hash, ":")
	return hex[:8]
}

func TestExportFile(t *testing.T) {
	tests := []struct {
		name     string
		exists   bool   // the file is there before the export
		existing string // its content, when it exists
		skills   []string
		wantErr  bool
		wantFile bool // the file is there after the export
	}{
		{name: "writes a new file", skills: []string{"mocked-skill-00004"}, wantFile: true},
		{name: "refuses an existing file and keeps it", exists: true, existing: "keep", skills: []string{"mocked-skill-00004"}, wantErr: true, wantFile: true},
		{name: "a refused selection leaves no file", skills: []string{"no-such-skill"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			path := filepath.Join(t.TempDir(), "out.skill.tar.gz")
			if tt.exists {
				if err := os.WriteFile(path, []byte(tt.existing), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			_, err := ExportFile(v, tt.skills, path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ExportFile error = %v, want error %t", err, tt.wantErr)
			}
			data, readErr := os.ReadFile(path)
			if (readErr == nil) != tt.wantFile {
				t.Fatalf("file exists = %t, want %t", readErr == nil, tt.wantFile)
			}
			if tt.exists && string(data) != tt.existing {
				t.Errorf("existing file changed to %q", data)
			}
			if tt.wantFile && !tt.exists {
				var want bytes.Buffer
				if _, err := Export(v, tt.skills, &want); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(data, want.Bytes()) {
					t.Errorf("file differs from Export of the same skills")
				}
			}
		})
	}
}
