package ops

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"sapato/internal/config"
	"sapato/internal/hashdir"
	"sapato/internal/target"
	"sapato/internal/testfixtures"
	"sapato/internal/vault"
)

// adoptNames are the skills of the adopt actions in acts, in order.
func adoptNames(acts []Action) []string {
	var names []string
	for _, a := range acts {
		if a.Op == OpAdopt {
			names = append(names, a.Skill+"@"+a.Agent)
		}
	}
	return names
}

// checkAdopted checks that each skill in names is in the vault, valid, locked in
// agent and synced there, with the same hash in the agent folder.
func checkAdopted(t *testing.T, v *vault.Vault, agent config.Agent, names []string) {
	t.Helper()
	for _, name := range names {
		got := vaultSkill(t, v, name)
		if !got.Valid {
			t.Errorf("adopted %s is invalid: %v", name, got.Problems)
		}
		lock, err := target.ReadLock(agent.Path)
		if err != nil {
			t.Fatal(err)
		}
		if entry, ok := lock.Skills[name]; !ok || entry.Hash != got.Hash {
			t.Errorf("%s lock entry = %+v (present %v), want hash %s", name, entry, ok, got.Hash)
		}
		if st := stateOf(t, v, agent, name); st != Synced {
			t.Errorf("%s status after adopt = %s, want synced", name, st)
		}
		if h, err := hashdir.Hash(filepath.Join(agent.Path, name)); err != nil || h != got.Hash {
			t.Errorf("%s target hash = %s (err %v), want vault hash %s", name, h, err, got.Hash)
		}
	}
}

