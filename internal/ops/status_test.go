package ops

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"sapato/internal/config"
	"sapato/internal/testfixtures"
	"sapato/internal/vault"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		p    presence
		want State
	}{
		{
			name: "valid in vault only is not installed",
			p:    presence{inVault: true, valid: true, vaultHash: "h1"},
			want: NotInstalled,
		},
		{
			name: "invalid in vault only is invalid",
			p:    presence{inVault: true, vaultHash: "h1"},
			want: Invalid,
		},
		{
			name: "invalid in vault and target is unmanaged",
			p:    presence{inVault: true, vaultHash: "h1", inTarget: true, targetHash: "t1"},
			want: Unmanaged,
		},
		{
			name: "invalid in lock, folder gone is missing",
			p:    presence{inVault: true, vaultHash: "h1", inLock: true, lockHash: "l1"},
			want: Missing,
		},
		{
			name: "in target only is unmanaged",
			p:    presence{inTarget: true, targetHash: "t1"},
			want: Unmanaged,
		},
		{
			name: "unmanaged wins even when also in vault",
			p:    presence{inVault: true, vaultHash: "h1", inTarget: true, targetHash: "t1"},
			want: Unmanaged,
		},
		{
			name: "in lock only is missing",
			p:    presence{inLock: true, lockHash: "l1"},
			want: Missing,
		},
		{
			name: "in lock and vault, folder gone is missing",
			p:    presence{inVault: true, vaultHash: "l1", inLock: true, lockHash: "l1"},
			want: Missing,
		},
		{
			name: "target equal to lock and vault is synced",
			p:    presence{inVault: true, vaultHash: "l1", inLock: true, lockHash: "l1", inTarget: true, targetHash: "l1"},
			want: Synced,
		},
		{
			name: "target equal to lock, vault differs is outdated",
			p:    presence{inVault: true, vaultHash: "h2", inLock: true, lockHash: "l1", inTarget: true, targetHash: "l1"},
			want: Outdated,
		},
		{
			name: "target equal to lock, not in vault is orphaned",
			p:    presence{inLock: true, lockHash: "l1", inTarget: true, targetHash: "l1"},
			want: Orphaned,
		},
		{
			name: "target differs from lock is modified",
			p:    presence{inVault: true, vaultHash: "l1", inLock: true, lockHash: "l1", inTarget: true, targetHash: "t1"},
			want: Modified,
		},
		{
			name: "modified takes precedence over outdated",
			p:    presence{inVault: true, vaultHash: "h2", inLock: true, lockHash: "l1", inTarget: true, targetHash: "t1"},
			want: Modified,
		},
		{
			name: "modified takes precedence over orphaned",
			p:    presence{inLock: true, lockHash: "l1", inTarget: true, targetHash: "t1"},
			want: Modified,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classify(tt.p); got != tt.want {
				t.Errorf("classify() = %s, want %s", got, tt.want)
			}
		})
	}
}

