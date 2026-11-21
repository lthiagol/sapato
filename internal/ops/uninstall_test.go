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
	"sapato/internal/target"
)

func TestUninstall(t *testing.T) {
	tests := []struct {
		name     string
		skill    string
		force    bool
		wantErr  string
		wantOp   []Action // nil means no actions
		wantGone bool     // the folder is absent afterwards
	}{
		{
			name:     "synced copy is removed",
			skill:    "mocked-skill-00001",
			wantOp:   []Action{{Op: OpUninstall, Skill: "mocked-skill-00001", Agent: "claude"}},
			wantGone: true,
		},
		{
			name:     "outdated copy is removed",
			skill:    "mocked-skill-00002",
			wantOp:   []Action{{Op: OpUninstall, Skill: "mocked-skill-00002", Agent: "claude"}},
			wantGone: true,
		},
		{
			name:     "orphaned copy is removed",
			skill:    "mocked-skill-00099",
			wantOp:   []Action{{Op: OpUninstall, Skill: "mocked-skill-00099", Agent: "claude"}},
			wantGone: true,
		},
		{
			name:     "missing copy loses its lock entry",
			skill:    "mocked-skill-00098",
			wantOp:   []Action{{Op: OpUninstall, Skill: "mocked-skill-00098", Agent: "claude"}},
			wantGone: true,
		},
		{
			name:    "modified copy is refused without force",
			skill:   "mocked-skill-00003",
			wantErr: "mocked-skill-00003 in claude: modified (use --force)",
		},
		{
			name:    "modified copy matched by glob is refused without force",
			skill:   "mocked-skill-0000[3]",
			wantErr: "mocked-skill-00003 in claude: modified (use --force)",
		},
		{
			name:     "modified copy is removed with force",
			skill:    "mocked-skill-00003",
			force:    true,
			wantOp:   []Action{{Op: OpUninstall, Skill: "mocked-skill-00003", Agent: "claude"}},
			wantGone: true,
		},
		{
			name:    "unmanaged folder is refused even with force",
			skill:   "hand-made-skill",
			force:   true,
			wantErr: "hand-made-skill in claude: unmanaged",
		},
		{
			name:  "skill not installed is skipped",
			skill: "mocked-skill-00004",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			agent := fixtureAgent(t, "claude")
			before := snapshot(t, agent.Path)

			actions, err := Uninstall(v, []config.Agent{agent}, []string{tt.skill}, Options{Force: tt.force})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				if after := snapshot(t, agent.Path); !reflect.DeepEqual(after, before) {
					t.Error("refused uninstall changed the target")
				}
				return
			}
			if err != nil {
				t.Fatalf("Uninstall() error = %v", err)
			}
			for i := range tt.wantOp {
				tt.wantOp[i].Path = filepath.Join(agent.Path, tt.skill)
			}
			if !reflect.DeepEqual(actions, tt.wantOp) {
				t.Errorf("actions = %+v, want %+v", actions, tt.wantOp)
			}
			if tt.wantGone {
				if _, err := os.Stat(filepath.Join(agent.Path, tt.skill)); !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("folder still there (err %v)", err)
				}
				lock, err := target.ReadLock(agent.Path)
				if err != nil {
					t.Fatal(err)
				}
				if _, ok := lock.Skills[tt.skill]; ok {
					t.Error("lock still has an entry for the skill")
				}
			}
			if tt.wantOp == nil {
				if after := snapshot(t, agent.Path); !reflect.DeepEqual(after, before) {
					t.Error("skipped skill changed the target")
				}
			}
		})
	}
}

