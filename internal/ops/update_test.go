package ops

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"sapato/internal/config"
	"sapato/internal/target"
)

func TestUpdateSelection(t *testing.T) {
	tests := []struct {
		name    string
		skills  []string
		opts    Options
		agents  []config.Agent // nil uses the fixture claude target
		want    []string       // summarize format
		wantErr string
	}{
		{
			name: "all skills: outdated updated, modified skipped",
			want: []string{"skip mocked-skill-00003: modified", "update mocked-skill-00002"},
		},
		{
			name: "force also updates modified",
			opts: Options{Force: true},
			want: []string{"update mocked-skill-00002", "update mocked-skill-00003"},
		},
		{
			name:   "glob selects outdated and modified",
			skills: []string{"mocked-skill-0000[23]"},
			want:   []string{"skip mocked-skill-00003: modified", "update mocked-skill-00002"},
		},
		{
			name:   "literal outdated skill is updated",
			skills: []string{"mocked-skill-00002"},
			want:   []string{"update mocked-skill-00002"},
		},
		{
			name:   "synced skill is left alone",
			skills: []string{"mocked-skill-00001"},
		},
		{
			name:   "skill not installed is left alone",
			skills: []string{"mocked-skill-00004"},
		},
		{
			name:   "missing folder is left alone",
			skills: []string{"mocked-skill-00098"},
		},
		{
			name:   "unmanaged folder is left alone even with force",
			skills: []string{"hand-made-skill"},
			opts:   Options{Force: true},
		},
		{
			name:    "unknown skill",
			skills:  []string{"no-such-skill"},
			wantErr: `no skill matches "no-such-skill"`,
		},
		{
			name:    "no agents",
			agents:  []config.Agent{},
			wantErr: "no agents selected",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			agents := tt.agents
			if agents == nil {
				agents = []config.Agent{fixtureAgent(t, "claude")}
			}
			actions, err := Update(v, agents, tt.skills, tt.opts)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Update() error = %v", err)
			}
			if got := summarize(actions); !slices.Equal(got, tt.want) {
				t.Errorf("actions = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUpdateApplies(t *testing.T) {
	v := fixtureVault(t)
	agent := fixtureAgent(t, "claude")
	before, err := target.ReadLock(agent.Path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Update(v, []config.Agent{agent}, nil, Options{Force: true}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	for _, name := range []string{"mocked-skill-00002", "mocked-skill-00003"} {
		if got := stateOf(t, v, agent, name); got != Synced {
			t.Errorf("%s status = %s, want synced", name, got)
		}
	}

	after, err := target.ReadLock(agent.Path)
	if err != nil {
		t.Fatal(err)
	}
	s := vaultSkill(t, v, "mocked-skill-00002")
	if entry := after.Skills["mocked-skill-00002"]; entry.Hash != s.Hash || entry.Version != "1.2.0" {
		t.Errorf("mocked-skill-00002 entry = %+v, want hash %s version 1.2.0", entry, s.Hash)
	}
	for _, name := range []string{"mocked-skill-00001", "mocked-skill-00098", "mocked-skill-00099"} {
		if !reflect.DeepEqual(after.Skills[name], before.Skills[name]) {
			t.Errorf("%s entry changed: %+v -> %+v", name, before.Skills[name], after.Skills[name])
		}
	}
	if _, err := os.Stat(filepath.Join(agent.Path, "hand-made-skill", "SKILL.md")); err != nil {
		t.Errorf("unmanaged folder was touched: %v", err)
	}
}

func TestUpdateDryRunChangesNothing(t *testing.T) {
	v := fixtureVault(t)
	agent := fixtureAgent(t, "claude")
	before := snapshot(t, agent.Path)

	actions, err := Update(v, []config.Agent{agent}, nil, Options{Force: true, DryRun: true})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	want := []string{"update mocked-skill-00002", "update mocked-skill-00003"}
	if got := summarize(actions); !slices.Equal(got, want) {
		t.Errorf("planned = %v, want %v", got, want)
	}
	if after := snapshot(t, agent.Path); !reflect.DeepEqual(after, before) {
		t.Error("dry run changed the target")
	}
}

func TestUpdateInvalidVaultCopy(t *testing.T) {
	v := fixtureVault(t)
	agent := fixtureAgent(t, "claude")
	if _, err := Install(v, []config.Agent{agent}, []string{"mocked-skill-00004"}, Options{}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	// The vault copy stops being valid after it was installed, so it is outdated.
	writeTestFile(t, filepath.Join(vaultSkill(t, v, "mocked-skill-00004").Dir, "SKILL.md"), "no frontmatter\n")
	before := snapshot(t, agent.Path)

	t.Run("glob skips it", func(t *testing.T) {
		actions, err := Update(v, []config.Agent{agent}, []string{"mocked-skill-0000[4]"}, Options{})
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}
		if got := summarize(actions); !slices.Equal(got, []string{"skip mocked-skill-00004: invalid"}) {
			t.Errorf("actions = %v", got)
		}
	})

	t.Run("literal name fails the call", func(t *testing.T) {
		_, err := Update(v, []config.Agent{agent}, []string{"mocked-skill-00004"}, Options{})
		if err == nil || !strings.Contains(err.Error(), "invalid skills are never installed: mocked-skill-00004 (") {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("literal invalid fails the whole call", func(t *testing.T) {
		_, err := Update(v, []config.Agent{agent}, []string{"mocked-skill-00002", "mocked-skill-00004"}, Options{Force: true})
		if err == nil || !strings.Contains(err.Error(), "invalid skills are never installed") {
			t.Errorf("error = %v", err)
		}
	})

	if after := snapshot(t, agent.Path); !reflect.DeepEqual(after, before) {
		t.Error("refused update changed the target")
	}
}
