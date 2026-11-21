package target

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"sapato/internal/testfixtures"
)

func TestScan(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string) // nil for a missing dir
		want  []string
	}{
		{
			name:  "missing dir has no skills",
			setup: nil,
			want:  nil,
		},
		{
			name: "empty dir",
			want: nil,
		},
		{
			name: "folders only, sorted, dot entries and files skipped",
			setup: func(t *testing.T, dir string) {
				mkdirs(t, dir, "zeta", "alpha", ".hidden", "mid")
				writeFile(t, filepath.Join(dir, "loose.md"), "x")
				writeFile(t, filepath.Join(dir, ".sapato-lock.json"), "{}")
			},
			want: []string{"alpha", "mid", "zeta"},
		},
		{
			name: "symlinked folder is skipped",
			setup: func(t *testing.T, dir string) {
				mkdirs(t, dir, "real")
				if err := os.Symlink("real", filepath.Join(dir, "linked")); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"real"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "skills")
			if tt.setup != nil {
				if err := os.Mkdir(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				tt.setup(t, dir)
			}
			got, err := Scan(dir)
			if err != nil {
				t.Fatalf("Scan() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Scan() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("fixture target", func(t *testing.T) {
		dir := testfixtures.CopyTree(t, filepath.Join(testfixtures.Root(), "targets", "claude"))
		got, err := Scan(dir)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"hand-made-skill", "mocked-skill-00001", "mocked-skill-00002", "mocked-skill-00003", "mocked-skill-00099"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Scan() = %v, want %v (the lock-only 00098 has no folder)", got, want)
		}
	})

	t.Run("path that is a file is an error", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "file")
		writeFile(t, p, "x")
		if _, err := Scan(p); err == nil {
			t.Error("Scan() on a file: error = nil")
		}
	})
}

func mkdirs(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.Mkdir(filepath.Join(dir, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
