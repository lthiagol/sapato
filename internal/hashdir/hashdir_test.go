package hashdir

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"sapato/internal/testfixtures"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func mustHash(t *testing.T, dir string) string {
	t.Helper()
	h, err := Hash(dir)
	if err != nil {
		t.Fatalf("Hash(%s): %v", dir, err)
	}
	return h
}

func TestHashFormat(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"SKILL.md": "x"})
	h := mustHash(t, dir)
	if !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(h) {
		t.Errorf("hash %q does not match sha256:<64 hex>", h)
	}
}

func TestHashEmptyDir(t *testing.T) {
	empty := sha256.Sum256(nil)
	want := "sha256:" + hex.EncodeToString(empty[:])
	if got := mustHash(t, t.TempDir()); got != want {
		t.Errorf("empty dir hash = %s, want %s", got, want)
	}
}

func TestHashMatchesSpecEncoding(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"b.txt": "bee", "a/x.md": "ex"})
	if err := os.Chmod(filepath.Join(dir, "b.txt"), 0o755); err != nil {
		t.Fatal(err)
	}

	// relpath "\n" mode "\n" len "\n" content, files sorted by relpath.
	want := sha256.New()
	want.Write([]byte("a/x.md\n-\n2\nex"))
	want.Write([]byte("b.txt\nx\n3\nbee"))
	wantHex := "sha256:" + hex.EncodeToString(want.Sum(nil))

	if got := mustHash(t, dir); got != wantHex {
		t.Errorf("hash = %s, want %s", got, wantHex)
	}
}

func TestHashDeterministic(t *testing.T) {
	files := map[string]string{"SKILL.md": "a", "scripts/run.sh": "b", "references/r.md": "c"}
	d1, d2 := t.TempDir(), t.TempDir()
	writeFiles(t, d1, files)
	writeFiles(t, d2, files)
	h1 := mustHash(t, d1)
	if h2 := mustHash(t, d2); h1 != h2 {
		t.Errorf("same content, different dirs: %s != %s", h1, h2)
	}
	if h3 := mustHash(t, d1); h1 != h3 {
		t.Errorf("repeated hash differs: %s != %s", h1, h3)
	}
}

func TestHashSensitivity(t *testing.T) {
	base := map[string]string{"SKILL.md": "body", "scripts/run.sh": "echo"}
	tests := []struct {
		name     string
		mutate   func(t *testing.T, dir string)
		wantSame bool
	}{
		{
			name:     "content change",
			mutate:   func(t *testing.T, dir string) { writeFiles(t, dir, map[string]string{"SKILL.md": "other"}) },
			wantSame: false,
		},
		{
			name: "file renamed",
			mutate: func(t *testing.T, dir string) {
				if err := os.Rename(filepath.Join(dir, "SKILL.md"), filepath.Join(dir, "SKILL2.md")); err != nil {
					t.Fatal(err)
				}
			},
			wantSame: false,
		},
		{
			name: "file moved into subdir",
			mutate: func(t *testing.T, dir string) {
				if err := os.Rename(filepath.Join(dir, "scripts", "run.sh"), filepath.Join(dir, "run.sh")); err != nil {
					t.Fatal(err)
				}
			},
			wantSame: false,
		},
		{
			name: "exec bit added",
			mutate: func(t *testing.T, dir string) {
				if err := os.Chmod(filepath.Join(dir, "SKILL.md"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantSame: false,
		},
		{
			name: "dot file added",
			mutate: func(t *testing.T, dir string) {
				writeFiles(t, dir, map[string]string{".DS_Store": "junk", ".git/HEAD": "ref"})
			},
			wantSame: true,
		},
		{
			name: "dot file changed",
			mutate: func(t *testing.T, dir string) {
				writeFiles(t, dir, map[string]string{".hidden": "changed"})
			},
			wantSame: true,
		},
		{
			name: "symlink added",
			mutate: func(t *testing.T, dir string) {
				if err := os.Symlink(filepath.Join(dir, "SKILL.md"), filepath.Join(dir, "link.md")); err != nil {
					t.Fatal(err)
				}
			},
			wantSame: true,
		},
		{
			name: "symlinked dir added",
			mutate: func(t *testing.T, dir string) {
				if err := os.Symlink(filepath.Join(dir, "scripts"), filepath.Join(dir, "linked")); err != nil {
					t.Fatal(err)
				}
			},
			wantSame: true,
		},
	}

	baseHash := mustHash(t, func() string {
		d := t.TempDir()
		writeFiles(t, d, base)
		return d
	}())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, base)
			tt.mutate(t, dir)
			got := mustHash(t, dir)
			if same := got == baseHash; same != tt.wantSame {
				t.Errorf("same as base = %v, want %v", same, tt.wantSame)
			}
		})
	}
}

func TestHashNotADirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Hash(file); err == nil {
		t.Error("Hash(file) returned nil error")
	}
}

func TestHashMissingDir(t *testing.T) {
	if _, err := Hash(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("Hash(missing) returned nil error")
	}
}

// TestHashSkillDBFixtureIgnoresDotPathsAndSymlinks removes the dot paths and
// the symlink from mocked-skill-00006 and checks the hash does not change.
func TestHashSkillDBFixtureIgnoresDotPathsAndSymlinks(t *testing.T) {
	src := filepath.Join(testfixtures.Root(), "skill-db", "skills", "mocked-skill-00006")
	dir := testfixtures.CopyTree(t, src)
	want := mustHash(t, src)
	if got := mustHash(t, dir); got != want {
		t.Fatalf("copy hash = %s, want %s", got, want)
	}

	for _, rel := range []string{".DS_Store", ".hidden", "references/.cache", "link.md"} {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if _, err := os.Lstat(p); err != nil {
			t.Fatalf("fixture lacks %s: %v", rel, err)
		}
		if err := os.RemoveAll(p); err != nil {
			t.Fatal(err)
		}
	}

	if got := mustHash(t, dir); got != want {
		t.Errorf("hash after removing dot paths and symlink = %s, want %s", got, want)
	}
}

func TestFilesListsRegularFilesSorted(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"b.txt": "bee", "a/x.md": "ex", ".DS_Store": "junk", ".hidden/n.md": "n", "a/.cache/t.md": "t"})
	if err := os.Chmod(filepath.Join(dir, "b.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "b.txt"), filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}

	got, err := Files(dir)
	if err != nil {
		t.Fatalf("Files() error = %v", err)
	}
	want := []struct {
		rel  string
		mode fs.FileMode
	}{
		{"a/x.md", 0o644},
		{"b.txt", 0o755},
	}
	if len(got) != len(want) {
		t.Fatalf("Files() = %+v, want %d files", got, len(want))
	}
	for i, w := range want {
		if got[i].Rel != w.rel || got[i].Mode != w.mode {
			t.Errorf("file %d = {%s %v}, want {%s %v}", i, got[i].Rel, got[i].Mode, w.rel, w.mode)
		}
		if got[i].Path != filepath.Join(dir, filepath.FromSlash(w.rel)) {
			t.Errorf("file %d Path = %s", i, got[i].Path)
		}
	}
}

func TestFilesErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Files(file); err == nil {
		t.Error("Files(file) returned nil error")
	}
	if _, err := Files(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("Files(missing) returned nil error")
	}
}
