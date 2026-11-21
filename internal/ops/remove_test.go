package ops

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sapato/internal/config"
)

func TestRemove(t *testing.T) {
	tests := []struct {
		name      string
		skill     string
		opts      Options
		wantErr   string
		wantState State // status in claude afterwards, when the skill is removed
	}{
		{
			name:  "skill installed nowhere is deleted",
			skill: "mocked-skill-00006",
		},
		{
			name:  "invalid vault skill that is installed nowhere is deleted",
			skill: "invalid-no-frontmatter",
		},
		{
			name:    "synced skill is refused without force",
			skill:   "mocked-skill-00001",
			wantErr: "installed in claude, agents (use --force)",
		},
		{
			name:    "outdated skill is refused without force",
			skill:   "mocked-skill-00002",
			wantErr: "installed in claude (use --force)",
		},
		{
			name:    "modified skill is refused without force",
			skill:   "mocked-skill-00003",
			wantErr: "installed in claude (use --force)",
		},
		{
			name:      "force removes an installed skill and its copies become orphaned",
			skill:     "mocked-skill-00001",
			opts:      Options{Force: true},
			wantState: Orphaned,
		},
		{
			name:    "unknown skill is refused",
			skill:   "nope",
			wantErr: "no skill in vault",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			agents := []config.Agent{fixtureAgent(t, "claude"), fixtureAgent(t, "agents")}
			vaultBefore := snapshot(t, v.Path)

			action, err := Remove(v, agents, tt.skill, tt.opts)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Remove() error = %v, want one containing %q", err, tt.wantErr)
				}
				if after := snapshot(t, v.Path); !reflect.DeepEqual(after, vaultBefore) {
					t.Error("vault changed on a refused remove")
				}
				return
			}
			if err != nil {
				t.Fatalf("Remove() error = %v", err)
			}

			wantAction := Action{Op: OpRemove, Skill: tt.skill, Path: v.SkillDir(tt.skill)}
			if action != wantAction {
				t.Errorf("action = %+v, want %+v", action, wantAction)
			}
			if _, err := os.Stat(v.SkillDir(tt.skill)); !os.IsNotExist(err) {
				t.Errorf("vault folder still present (err %v)", err)
			}
			if tt.wantState != "" {
				if got := stateOf(t, v, agents[0], tt.skill); got != tt.wantState {
					t.Errorf("status in claude = %s, want %s", got, tt.wantState)
				}
			}
		})
	}
}

func TestRemoveDryRun(t *testing.T) {
	v := fixtureVault(t)
	agents := []config.Agent{fixtureAgent(t, "claude")}
	before := snapshot(t, v.Path)

	action, err := Remove(v, agents, "mocked-skill-00006", Options{DryRun: true})
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if action.Op != OpRemove || action.Skill != "mocked-skill-00006" {
		t.Errorf("action = %+v", action)
	}
	if after := snapshot(t, v.Path); !reflect.DeepEqual(after, before) {
		t.Error("vault changed on a dry run")
	}
}

func TestRemoveIgnoresUnmanagedCopies(t *testing.T) {
	// A same-named folder that sapato does not manage is not installed, so
	// Remove neither refuses for it nor deletes it.
	v := fixtureVault(t)
	agents := []config.Agent{fixtureAgent(t, "claude")}
	writeSkillDir(t, filepath.Join(agents[0].Path, "mocked-skill-00006"), "mocked-skill-00006")

	if _, err := Remove(v, agents, "mocked-skill-00006", Options{}); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(agents[0].Path, "mocked-skill-00006", "SKILL.md")); err != nil {
		t.Errorf("unmanaged copy was touched: %v", err)
	}
}
