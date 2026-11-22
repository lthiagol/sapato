package target

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"sapato/internal/testfixtures"
)

var installedAt = time.Date(2026, 11, 21, 13, 0, 0, 0, time.UTC)

func writeLockFile(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, lockName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadLock(t *testing.T) {
	tests := []struct {
		name     string
		content  string // empty means no lock file
		want     Lock
		wantErr  string
		wantFile bool // error must name the lock path
	}{
		{
			name: "missing file is empty lock",
			want: Lock{Version: 1, Skills: map[string]LockEntry{}},
		},
		{
			name:    "version 1 without skills",
			content: `{"version": 1}`,
			want:    Lock{Version: 1, Skills: map[string]LockEntry{}},
		},
		{
			name:    "entries",
			content: `{"version":1,"skills":{"code-review":{"hash":"sha256:ab","version":"1.2.0","installed_at":"2026-11-21T13:00:00Z"}}}`,
			want: Lock{Version: 1, Skills: map[string]LockEntry{
				"code-review": {Hash: "sha256:ab", Version: "1.2.0", InstalledAt: installedAt},
			}},
		},
		{
			name:     "corrupt JSON names the file",
			content:  `{"version": 1,`,
			wantErr:  "parse lock",
			wantFile: true,
		},
		{
			name:     "empty file is corrupt",
			content:  "",
			wantErr:  "parse lock",
			wantFile: true,
		},
		{
			name:     "bad installed_at is corrupt",
			content:  `{"version":1,"skills":{"a":{"hash":"x","version":"","installed_at":"yesterday"}}}`,
			wantErr:  "parse lock",
			wantFile: true,
		},
		{
			name:     "unsupported version",
			content:  `{"version": 2}`,
			wantErr:  "unsupported version 2",
			wantFile: true,
		},
		{
			name:     "missing version",
			content:  `{"skills": {}}`,
			wantErr:  "unsupported version 0",
			wantFile: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, lockName)
			if tt.content != "" || tt.wantErr != "" {
				writeLockFile(t, dir, tt.content)
			}

			got, err := ReadLock(dir)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				if tt.wantFile && !strings.Contains(err.Error(), path) {
					t.Errorf("error = %v, want it to name %s", err, path)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadLock() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReadLock() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestWriteLock(t *testing.T) {
	t.Run("round trip writes version 1 and keeps empty version strings", func(t *testing.T) {
		dir := t.TempDir()
		in := Lock{Skills: map[string]LockEntry{
			"plain":  {Hash: "sha256:01", InstalledAt: installedAt},
			"tagged": {Hash: "sha256:02", Version: "2.0.0", InstalledAt: installedAt},
		}}
		if err := WriteLock(dir, in); err != nil {
			t.Fatalf("WriteLock() error = %v", err)
		}
		data, err := os.ReadFile(filepath.Join(dir, lockName))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`"version": 1`, `"version": ""`, `"installed_at": "2026-11-21T13:00:00Z"`} {
			if !strings.Contains(string(data), want) {
				t.Errorf("lock missing %s:\n%s", want, data)
			}
		}
		got, err := ReadLock(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, Lock{Version: 1, Skills: in.Skills}) {
			t.Errorf("read back = %+v", got)
		}
	})

	t.Run("nil skills writes an empty object", func(t *testing.T) {
		dir := t.TempDir()
		if err := WriteLock(dir, Lock{}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(dir, lockName))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), `"skills": {}`) {
			t.Errorf("lock = %s, want empty skills object", data)
		}
	})

	t.Run("leaves no temp files behind", func(t *testing.T) {
		dir := t.TempDir()
		for _, hash := range []string{"sha256:01", "sha256:02"} {
			if err := WriteLock(dir, Lock{Skills: map[string]LockEntry{"a": {Hash: hash}}}); err != nil {
				t.Fatal(err)
			}
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || entries[0].Name() != lockName {
			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
			}
			t.Errorf("dir entries = %v, want only %s", names, lockName)
		}
	})

	t.Run("unchanged contents do not rewrite the file", func(t *testing.T) {
		dir := testfixtures.CopyTree(t, filepath.Join(testfixtures.Root(), "targets", "claude"))
		lock, err := ReadLock(dir)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, lockName)
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		// Age the file so a rewrite would be visible as a new mtime.
		old := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}

		if err := WriteLock(dir, lock); err != nil {
			t.Fatalf("WriteLock() error = %v", err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(old) {
			t.Errorf("mtime = %v, want untouched %v", info.ModTime(), old)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Errorf("fixture lock re-encoded differently:\n%s", after)
		}
	})

	t.Run("changed contents are written", func(t *testing.T) {
		dir := t.TempDir()
		if err := WriteLock(dir, Lock{}); err != nil {
			t.Fatal(err)
		}
		next := Lock{Skills: map[string]LockEntry{"a": {Hash: "sha256:aa", InstalledAt: installedAt}}}
		if err := WriteLock(dir, next); err != nil {
			t.Fatal(err)
		}
		got, err := ReadLock(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Skills, next.Skills) {
			t.Errorf("skills = %+v, want %+v", got.Skills, next.Skills)
		}
	})

	t.Run("missing target dir is an error", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "absent")
		err := WriteLock(dir, Lock{})
		if err == nil || !strings.Contains(err.Error(), filepath.Join(dir, lockName)) {
			t.Errorf("error = %v, want one naming the lock path", err)
		}
	})
}

func TestLockExists(t *testing.T) {
	tests := []struct {
		name    string
		content string // empty means no lock file
		want    bool
	}{
		{name: "missing file", want: false},
		{name: "present file", content: `{"version":1,"skills":{}}`, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.content != "" {
				writeLockFile(t, dir, tt.content)
			}
			got, err := LockExists(dir)
			if err != nil {
				t.Fatalf("LockExists() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("LockExists() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestRemoveLock(t *testing.T) {
	tests := []struct {
		name    string
		content string // empty means no lock file
	}{
		{name: "present file is removed", content: `{"version":1,"skills":{}}`},
		{name: "missing file is not an error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.content != "" {
				writeLockFile(t, dir, tt.content)
			}
			if err := RemoveLock(dir); err != nil {
				t.Fatalf("RemoveLock() error = %v", err)
			}
			if got, err := LockExists(dir); err != nil || got {
				t.Errorf("after RemoveLock: LockExists() = %t, %v; want absent", got, err)
			}
		})
	}
}
