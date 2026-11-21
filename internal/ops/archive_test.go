package ops

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"sapato/internal/vault"
)

// tarEntry is one entry of a test tarball. A zero typeflag means a regular file,
// and a zero mode means 0644.
type tarEntry struct {
	name     string
	typeflag byte
	body     []byte
	mode     int64
	link     string
}

// makeTarball writes entries as a gzip tarball into a new file and returns its path.
func makeTarball(t *testing.T, entries ...tarEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.skill.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typ := e.typeflag
		if typ == 0 {
			typ = tar.TypeReg
		}
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		hdr := &tar.Header{Name: e.name, Typeflag: typ, Mode: mode, Linkname: e.link}
		if typ == tar.TypeXGlobalHeader {
			// The writer takes a global header from its pax records alone.
			hdr = &tar.Header{Name: e.name, Typeflag: typ, PAXRecords: map[string]string{"comment": string(e.body)}}
		}
		if typ == tar.TypeReg {
			hdr.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			if _, err := tw.Write(e.body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// skillEntries returns a valid skill called name with a script that is executable.
func skillEntries(name string) []tarEntry {
	return []tarEntry{
		{name: name + "/", typeflag: tar.TypeDir},
		{name: name + "/SKILL.md", body: []byte("---\nname: " + name + "\ndescription: From a tarball.\n---\n")},
		{name: name + "/scripts/", typeflag: tar.TypeDir},
		{name: name + "/scripts/run.sh", body: []byte("#!/bin/sh\n"), mode: 0o755},
	}
}

// gitGlobalHeader returns the pax global header that git archive writes first. Its
// body is the commit id, stored as the "comment" record.
func gitGlobalHeader() tarEntry {
	return tarEntry{name: "pax_global_header", typeflag: tar.TypeXGlobalHeader, body: []byte(strings.Repeat("a", 40))}
}

// manifestEntry returns a manifest entry with the given content.
func manifestEntry(content string) tarEntry {
	return tarEntry{name: manifestName, body: []byte(content)}
}

func TestImportArchive(t *testing.T) {
	entries := func(es ...[]tarEntry) func() []tarEntry {
		return func() []tarEntry {
			var all []tarEntry
			for _, e := range es {
				all = append(all, e...)
			}
			return all
		}
	}
	tests := []struct {
		name       string
		entries    func() []tarEntry
		opts       ImportOptions
		wantSkills []string
		wantErr    string
		needsForce bool
		check      func(t *testing.T, v *vault.Vault)
	}{
		{
			name:       "single skill is imported",
			entries:    entries(skillEntries("tar-skill")),
			wantSkills: []string{"tar-skill"},
		},
		{
			name:       "several skills are imported together",
			entries:    entries(skillEntries("tar-one"), skillEntries("tar-two")),
			wantSkills: []string{"tar-one", "tar-two"},
		},
		{
			name: "dot paths are skipped as hashing skips them",
			entries: entries(skillEntries("tar-skill"), []tarEntry{
				{name: "tar-skill/.DS_Store", body: []byte("x")},
				{name: "tar-skill/.hidden/", typeflag: tar.TypeDir},
				{name: "tar-skill/.hidden/secret", body: []byte("x")},
				{name: ".DS_Store", body: []byte("x")},
			}),
			wantSkills: []string{"tar-skill"},
			check: func(t *testing.T, v *vault.Vault) {
				for _, p := range []string{".DS_Store", ".hidden"} {
					if _, err := os.Stat(filepath.Join(v.SkillDir("tar-skill"), p)); !os.IsNotExist(err) {
						t.Errorf("%s was extracted (err %v)", p, err)
					}
				}
			},
		},
		{
			name:       "name matching a single-skill archive is accepted",
			entries:    entries(skillEntries("tar-skill")),
			opts:       ImportOptions{Name: "tar-skill"},
			wantSkills: []string{"tar-skill"},
		},
		{
			name:    "name differing from a single-skill archive is refused",
			entries: entries(skillEntries("tar-skill")),
			opts:    ImportOptions{Name: "other"},
			wantErr: `name "other" does not match archive skill "tar-skill"`,
		},
		{
			name:    "name with a multi-skill archive is refused",
			entries: entries(skillEntries("tar-one"), skillEntries("tar-two")),
			opts:    ImportOptions{Name: "tar-one"},
			wantErr: "single-skill archive",
		},
		{
			name:    "subdir with a tarball is refused",
			entries: entries(skillEntries("tar-skill")),
			opts:    ImportOptions{Subdir: "tar-skill"},
			wantErr: "subdir does not apply to a tarball",
		},
		{
			name: "invalid skill among several imports nothing",
			entries: entries(skillEntries("tar-one"), []tarEntry{
				{name: "tar-two/", typeflag: tar.TypeDir},
				{name: "tar-two/SKILL.md", body: []byte("---\nname: tar-two\n---\n")},
			}),
			wantErr: "invalid skill tar-two",
		},
		{
			name: "folder name differing from the frontmatter is invalid",
			entries: entries([]tarEntry{
				{name: "tar-skill/", typeflag: tar.TypeDir},
				{name: "tar-skill/SKILL.md", body: []byte("---\nname: other\ndescription: A.\n---\n")},
			}),
			wantErr: "invalid skill tar-skill",
		},
		{
			name: "folder without SKILL.md is invalid",
			entries: entries([]tarEntry{
				{name: "tar-skill/", typeflag: tar.TypeDir},
				{name: "tar-skill/notes.md", body: []byte("x")},
			}),
			wantErr: "SKILL.md is missing",
		},
		{
			name:    "archive without skills is refused",
			entries: func() []tarEntry { return nil },
			wantErr: "holds no skills",
		},
		{
			name:    "archive with only a manifest is refused",
			entries: entries([]tarEntry{manifestEntry(`{"format":1,"skills":{}}`)}),
			wantErr: "holds no skills",
		},
		{
			name:       "git pax global header is skipped",
			entries:    entries([]tarEntry{gitGlobalHeader()}, skillEntries("tar-skill")),
			wantSkills: []string{"tar-skill"},
		},
		{
			name: "pax global header counts toward the entry cap",
			entries: func() []tarEntry {
				es := []tarEntry{gitGlobalHeader()}
				for i := range maxArchiveEntries {
					es = append(es, tarEntry{name: fmt.Sprintf("d%05d/", i), typeflag: tar.TypeDir})
				}
				return es
			},
			wantErr: "more than 10000 entries",
		},
		{
			name:    "absolute path is refused",
			entries: entries([]tarEntry{{name: "/evil/SKILL.md", body: []byte("x")}}),
			wantErr: "absolute path",
		},
		{
			name:    "dot-dot component is refused",
			entries: entries([]tarEntry{{name: "tar-skill/../../evil", body: []byte("x")}}),
			wantErr: ".. component",
		},
		{
			name:    "leading ./ is refused",
			entries: entries([]tarEntry{{name: "./tar-skill/SKILL.md", body: []byte("x")}}),
			wantErr: "empty or . component",
		},
		{
			name:    "empty path part is refused",
			entries: entries([]tarEntry{{name: "tar-skill//SKILL.md", body: []byte("x")}}),
			wantErr: "empty or . component",
		},
		{
			name:    "backslash in path is refused",
			entries: entries([]tarEntry{{name: `tar-skill\..\evil`, body: []byte("x")}}),
			wantErr: "backslash",
		},
		{
			name:    "symlink is refused",
			entries: entries([]tarEntry{{name: "tar-skill/link.md", typeflag: tar.TypeSymlink, link: "/etc/passwd"}}),
			wantErr: "is not allowed",
		},
		{
			name: "hardlink is refused",
			entries: entries(skillEntries("tar-skill"), []tarEntry{
				{name: "tar-skill/hard.md", typeflag: tar.TypeLink, link: "tar-skill/SKILL.md"},
			}),
			wantErr: "is not allowed",
		},
		{
			name:    "character device is refused",
			entries: entries([]tarEntry{{name: "tar-skill/null", typeflag: tar.TypeChar}}),
			wantErr: "is not allowed",
		},
		{
			name:    "top-level file is refused",
			entries: entries(skillEntries("tar-skill"), []tarEntry{{name: "README.md", body: []byte("x")}}),
			wantErr: "top-level files are not allowed",
		},
		{
			name: "duplicate file is refused",
			entries: entries([]tarEntry{
				{name: "tar-skill/SKILL.md", body: []byte("---\nname: tar-skill\ndescription: A.\n---\n")},
				{name: "tar-skill/SKILL.md", body: []byte("---\nname: tar-skill\ndescription: B.\n---\n")},
			}),
			wantErr: "file exists",
		},
		{
			name: "more than the entry cap is refused",
			entries: func() []tarEntry {
				es := make([]tarEntry, maxArchiveEntries+1)
				for i := range es {
					es[i] = tarEntry{name: fmt.Sprintf("d%05d/", i), typeflag: tar.TypeDir}
				}
				return es
			},
			wantErr: "more than 10000 entries",
		},
		{
			name: "more than the size cap is refused",
			entries: func() []tarEntry {
				return []tarEntry{{name: "tar-skill/SKILL.md", body: make([]byte, maxArchiveBytes+1)}}
			},
			wantErr: "more than 52428800 bytes",
		},
		{
			name:       "skill already in the vault is refused",
			entries:    entries(skillEntries("mocked-skill-00004")),
			wantErr:    "already in the vault (use --force): mocked-skill-00004",
			needsForce: true,
		},
		{
			name:       "force replaces skills already in the vault",
			entries:    entries(skillEntries("mocked-skill-00004")),
			opts:       ImportOptions{Options: Options{Force: true}},
			wantSkills: []string{"mocked-skill-00004"},
		},
		{
			name:    "manifest with a wrong hash is refused",
			entries: entries(skillEntries("tar-skill"), []tarEntry{manifestEntry(`{"format":1,"skills":{"tar-skill":"sha256:00"}}`)}),
			wantErr: "does not match skill tar-skill",
		},
		{
			name:    "manifest missing a skill in the archive is refused",
			entries: entries(skillEntries("tar-skill"), []tarEntry{manifestEntry(`{"format":1,"skills":{}}`)}),
			wantErr: "does not list the skills",
		},
		{
			name:    "manifest in another format is refused",
			entries: entries(skillEntries("tar-skill"), []tarEntry{manifestEntry(`{"format":2,"skills":{}}`)}),
			wantErr: "unsupported format 2",
		},
		{
			name:    "malformed manifest is refused",
			entries: entries(skillEntries("tar-skill"), []tarEntry{manifestEntry(`{"format":`)}),
			wantErr: manifestName,
		},
		{
			name:    "duplicate manifest is refused",
			entries: entries([]tarEntry{manifestEntry(`{"format":1,"skills":{}}`), manifestEntry(`{"format":1,"skills":{}}`)}),
			wantErr: "duplicate manifest",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("TMPDIR", tmp)
			v := fixtureVault(t)
			before := snapshot(t, v.Path)
			src := makeTarball(t, tt.entries()...)

			actions, err := Import(v, src, tt.opts)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Import() error = %v, want one containing %q", err, tt.wantErr)
				}
				if got := errors.Is(err, ErrNeedsForce); got != tt.needsForce {
					t.Errorf("errors.Is(err, ErrNeedsForce) = %v, want %v", got, tt.needsForce)
				}
				if after := snapshot(t, v.Path); !reflect.DeepEqual(after, before) {
					t.Error("vault changed on a refused import")
				}
				assertTempEmpty(t, tmp)
				return
			}
			if err != nil {
				t.Fatalf("Import() error = %v", err)
			}

			var gotSkills []string
			for _, a := range actions {
				if a.Op != OpImport {
					t.Errorf("action op = %q, want %q", a.Op, OpImport)
				}
				gotSkills = append(gotSkills, a.Skill)
			}
			if !slices.Equal(gotSkills, tt.wantSkills) {
				t.Errorf("imported %v, want %v", gotSkills, tt.wantSkills)
			}
			for _, name := range tt.wantSkills {
				if got := vaultSkill(t, v, name); !got.Valid {
					t.Errorf("imported %s is invalid: %v", name, got.Problems)
				}
			}
			if tt.check != nil {
				tt.check(t, v)
			}
			assertTempEmpty(t, tmp)
		})
	}
}