func TestAdopt(t *testing.T) {
	tests := []struct {
		name      string
		skills    []string
		prep      func(t *testing.T, agent config.Agent)
		opts      Options
		want      []string // the adopted skills, in order
		wantSkips []string // the skipped skills, in order
		wantErr   string
	}{
		{
			name:   "unmanaged folder is copied into the vault and locked",
			skills: []string{"hand-made-skill"},
			want:   []string{"hand-made-skill"},
		},
		{
			name:    "managed skill is refused",
			skills:  []string{"mocked-skill-00001"},
			wantErr: "synced, want unmanaged",
		},
		{
			name:    "folder missing from the agent is refused",
			skills:  []string{"absent-skill"},
			wantErr: "not-installed, want unmanaged",
		},
		{
			name:    "name that is not a folder name is refused",
			skills:  []string{"../hand-made-skill"},
			wantErr: "not a skill folder name",
		},
		{
			name:   "invalid unmanaged folder is refused",
			skills: []string{"broken"},
			prep: func(t *testing.T, agent config.Agent) {
				writeTestFile(t, filepath.Join(agent.Path, "broken", "SKILL.md"), "# no frontmatter\n")
			},
			wantErr: "invalid skill",
		},
		{
			name:   "name already in the vault is refused",
			skills: []string{"mocked-skill-00004"},
			prep: func(t *testing.T, agent config.Agent) {
				writeSkillDir(t, filepath.Join(agent.Path, "mocked-skill-00004"), "mocked-skill-00004")
			},
			wantErr: "already in the vault (use --force)",
		},
		{
			name:   "force replaces the vault skill",
			skills: []string{"mocked-skill-00004"},
			prep: func(t *testing.T, agent config.Agent) {
				writeSkillDir(t, filepath.Join(agent.Path, "mocked-skill-00004"), "mocked-skill-00004")
			},
			opts: Options{Force: true},
			want: []string{"mocked-skill-00004"},
		},
		{
			name:   "dry run plans the adoption and changes nothing",
			skills: []string{"hand-made-skill"},
			opts:   Options{DryRun: true},
			want:   []string{"hand-made-skill"},
		},
		{
			name:   "glob adopts every unmanaged match",
			skills: []string{"hand-*"},
			prep: func(t *testing.T, agent config.Agent) {
				writeSkillDir(t, filepath.Join(agent.Path, "hand-two"), "hand-two")
			},
			want: []string{"hand-made-skill", "hand-two"},
		},
		{
			name:   "glob skips an invalid unmanaged match",
			skills: []string{"hand-*"},
			prep: func(t *testing.T, agent config.Agent) {
				writeTestFile(t, filepath.Join(agent.Path, "hand-bad", "SKILL.md"), "# no frontmatter\n")
			},
			want:      []string{"hand-made-skill"},
			wantSkips: []string{"hand-bad"},
		},
		{
			name:    "glob matching no unmanaged skill is refused",
			skills:  []string{"mocked-*"},
			wantErr: "no unmanaged skill in claude matches",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			agent := fixtureAgent(t, "claude")
			if tt.prep != nil {
				tt.prep(t, agent)
			}
			vaultBefore := snapshot(t, v.Path)
			agentBefore := snapshot(t, agent.Path)

			acts, err := Adopt(v, []config.Agent{agent}, agent, tt.skills, tt.opts)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Adopt() error = %v, want one containing %q", err, tt.wantErr)
				}
				if after := snapshot(t, v.Path); !reflect.DeepEqual(after, vaultBefore) {
					t.Error("vault changed on a refused adopt")
				}
				if after := snapshot(t, agent.Path); !reflect.DeepEqual(after, agentBefore) {
					t.Error("agent dir changed on a refused adopt")
				}
				return
			}
			if err != nil {
				t.Fatalf("Adopt() error = %v", err)
			}

			var wantActs []Action
			for _, name := range tt.want {
				wantActs = append(wantActs, Action{Op: OpAdopt, Skill: name, Agent: "claude", Path: filepath.Join(agent.Path, name)})
			}
			for _, name := range tt.wantSkips {
				wantActs = append(wantActs, Action{Op: OpSkip, Skill: name, Agent: "claude", Reason: "invalid"})
			}
			byName := func(a, b Action) int { return strings.Compare(a.Skill, b.Skill) }
			slices.SortFunc(acts, byName)
			slices.SortFunc(wantActs, byName)
			if !reflect.DeepEqual(acts, wantActs) {
				t.Errorf("actions = %+v, want %+v", acts, wantActs)
			}
			if tt.opts.DryRun {
				if after := snapshot(t, v.Path); !reflect.DeepEqual(after, vaultBefore) {
					t.Error("vault changed on a dry run")
				}
				if after := snapshot(t, agent.Path); !reflect.DeepEqual(after, agentBefore) {
					t.Error("agent dir changed on a dry run")
				}
				return
			}
			checkAdopted(t, v, agent, tt.want)
			for _, name := range tt.wantSkips {
				if _, ok := mustLock(t, agent.Path).Skills[name]; ok {
					t.Errorf("skipped %s was locked", name)
				}
			}
		})
	}
}

// handMadeFixture is the unmanaged skill folder of the shared claude fixture. Tests
// only read it.
func handMadeFixture() string {
	return filepath.Join(testfixtures.Root(), "targets", "claude", "hand-made-skill")
}

// mustLock reads the lock of the agent folder dir.
func mustLock(t *testing.T, dir string) target.Lock {
	t.Helper()
	lock, err := target.ReadLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	return lock
}

