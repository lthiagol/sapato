package testfixtures

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"sapato/internal/hashdir"
)

func TestRootHasFixtures(t *testing.T) {
	for _, rel := range []string{"skill-db/sapato.toml", "targets/claude", "targets/agents", "targets/cursor"} {
		if _, err := os.Stat(filepath.Join(Root(), rel)); err != nil {
			t.Errorf("Root() missing %s: %v", rel, err)
		}
	}
}

func TestCopyTree(t *testing.T) {
	src := filepath.Join(Root(), "skill-db", "skills", "mocked-skill-00006")

	t.Run("preserves modes, dot paths and symlinks", func(t *testing.T) {
		dst := CopyTree(t, src)
		if filepath.Base(dst) != "mocked-skill-00006" {
			t.Errorf("base = %s, want mocked-skill-00006", filepath.Base(dst))
		}

		modes := map[string]fs.FileMode{
			"scripts/run.sh":    0o755,
			"scripts/check.py":  0o755,
			"scripts/README.md": 0o644,
			"references/api.md": 0o644,
			"assets/logo.png":   0o644,
		}
		for rel, want := range modes {
			info, err := os.Stat(filepath.Join(dst, filepath.FromSlash(rel)))
			if err != nil {
				t.Fatalf("%s: %v", rel, err)
			}
			if got := info.Mode().Perm(); got != want {
				t.Errorf("%s mode = %o, want %o", rel, got, want)
			}
		}

		for _, rel := range []string{".DS_Store", ".hidden/notes.md", "references/.cache/tmp.md"} {
			if _, err := os.Lstat(filepath.Join(dst, filepath.FromSlash(rel))); err != nil {
				t.Errorf("dot path %s not copied: %v", rel, err)
			}
		}

		link, err := os.Readlink(filepath.Join(dst, "link.md"))
		if err != nil {
			t.Fatalf("link.md is not a symlink: %v", err)
		}
		if link != "SKILL.md" {
			t.Errorf("link.md -> %q, want SKILL.md", link)
		}
	})

	t.Run("mutating the copy leaves the fixture alone", func(t *testing.T) {
		before, err := hashdir.Hash(src)
		if err != nil {
			t.Fatal(err)
		}
		dst := CopyTree(t, src)
		if err := os.WriteFile(filepath.Join(dst, "SKILL.md"), []byte("changed"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(filepath.Join(dst, "scripts")); err != nil {
			t.Fatal(err)
		}
		after, err := hashdir.Hash(src)
		if err != nil {
			t.Fatal(err)
		}
		if before != after {
			t.Errorf("fixture hash changed from %s to %s", before, after)
		}
	})
}

// lockFile is the subset of the lock format the consistency test reads.
type lockFile struct {
	Skills map[string]struct {
		Hash string `json:"hash"`
	} `json:"skills"`
}

func readLock(t *testing.T, agentDir string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(agentDir, ".sapato-lock.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}
		}
		t.Fatal(err)
	}
	var lf lockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		t.Fatalf("parse lock in %s: %v", agentDir, err)
	}
	hashes := make(map[string]string, len(lf.Skills))
	for name, e := range lf.Skills {
		hashes[name] = e.Hash
	}
	return hashes
}

// exists reports whether path is present, failing the test on other errors.
func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return true
	case os.IsNotExist(err):
		return false
	default:
		t.Fatal(err)
		return false
	}
}

