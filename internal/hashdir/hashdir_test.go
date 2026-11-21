package hashdir

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"testing"
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