func TestAdoptIdenticalCopies(t *testing.T) {
	tests := []struct {
		name      string
		other     func(t *testing.T, dir string) // writes the copy in the second agent
		wantActs  []string                       // adopt actions, skill@agent
		wantOther State                          // the second agent's status after the adopt
	}{
		{
			name: "identical copy in a second agent becomes synced",
			other: func(t *testing.T, dir string) {
				if err := os.CopyFS(dir, os.DirFS(handMadeFixture())); err != nil {
					t.Fatal(err)
				}
			},
			wantActs:  []string{"hand-made-skill@claude", "hand-made-skill@cursor"},
			wantOther: Synced,
		},
		{
			name: "copy with a different hash stays unmanaged",
			other: func(t *testing.T, dir string) {
				if err := os.CopyFS(dir, os.DirFS(handMadeFixture())); err != nil {
					t.Fatal(err)
				}
				writeTestFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: hand-made-skill\ndescription: Other.\n---\n")
			},
			wantActs:  []string{"hand-made-skill@claude"},
			wantOther: Unmanaged,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			claude := fixtureAgent(t, "claude")
			cursor := fixtureAgent(t, "cursor")
			tt.other(t, filepath.Join(cursor.Path, "hand-made-skill"))
			agents := []config.Agent{claude, cursor}

			acts, err := Adopt(v, agents, claude, []string{"hand-made-skill"}, Options{})
			if err != nil {
				t.Fatalf("Adopt() error = %v", err)
			}
			if got := adoptNames(acts); !slices.Equal(got, tt.wantActs) {
				t.Errorf("adopt actions = %q, want %q", got, tt.wantActs)
			}
			if st := stateOf(t, v, cursor, "hand-made-skill"); st != tt.wantOther {
				t.Errorf("cursor status = %s, want %s", st, tt.wantOther)
			}
			if st := stateOf(t, v, claude, "hand-made-skill"); st != Synced {
				t.Errorf("claude status = %s, want synced", st)
			}
			if tt.wantOther == Unmanaged {
				if _, ok := mustLock(t, cursor.Path).Skills["hand-made-skill"]; ok {
					t.Error("cursor lock records a copy with a different hash")
				}
			}
		})
	}
}

func TestAdoptLockFailureKeepsLocks(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	v := fixtureVault(t)
	claude := fixtureAgent(t, "claude")
	cursor := fixtureAgent(t, "cursor")
	if err := os.CopyFS(filepath.Join(cursor.Path, "hand-made-skill"), os.DirFS(handMadeFixture())); err != nil {
		t.Fatal(err)
	}
	vaultBefore := snapshot(t, v.Path)
	claudeBefore := snapshot(t, claude.Path)

	// Claude's lock is written first. The read-only cursor dir then fails its lock write,
	// so Claude's lock must get its old contents back.
	if err := os.Chmod(cursor.Path, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cursor.Path, 0o755) })

	_, err := Adopt(v, []config.Agent{claude, cursor}, claude, []string{"hand-made-skill"}, Options{})
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("Adopt() error = %v, want a permission error from the lock write", err)
	}
	if after := snapshot(t, v.Path); !reflect.DeepEqual(after, vaultBefore) {
		t.Error("failed adopt changed the vault")
	}
	if after := snapshot(t, claude.Path); !reflect.DeepEqual(after, claudeBefore) {
		t.Error("failed adopt changed the claude lock")
	}
}

func TestAdoptLockFailureRemovesNewLock(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	v := fixtureVault(t)
	claude := fixtureAgent(t, "claude")
	cursor := fixtureAgent(t, "cursor")
	if err := os.CopyFS(filepath.Join(cursor.Path, "hand-made-skill"), os.DirFS(handMadeFixture())); err != nil {
		t.Fatal(err)
	}
	if has, err := target.LockExists(cursor.Path); err != nil || has {
		t.Fatalf("cursor lock present before adopt = %t (err %v), want none", has, err)
	}
	cursorBefore := snapshot(t, cursor.Path)

	// Cursor, the adopted agent, gets a new lock first. Claude's identical copy then
	// fails its lock write, so the lock cursor did not have must be removed again.
	if err := os.Chmod(claude.Path, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(claude.Path, 0o755) })
	claudeBefore := snapshot(t, claude.Path)

	_, err := Adopt(v, []config.Agent{cursor, claude}, cursor, []string{"hand-made-skill"}, Options{})
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("Adopt() error = %v, want a permission error from the lock write", err)
	}
	if has, err := target.LockExists(cursor.Path); err != nil || has {
		t.Errorf("cursor lock present after failed adopt = %t (err %v), want removed", has, err)
	}
	if after := snapshot(t, cursor.Path); !reflect.DeepEqual(after, cursorBefore) {
		t.Error("failed adopt changed the cursor folder")
	}
	if after := snapshot(t, claude.Path); !reflect.DeepEqual(after, claudeBefore) {
		t.Error("failed adopt changed the claude folder")
	}
}