// TestFixturesConsistent fails when the lock files under targets/ drift from
// the skill folders and the vault, so edits to fixtures must update the locks.
func TestFixturesConsistent(t *testing.T) {
	root := Root()
	vaultSkills := filepath.Join(root, "skill-db", "skills")
	targets := filepath.Join(root, "targets")

	// Each row is one (agent, skill) pair from spec §6. A status of "missing"
	// means the lock names the skill but its folder is gone.
	tests := []struct {
		agent, skill string
		inVault      bool
		inTarget     bool
		inLock       bool
		// Only checked when the relevant side exists.
		targetMatchesLock bool // copy hash == lock hash
		lockMatchesVault  bool // lock hash == vault hash
		status            string
	}{
		{agent: "claude", skill: "mocked-skill-00001", inVault: true, inTarget: true, inLock: true, targetMatchesLock: true, lockMatchesVault: true, status: "synced"},
		{agent: "claude", skill: "mocked-skill-00002", inVault: true, inTarget: true, inLock: true, targetMatchesLock: true, lockMatchesVault: false, status: "outdated"},
		{agent: "claude", skill: "mocked-skill-00003", inVault: true, inTarget: true, inLock: true, targetMatchesLock: false, lockMatchesVault: true, status: "modified"},
		{agent: "claude", skill: "mocked-skill-00099", inVault: false, inTarget: true, inLock: true, targetMatchesLock: true, status: "orphaned"},
		{agent: "claude", skill: "mocked-skill-00098", inVault: false, inTarget: false, inLock: true, status: "missing"},
		{agent: "claude", skill: "hand-made-skill", inVault: false, inTarget: true, inLock: false, status: "unmanaged"},
		{agent: "agents", skill: "mocked-skill-00001", inVault: true, inTarget: true, inLock: true, targetMatchesLock: true, lockMatchesVault: true, status: "synced"},
		// Not compatible with agents (sapato.toml), installed anyway: synced, compatible: false.
		{agent: "agents", skill: "mocked-skill-00005", inVault: true, inTarget: true, inLock: true, targetMatchesLock: true, lockMatchesVault: true, status: "synced"},
	}

	for _, tt := range tests {
		t.Run(tt.agent+"/"+tt.skill+" ("+tt.status+")", func(t *testing.T) {
			agentDir := filepath.Join(targets, tt.agent)
			skillDir := filepath.Join(agentDir, tt.skill)
			vaultDir := filepath.Join(vaultSkills, tt.skill)

			if got := exists(t, vaultDir); got != tt.inVault {
				t.Fatalf("in vault = %v, want %v", got, tt.inVault)
			}
			if got := exists(t, skillDir); got != tt.inTarget {
				t.Fatalf("in target = %v, want %v", got, tt.inTarget)
			}
			lock := readLock(t, agentDir)
			lockHash, got := lock[tt.skill]
			if got != tt.inLock {
				t.Fatalf("in lock = %v, want %v", got, tt.inLock)
			}

			if tt.inTarget && tt.inLock {
				h, err := hashdir.Hash(CopyTree(t, skillDir))
				if err != nil {
					t.Fatal(err)
				}
				if (h == lockHash) != tt.targetMatchesLock {
					t.Errorf("target hash %s vs lock %s: equal = %v, want %v", h, lockHash, h == lockHash, tt.targetMatchesLock)
				}
			}
			if tt.inVault && tt.inLock {
				h, err := hashdir.Hash(vaultDir)
				if err != nil {
					t.Fatal(err)
				}
				if (h == lockHash) != tt.lockMatchesVault {
					t.Errorf("vault hash %s vs lock %s: equal = %v, want %v", h, lockHash, h == lockHash, tt.lockMatchesVault)
				}
			}
		})
	}

	// Nothing may appear in a target or lock beyond the rows above.
	for _, agent := range []string{"claude", "agents", "cursor"} {
		t.Run("no extra entries in "+agent, func(t *testing.T) {
			var wantLock, wantFolders []string
			for _, tt := range tests {
				if tt.agent != agent {
					continue
				}
				if tt.inLock {
					wantLock = append(wantLock, tt.skill)
				}
				if tt.inTarget {
					wantFolders = append(wantFolders, tt.skill)
				}
			}

			var gotLock []string
			for name := range readLock(t, filepath.Join(targets, agent)) {
				gotLock = append(gotLock, name)
			}
			entries, err := os.ReadDir(filepath.Join(targets, agent))
			if err != nil {
				t.Fatal(err)
			}
			var gotFolders []string
			for _, e := range entries {
				if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
					gotFolders = append(gotFolders, e.Name())
				}
			}

			slices.Sort(wantLock)
			slices.Sort(gotLock)
			if !reflect.DeepEqual(gotLock, wantLock) {
				t.Errorf("lock entries = %v, want %v", gotLock, wantLock)
			}
			slices.Sort(wantFolders)
			slices.Sort(gotFolders)
			if !reflect.DeepEqual(gotFolders, wantFolders) {
				t.Errorf("folders = %v, want %v", gotFolders, wantFolders)
			}
		})
	}
}