func TestUninstallSkipsUnmanagedMatchedByGlob(t *testing.T) {
	v := fixtureVault(t)
	agent := fixtureAgent(t, "claude")
	before := snapshot(t, agent.Path)

	actions, err := Uninstall(v, []config.Agent{agent}, []string{"h*"}, Options{Force: true})
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	want := []Action{{Op: OpSkip, Skill: "hand-made-skill", Agent: "claude", Reason: "unmanaged"}}
	if !reflect.DeepEqual(actions, want) {
		t.Errorf("actions = %+v, want %+v", actions, want)
	}
	if after := snapshot(t, agent.Path); !reflect.DeepEqual(after, before) {
		t.Error("skipped unmanaged folder changed the target")
	}

	// A broad glob with --force removes what it can and leaves the unmanaged folder.
	if _, err := Uninstall(v, []config.Agent{agent}, []string{"*"}, Options{Force: true}); err != nil {
		t.Fatalf("Uninstall('*') error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(agent.Path, "hand-made-skill", "SKILL.md")); err != nil {
		t.Errorf("unmanaged folder was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(agent.Path, "mocked-skill-00001")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("mocked-skill-00001 still there (err %v)", err)
	}
}

func TestUninstallLeavesOtherEntries(t *testing.T) {
	v := fixtureVault(t)
	agent := fixtureAgent(t, "claude")
	before, err := target.ReadLock(agent.Path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Uninstall(v, []config.Agent{agent}, []string{"mocked-skill-00001"}, Options{}); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	after, err := target.ReadLock(agent.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := after.Skills["mocked-skill-00001"]; ok {
		t.Error("removed skill still in lock")
	}
	if len(after.Skills) != len(before.Skills)-1 {
		t.Errorf("lock has %d entries, want %d", len(after.Skills), len(before.Skills)-1)
	}
	for name, entry := range after.Skills {
		if !reflect.DeepEqual(entry, before.Skills[name]) {
			t.Errorf("%s entry changed: %+v -> %+v", name, before.Skills[name], entry)
		}
	}
}

func TestUninstallDryRunChangesNothing(t *testing.T) {
	v := fixtureVault(t)
	agent := fixtureAgent(t, "claude")
	before := snapshot(t, agent.Path)

	actions, err := Uninstall(v, []config.Agent{agent}, []string{"mocked-skill-00001", "mocked-skill-00003"}, Options{DryRun: true, Force: true})
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if len(actions) != 2 {
		t.Errorf("planned %d actions, want 2", len(actions))
	}
	if after := snapshot(t, agent.Path); !reflect.DeepEqual(after, before) {
		t.Error("dry run changed the target")
	}
}

func TestUninstallRefusesUnsafeLockKey(t *testing.T) {
	v := fixtureVault(t)
	agent := fixtureAgent(t, "claude")
	outside := filepath.Join(filepath.Dir(agent.Path), "outside")
	writeTestFile(t, filepath.Join(outside, "keep.txt"), "keep")

	lock, err := target.ReadLock(agent.Path)
	if err != nil {
		t.Fatal(err)
	}
	lock.Skills["../outside"] = target.LockEntry{Hash: "sha256:x"}
	if err := target.WriteLock(agent.Path, lock); err != nil {
		t.Fatal(err)
	}

	_, err = Uninstall(v, []config.Agent{agent}, []string{"../outside"}, Options{Force: true})
	if err == nil || !strings.Contains(err.Error(), "refusing to remove") {
		t.Fatalf("error = %v, want refusing to remove", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "keep.txt")); err != nil {
		t.Errorf("file outside the agent dir was touched: %v", err)
	}
}

func TestUninstallRejectsBadArgs(t *testing.T) {
	tests := []struct {
		name    string
		skills  []string
		wantErr string
	}{
		{name: "no match", skills: []string{"no-such-skill"}, wantErr: `no skill matches "no-such-skill"`},
		{name: "malformed glob", skills: []string{"["}, wantErr: "bad skill pattern"},
		{name: "no skills", skills: nil, wantErr: "no skills given"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			agent := fixtureAgent(t, "claude")
			_, err := Uninstall(v, []config.Agent{agent}, tt.skills, Options{})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