func TestAdoptKeepsModeBits(t *testing.T) {
	v := fixtureVault(t)
	agent := fixtureAgent(t, "claude")
	dir := filepath.Join(agent.Path, "hand-made-skill")
	if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := Adopt(v, []config.Agent{agent}, agent, []string{"hand-made-skill"}, Options{}); err != nil {
		t.Fatalf("Adopt() error = %v", err)
	}
	info, err := os.Stat(filepath.Join(v.SkillDir("hand-made-skill"), "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("run.sh mode = %v, want 0755", info.Mode().Perm())
	}
}

func TestAdoptLockFailureKeepsVault(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	tests := []struct {
		name  string
		skill string
		force bool
	}{
		{name: "new vault skill is removed again", skill: "hand-made-skill"},
		{name: "forced replacement keeps the old vault copy", skill: "mocked-skill-00004", force: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			agent := fixtureAgent(t, "claude")
			if tt.skill == "mocked-skill-00004" {
				writeSkillDir(t, filepath.Join(agent.Path, tt.skill), tt.skill)
			}
			vaultBefore := snapshot(t, v.Path)

			// The lock is written through a temp file in the agent dir, so a read-only
			// dir makes the lock write fail after the vault copy is placed.
			if err := os.Chmod(agent.Path, 0o555); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(agent.Path, 0o755) })

			_, err := Adopt(v, []config.Agent{agent}, agent, []string{tt.skill}, Options{Force: tt.force})
			if !errors.Is(err, fs.ErrPermission) {
				t.Fatalf("Adopt() error = %v, want a permission error from the lock write", err)
			}
			if after := snapshot(t, v.Path); !reflect.DeepEqual(after, vaultBefore) {
				t.Error("failed adopt changed the vault")
			}
		})
	}
}

func TestUnmanagedCopies(t *testing.T) {
	tests := []struct {
		name  string
		other func(t *testing.T, dir string) // writes the copy in cursor, if any
		want  []UnmanagedCopy
	}{
		{
			name:  "one copy is no conflict",
			other: nil,
			want:  []UnmanagedCopy{{Skill: "hand-made-skill", Agent: "claude"}},
		},
		{
			name: "same name with the same hash is no conflict",
			other: func(t *testing.T, dir string) {
				if err := os.CopyFS(dir, os.DirFS(handMadeFixture())); err != nil {
					t.Fatal(err)
				}
			},
			want: []UnmanagedCopy{{Skill: "hand-made-skill", Agent: "claude"}, {Skill: "hand-made-skill", Agent: "cursor"}},
		},
		{
			name: "same name with different hashes conflicts on each copy",
			other: func(t *testing.T, dir string) {
				writeSkillDir(t, dir, "hand-made-skill")
			},
			want: []UnmanagedCopy{
				{Skill: "hand-made-skill", Agent: "claude", Conflict: true},
				{Skill: "hand-made-skill", Agent: "cursor", Conflict: true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			claude := fixtureAgent(t, "claude")
			cursor := fixtureAgent(t, "cursor")
			if tt.other != nil {
				tt.other(t, filepath.Join(cursor.Path, "hand-made-skill"))
			}
			got, err := UnmanagedCopies(v, []config.Agent{claude, cursor})
			if err != nil {
				t.Fatalf("UnmanagedCopies() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d copies, want %d: %+v", len(got), len(tt.want), got)
			}
			for i, w := range tt.want {
				g := got[i]
				if g.Skill != w.Skill || g.Agent != w.Agent || g.Conflict != w.Conflict {
					t.Errorf("copy %d = %s in %s (conflict %t), want %s in %s (conflict %t)", i, g.Skill, g.Agent, g.Conflict, w.Skill, w.Agent, w.Conflict)
				}
				if g.Hash == "" {
					t.Errorf("copy %d has no hash", i)
				}
			}
		})
	}
}
