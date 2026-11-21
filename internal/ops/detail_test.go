package ops

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"sapato/internal/target"
	"sapato/internal/testfixtures"
)

func TestSkillFiles(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		modes map[string]os.FileMode
		want  []File
	}{
		{
			name:  "sorted, sizes and exec bits",
			files: map[string]string{"SKILL.md": "abc", "scripts/run.sh": "echo\n", "references/api.md": ""},
			modes: map[string]os.FileMode{"scripts/run.sh": 0o755},
			want: []File{
				{Path: "SKILL.md", Size: 3},
				{Path: "references/api.md", Size: 0},
				{Path: "scripts/run.sh", Size: 5, Exec: true},
			},
		},
		{
			name:  "dot paths are left out",
			files: map[string]string{"SKILL.md": "a", ".DS_Store": "junk", ".hidden/notes.md": "n"},
			want:  []File{{Path: "SKILL.md", Size: 1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for rel, content := range tt.files {
				p := filepath.Join(dir, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for rel, mode := range tt.modes {
				if err := os.Chmod(filepath.Join(dir, filepath.FromSlash(rel)), mode); err != nil {
					t.Fatal(err)
				}
			}
			got, err := SkillFiles(dir)
			if err != nil {
				t.Fatalf("SkillFiles: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SkillFiles = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSkillFilesOfFixtureSkills(t *testing.T) {
	root := filepath.Join(testfixtures.Root(), "skill-db", "skills")
	tests := []struct {
		skill string
		want  []string
	}{
		{"mocked-skill-00001", []string{"SKILL.md"}},
		{"mocked-skill-00004", []string{"SKILL.md", "scripts/README.md", "scripts/check.py", "scripts/run.sh"}},
		{"mocked-skill-00006", []string{
			"SKILL.md", "assets/logo.png", "assets/template.txt", "references/api.md",
			"references/guides/advanced.md", "references/overview.md",
			"scripts/README.md", "scripts/check.py", "scripts/run.sh",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.skill, func(t *testing.T) {
			got, err := SkillFiles(filepath.Join(root, tt.skill))
			if err != nil {
				t.Fatalf("SkillFiles: %v", err)
			}
			var paths []string
			for _, f := range got {
				paths = append(paths, f.Path)
			}
			if !reflect.DeepEqual(paths, tt.want) {
				t.Errorf("paths = %v, want %v", paths, tt.want)
			}
		})
	}
}

func TestSkillFilesErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		dir  string
	}{
		{"missing dir", filepath.Join(t.TempDir(), "absent")},
		{"not a directory", file},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := SkillFiles(tt.dir); err == nil {
				t.Errorf("SkillFiles(%s) returned no error", tt.dir)
			}
		})
	}
}

func TestLockEntry(t *testing.T) {
	installed := time.Date(2026, 11, 21, 13, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		lock    string // the lock file's content, "" for no file
		skill   string
		wantOK  bool
		wantErr bool
		want    target.LockEntry
	}{
		{
			name:   "managed skill",
			lock:   `{"version":1,"skills":{"alpha":{"hash":"sha256:h1","version":"1.2.0","installed_at":"2026-11-21T13:00:00Z"}}}`,
			skill:  "alpha",
			wantOK: true,
			want:   target.LockEntry{Hash: "sha256:h1", Version: "1.2.0", InstalledAt: installed},
		},
		{
			name:   "skill not in the lock",
			lock:   `{"version":1,"skills":{"alpha":{"hash":"sha256:h1"}}}`,
			skill:  "beta",
			wantOK: false,
		},
		{
			name:   "no lock file",
			skill:  "alpha",
			wantOK: false,
		},
		{
			name:    "corrupt lock",
			lock:    `{"version":`,
			skill:   "alpha",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.lock != "" {
				if err := os.WriteFile(filepath.Join(dir, ".sapato-lock.json"), []byte(tt.lock), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, ok, err := LockEntry(dir, tt.skill)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %t", err, tt.wantErr)
			}
			if ok != tt.wantOK {
				t.Errorf("ok = %t, want %t", ok, tt.wantOK)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("entry = %+v, want %+v", got, tt.want)
			}
		})
	}
}