// assertTempEmpty fails when the import left something in its temp dir.
func assertTempEmpty(t *testing.T, tmp string) {
	t.Helper()
	left, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("import left %d entries in its temp dir", len(left))
	}
}

func TestImportArchiveDryRunChangesNothing(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	v := fixtureVault(t)
	before := snapshot(t, v.Path)
	src := makeTarball(t, append(skillEntries("tar-one"), skillEntries("mocked-skill-00004")...)...)

	actions, err := Import(v, src, ImportOptions{Options: Options{Force: true, DryRun: true}})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if len(actions) != 2 || actions[0].Skill != "mocked-skill-00004" || actions[1].Skill != "tar-one" {
		t.Errorf("actions = %+v, want the two skills in name order", actions)
	}
	if after := snapshot(t, v.Path); !reflect.DeepEqual(after, before) {
		t.Error("vault changed on a dry run")
	}
}

func TestImportArchiveRollsBackWhenACopyFails(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	v := fixtureVault(t)
	before := snapshot(t, v.Path)
	src := makeTarball(t, append(skillEntries("mocked-skill-00001"), skillEntries("mocked-skill-00002")...)...)

	realRename := renamePath
	renamePath = func(oldPath, newPath string) error {
		if filepath.Base(newPath) == "mocked-skill-00002" {
			return errors.New("injected rename failure")
		}
		return realRename(oldPath, newPath)
	}
	t.Cleanup(func() { renamePath = realRename })

	_, err := Import(v, src, ImportOptions{Options: Options{Force: true}})
	if err == nil || !strings.Contains(err.Error(), "injected rename failure") {
		t.Fatalf("Import() error = %v, want the injected rename failure", err)
	}
	if after := snapshot(t, v.Path); !reflect.DeepEqual(after, before) {
		t.Error("failed archive import changed the vault")
	}
}