// fixtureVault opens a copy of the skill-db fixture.
func fixtureVault(t *testing.T) *vault.Vault {
	t.Helper()
	v, err := vault.Open(testfixtures.CopyTree(t, filepath.Join(testfixtures.Root(), "skill-db")))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// fixtureValidity maps each vault skill of the fixture to its Valid flag.
func fixtureValidity(t *testing.T, v *vault.Vault) map[string]bool {
	t.Helper()
	skills, err := v.Skills()
	if err != nil {
		t.Fatal(err)
	}
	valid := make(map[string]bool, len(skills))
	for _, s := range skills {
		valid[s.Name] = s.Valid
	}
	return valid
}

// fixtureAgent copies targets/<id> and points an agent at it.
func fixtureAgent(t *testing.T, id string) config.Agent {
	t.Helper()
	return config.Agent{ID: id, Path: testfixtures.CopyTree(t, filepath.Join(testfixtures.Root(), "targets", id))}
}

func TestStatusFixtures(t *testing.T) {
	v := fixtureVault(t)
	agents := []config.Agent{fixtureAgent(t, "claude"), fixtureAgent(t, "agents"), fixtureAgent(t, "cursor")}

	// Pairs that differ from the default. A vault skill with no target or lock
	// entry in an agent is not-installed if valid, invalid if not. Keys are
	// "agent/skill".
	special := map[string]State{
		"claude/mocked-skill-00001": Synced,
		"claude/mocked-skill-00002": Outdated,
		"claude/mocked-skill-00003": Modified,
		"claude/mocked-skill-00098": Missing,
		"claude/mocked-skill-00099": Orphaned,
		"claude/hand-made-skill":    Unmanaged,
		"agents/mocked-skill-00001": Synced,
	}
	seen := make(map[string]bool, len(special))
	states := make(map[State]bool)

	rows, err := Status(v, agents, Filter{})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	valid := fixtureValidity(t, v)

	// claude: 12 vault skills + 00098, 00099, hand-made-skill = 15.
	// agents and cursor: the 12 vault skills each = 24.
	if len(rows) != 39 {
		t.Errorf("got %d rows, want 39", len(rows))
	}
	for _, r := range rows {
		key := r.Agent + "/" + r.Skill
		want, ok := special[key]
		if ok {
			seen[key] = true
		} else {
			isValid, inVault := valid[r.Skill]
			if !inVault {
				t.Errorf("unexpected row %+v for a skill not in the vault", r)
				continue
			}
			want = NotInstalled
			if !isValid {
				want = Invalid
			}
		}
		if r.Status != want {
			t.Errorf("%s = %s, want %s", key, r.Status, want)
		}
		states[r.Status] = true
	}
	for key := range special {
		if !seen[key] {
			t.Errorf("no row for %s", key)
		}
	}
	for _, s := range []State{NotInstalled, Invalid, Synced, Outdated, Modified, Orphaned, Unmanaged, Missing} {
		if !states[s] {
			t.Errorf("no row with status %s", s)
		}
	}
}

func TestStatusFilter(t *testing.T) {
	v := fixtureVault(t)
	agents := []config.Agent{fixtureAgent(t, "claude"), fixtureAgent(t, "agents")}

	tests := []struct {
		name  string
		globs []string
		want  []Row
	}{
		{
			name:  "glob selects skills and keeps agent order",
			globs: []string{"mocked-skill-0000[12]"},
			want: []Row{
				{Skill: "mocked-skill-00001", Agent: "claude", Status: Synced},
				{Skill: "mocked-skill-00001", Agent: "agents", Status: Synced},
				{Skill: "mocked-skill-00002", Agent: "claude", Status: Outdated},
				{Skill: "mocked-skill-00002", Agent: "agents", Status: NotInstalled},
			},
		},
		{
			name:  "glob can select an unmanaged skill",
			globs: []string{"hand-*"},
			want:  []Row{{Skill: "hand-made-skill", Agent: "claude", Status: Unmanaged}},
		},
		{
			name:  "several globs are unioned",
			globs: []string{"hand-*", "mocked-skill-00098"},
			want: []Row{
				{Skill: "hand-made-skill", Agent: "claude", Status: Unmanaged},
				{Skill: "mocked-skill-00098", Agent: "claude", Status: Missing},
			},
		},
		{
			name:  "no match gives no rows",
			globs: []string{"nothing-here"},
			want:  []Row{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Status(v, agents, Filter{Skills: tt.globs})
			if err != nil {
				t.Fatalf("Status() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Status() =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestStatusMissingTargetDir(t *testing.T) {
	v := fixtureVault(t)
	agent := config.Agent{ID: "pi", Path: filepath.Join(t.TempDir(), "absent")}

	rows, err := Status(v, []config.Agent{agent}, Filter{})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	valid := fixtureValidity(t, v)
	if len(rows) != 12 {
		t.Fatalf("got %d rows, want 12 vault skills", len(rows))
	}
	for _, r := range rows {
		want := NotInstalled
		if !valid[r.Skill] {
			want = Invalid
		}
		if r.Status != want {
			t.Errorf("%s = %s, want %s", r.Skill, r.Status, want)
		}
	}
}

func TestStatusErrors(t *testing.T) {
	t.Run("bad glob", func(t *testing.T) {
		_, err := Status(fixtureVault(t), []config.Agent{fixtureAgent(t, "claude")}, Filter{Skills: []string{"["}})
		if err == nil || !strings.Contains(err.Error(), `bad skill pattern "["`) {
			t.Errorf("error = %v, want bad skill pattern", err)
		}
	})

	t.Run("corrupt lock names the file", func(t *testing.T) {
		agent := fixtureAgent(t, "claude")
		lockPath := filepath.Join(agent.Path, ".sapato-lock.json")
		if err := os.WriteFile(lockPath, []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Status(fixtureVault(t), []config.Agent{agent}, Filter{})
		if err == nil || !strings.Contains(err.Error(), lockPath) {
			t.Errorf("error = %v, want it to name %s", err, lockPath)
		}
	})

	t.Run("vault without skills dir is an error", func(t *testing.T) {
		dir := t.TempDir()
		if err := vault.Init(dir); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(filepath.Join(dir, "skills")); err != nil {
			t.Fatal(err)
		}
		v, err := vault.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Status(v, nil, Filter{}); err == nil {
			t.Error("Status() error = nil")
		}
	})
}

func TestSelectAgents(t *testing.T) {
	agents := []config.Agent{
		{ID: "claude", Path: "/c"},
		{ID: "cursor", Path: "/u"},
		{ID: "pi", Path: "/p"},
	}
	tests := []struct {
		name    string
		ids     []string
		want    []string
		wantErr string
	}{
		{name: "no ids selects every configured agent", ids: nil, want: []string{"claude", "cursor", "pi"}},
		{name: "explicit ids keep config order", ids: []string{"pi", "claude"}, want: []string{"claude", "pi"}},
		{name: "explicit id selects any configured agent", ids: []string{"cursor"}, want: []string{"cursor"}},
		{name: "repeated id selected once", ids: []string{"pi", "pi"}, want: []string{"pi"}},
		{name: "unknown id", ids: []string{"windsurf"}, wantErr: `unknown agent "windsurf"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SelectAgents(agents, tt.ids)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("SelectAgents() error = %v", err)
			}
			var ids []string
			for _, a := range got {
				ids = append(ids, a.ID)
			}
			if !slices.Equal(ids, tt.want) {
				t.Errorf("ids = %v, want %v", ids, tt.want)
			}
		})
	}
}
