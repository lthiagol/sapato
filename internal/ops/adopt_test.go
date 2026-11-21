package ops

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sapato/internal/config"
	"sapato/internal/hashdir"
	"sapato/internal/target"
)

func TestAdopt(t *testing.T) {
	tests := []struct {
		name    string
		skill   string
		prep    func(t *testing.T, agent config.Agent)
		opts    Options
		wantErr string
	}{
		{
			name:  "unmanaged folder is copied into the vault and locked",
			skill: "hand-made-skill",
		},
		{
			name:    "managed skill is refused",
			skill:   "mocked-skill-00001",
			wantErr: "synced, want unmanaged",
		},
		{
			name:    "folder missing from the agent is refused",
			skill:   "absent-skill",
			wantErr: "not-installed, want unmanaged",
		},
		{
			name:    "name that is not a folder name is refused",
			skill:   "../hand-made-skill",
			wantErr: "not a skill folder name",
		},
		{
			name:  "invalid unmanaged folder is refused",
			skill: "broken",
			prep: func(t *testing.T, agent config.Agent) {
				writeTestFile(t, filepath.Join(agent.Path, "broken", "SKILL.md"), "# no frontmatter\n")
			},
			wantErr: "invalid skill",
		},
		{
			name:  "name already in the vault is refused",
			skill: "mocked-skill-00004",
			prep: func(t *testing.T, agent config.Agent) {
				writeSkillDir(t, filepath.Join(agent.Path, "mocked-skill-00004"), "mocked-skill-00004")
			},
			wantErr: "already in the vault (use --force)",
		},
		{
			name:  "force replaces the vault skill",
			skill: "mocked-skill-00004",
			prep: func(t *testing.T, agent config.Agent) {
				writeSkillDir(t, filepath.Join(agent.Path, "mocked-skill-00004"), "mocked-skill-00004")
			},
			opts: Options{Force: true},
		},
		{
			name:  "dry run plans the adoption and changes nothing",
			skill: "hand-made-skill",
			opts:  Options{DryRun: true},
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

			action, err := Adopt(v, agent, tt.skill, tt.opts)
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

			wantAction := Action{Op: OpAdopt, Skill: tt.skill, Agent: "claude", Path: filepath.Join(agent.Path, tt.skill)}
			if action != wantAction {
				t.Errorf("action = %+v, want %+v", action, wantAction)
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

			got := vaultSkill(t, v, tt.skill)
			if !got.Valid {
				t.Errorf("adopted skill is invalid: %v", got.Problems)
			}
			lock, err := target.ReadLock(agent.Path)
			if err != nil {
				t.Fatal(err)
			}
			entry, ok := lock.Skills[tt.skill]
			if !ok || entry.Hash != got.Hash {
				t.Errorf("lock entry = %+v (present %v), want hash %s", entry, ok, got.Hash)
			}
			if st := stateOf(t, v, agent, tt.skill); st != Synced {
				t.Errorf("status after adopt = %s, want synced", st)
			}
			if h, err := hashdir.Hash(filepath.Join(agent.Path, tt.skill)); err != nil || h != got.Hash {
				t.Errorf("target hash = %s (err %v), want vault hash %s", h, err, got.Hash)
			}
		})
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

	if _, err := Adopt(v, agent, "hand-made-skill", Options{}); err != nil {
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

			_, err := Adopt(v, agent, tt.skill, Options{Force: tt.force})
			if !errors.Is(err, fs.ErrPermission) {
				t.Fatalf("Adopt() error = %v, want a permission error from the lock write", err)
			}
			if after := snapshot(t, v.Path); !reflect.DeepEqual(after, vaultBefore) {
				t.Error("failed adopt changed the vault")
			}
		})
	}
}