func TestImportArchiveNotGzip(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	src := filepath.Join(t.TempDir(), "plain.tar.gz")
	writeTestFile(t, src, "not gzip")

	_, err := Import(fixtureVault(t), src, ImportOptions{})
	if err == nil || !strings.Contains(err.Error(), "read gzip") {
		t.Fatalf("Import() error = %v, want a gzip error", err)
	}
}

func TestImportArchiveManifestFromExport(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	src := fixtureVault(t)
	path := exportTo(t, src, "mocked-skill-00003", "mocked-skill-00005")

	m := archiveManifest(t, mustRead(t, path))
	if m.Format != manifestFormat || len(m.Skills) != 2 {
		t.Fatalf("manifest = %+v, want format 1 with two skills", m)
	}
	if m.Skills["mocked-skill-00003"] != vaultSkill(t, src, "mocked-skill-00003").Hash {
		t.Errorf("manifest hash for mocked-skill-00003 = %s", m.Skills["mocked-skill-00003"])
	}

	dst := emptyVault(t)
	if _, err := Import(dst, path, ImportOptions{}); err != nil {
		t.Fatalf("Import() error = %v", err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestIsArchiveIgnoresCase(t *testing.T) {
	tests := []struct {
		src  string
		want bool
	}{
		{"skill.tar.gz", true},
		{"skill.tgz", true},
		{"SKILL.TGZ", true},
		{"Skill.TAR.GZ", true},
		{"skill.tar", false},
		{"skill.zip", false},
	}
	for _, tt := range tests {
		if got := isArchive(tt.src); got != tt.want {
			t.Errorf("isArchive(%q) = %t, want %t", tt.src, got, tt.want)
		}
	}
}

func TestImportArchiveWithUppercaseExtension(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	v := emptyVault(t)
	src := filepath.Join(t.TempDir(), "Skill.TAR.GZ")
	if err := os.Rename(makeTarball(t, skillEntries("tar-skill")...), src); err != nil {
		t.Fatal(err)
	}

	actions, err := Import(v, src, ImportOptions{})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if len(actions) != 1 || actions[0].Skill != "tar-skill" {
		t.Errorf("actions = %+v, want one import of tar-skill", actions)
	}
}
