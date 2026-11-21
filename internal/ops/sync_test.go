package ops

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"sapato/internal/config"
	"sapato/internal/vault"
)

// syncVault makes a vault in a temp dir whose manifest holds assignments. It
// adds the valid and invalid skills by name.
func syncVault(t *testing.T, assignments string, valid, invalid []string) *vault.Vault {
	t.Helper()
	dir := t.TempDir()
	if err := vault.Init(dir); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dir, "sapato.toml"), "format = 1\n\n[assignments]\n"+assignments)
	v, err := vault.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range valid {
		writeSkillDir(t, v.SkillDir(name), name)
	}
	for _, name := range invalid {
		writeTestFile(t, filepath.Join(v.SkillDir(name), "SKILL.md"), "# no frontmatter\n")
	}
	return v
}

// syncAgents makes the five built-in agents in a temp dir. Only claude's
// enabled flag can be set; the others are enabled.
func syncAgents(t *testing.T, claudeEnabled bool) []config.Agent {
	t.Helper()
	root := t.TempDir()
	agents := []config.Agent{
		{ID: "claude", Path: filepath.Join(root, "claude"), Enabled: claudeEnabled},
		{ID: "agents", Path: filepath.Join(root, "agents"), Enabled: true},
		{ID: "cursor", Path: filepath.Join(root, "cursor"), Enabled: true},
		{ID: "opencode", Path: filepath.Join(root, "opencode"), Enabled: true},
		{ID: "pi", Path: filepath.Join(root, "pi"), Enabled: true},
	}
	for _, a := range agents {
		if err := os.MkdirAll(a.Path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return agents
}

// agentNamed returns the agent with id from agents.
func agentNamed(t *testing.T, agents []config.Agent, id string) config.Agent {
	t.Helper()
	i := slices.IndexFunc(agents, func(a config.Agent) bool { return a.ID == id })
	if i < 0 {
		t.Fatalf("no agent %s", id)
	}
	return agents[i]
}

// snapshotAll snapshots the vault and every agent dir in one map.
func snapshotAll(t *testing.T, v *vault.Vault, agents []config.Agent) map[string]string {
	t.Helper()
	all := make(map[string]string)
	for _, dir := range append([]string{v.Path}, agentPaths(agents)...) {
		for rel, s := range snapshot(t, dir) {
			all[dir+"/"+rel] = s
		}
	}
	return all
}

func agentPaths(agents []config.Agent) []string {
	paths := make([]string, 0, len(agents))
	for _, a := range agents {
		paths = append(paths, a.Path)
	}
	return paths
}

// planLines renders actions as sorted "op skill in agent" lines, with ": reason" on skips.
func planLines(actions []Action) []string {
	out := make([]string, 0, len(actions))
	for _, a := range actions {
		line := a.Op + " " + a.Skill + " in " + a.Agent
		if a.Op == OpSkip {
			line += ": " + a.Reason
		}
		out = append(out, line)
	}
	slices.Sort(out)
	return out
}

// sortedCopy returns a sorted copy of lines, so a test can list them in any order.
func sortedCopy(lines []string) []string {
	out := slices.Clone(lines)
	slices.Sort(out)
	return out
}

// installFor installs names into agent, failing the test on error.
func installFor(t *testing.T, v *vault.Vault, agent config.Agent, names ...string) {
	t.Helper()
	if _, err := Install(v, []config.Agent{agent}, names, Options{}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
}

func TestSync(t *testing.T) {
	tests := []struct {
		name        string
		assignments string
		valid       []string
		invalid     []string
		disabled    bool // claude is disabled
		prep        func(t *testing.T, v *vault.Vault, agents []config.Agent)
		opts        SyncOptions
		want        []string
		wantErr     string
		wantAfter   map[string]State // "agent skill" -> status after a real run; "" when no row
	}{
		{
			name:        "installs literal names and globs",
			assignments: "claude = [\"code-review\", \"obsidian-*\"]\n",
			valid:       []string{"code-review", "obsidian-daily", "obsidian-notes", "unrelated"},
			want:        []string{"install code-review in claude", "install obsidian-daily in claude", "install obsidian-notes in claude"},
			wantAfter:   map[string]State{"claude code-review": Synced, "claude unrelated": NotInstalled},
		},
		{
			name:        "overlapping patterns install a skill once",
			assignments: "claude = [\"code-review\", \"code-*\"]\n",
			valid:       []string{"code-review"},
			want:        []string{"install code-review in claude"},
		},
		{
			name:        "outdated skill is updated",
			assignments: "claude = [\"code-review\"]\n",
			valid:       []string{"code-review"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				installFor(t, v, agentNamed(t, agents, "claude"), "code-review")
				writeTestFile(t, filepath.Join(v.SkillDir("code-review"), "notes.md"), "newer\n")
			},
			want:      []string{"update code-review in claude"},
			wantAfter: map[string]State{"claude code-review": Synced},
		},
		{
			name:        "synced skill is left alone",
			assignments: "claude = [\"code-review\"]\n",
			valid:       []string{"code-review"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				installFor(t, v, agentNamed(t, agents, "claude"), "code-review")
			},
			wantAfter: map[string]State{"claude code-review": Synced},
		},
		{
			name:        "modified skill is skipped without force",
			assignments: "claude = [\"code-review\"]\n",
			valid:       []string{"code-review"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				claude := agentNamed(t, agents, "claude")
				installFor(t, v, claude, "code-review")
				writeTestFile(t, filepath.Join(claude.Path, "code-review", "notes.md"), "local edit\n")
			},
			want:      []string{"skip code-review in claude: modified"},
			wantAfter: map[string]State{"claude code-review": Modified},
		},
		{
			name:        "modified skill is overwritten with force",
			assignments: "claude = [\"code-review\"]\n",
			valid:       []string{"code-review"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				claude := agentNamed(t, agents, "claude")
				installFor(t, v, claude, "code-review")
				writeTestFile(t, filepath.Join(claude.Path, "code-review", "notes.md"), "local edit\n")
			},
			opts:      SyncOptions{Options: Options{Force: true}},
			want:      []string{"update code-review in claude"},
			wantAfter: map[string]State{"claude code-review": Synced},
		},
		{
			name:        "unmanaged folder of an assigned skill is refused",
			assignments: "claude = [\"code-review\"]\n",
			valid:       []string{"code-review"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				writeSkillDir(t, filepath.Join(agentNamed(t, agents, "claude").Path, "code-review"), "code-review")
			},
			wantErr: "refusing to overwrite code-review in claude: unmanaged",
		},
		{
			name:        "unmanaged folder is replaced with force",
			assignments: "claude = [\"code-review\"]\n",
			valid:       []string{"code-review"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				writeSkillDir(t, filepath.Join(agentNamed(t, agents, "claude").Path, "code-review"), "code-review")
			},
			opts:      SyncOptions{Options: Options{Force: true}},
			want:      []string{"install code-review in claude"},
			wantAfter: map[string]State{"claude code-review": Synced},
		},
		{
			name:        "missing folder with a lock entry is reinstalled",
			assignments: "claude = [\"code-review\"]\n",
			valid:       []string{"code-review"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				claude := agentNamed(t, agents, "claude")
				installFor(t, v, claude, "code-review")
				if err := os.RemoveAll(filepath.Join(claude.Path, "code-review")); err != nil {
					t.Fatal(err)
				}
			},
			want:      []string{"install code-review in claude"},
			wantAfter: map[string]State{"claude code-review": Synced},
		},
		{
			name:        "invalid skill matched by a glob is skipped",
			assignments: "claude = [\"obsidian-*\"]\n",
			valid:       []string{"obsidian-daily"},
			invalid:     []string{"obsidian-broken"},
			want:        []string{"install obsidian-daily in claude", "skip obsidian-broken in claude: invalid"},
		},
		{
			name:        "invalid skill named literally is an error",
			assignments: "claude = [\"obsidian-broken\"]\n",
			invalid:     []string{"obsidian-broken"},
			wantErr:     "invalid skills are never installed: obsidian-broken",
		},
		{
			name:        "literal name missing from the vault is an error",
			assignments: "claude = [\"nope\"]\n",
			valid:       []string{"code-review"},
			wantErr:     `no skill "nope" in vault`,
		},
		{
			name:        "glob matching nothing selects nothing",
			assignments: "claude = [\"nope-*\"]\n",
			valid:       []string{"code-review"},
		},
		{
			name:        "unknown agent id is an error",
			assignments: "nope = [\"code-review\"]\n",
			valid:       []string{"code-review"},
			wantErr:     `assignments name unknown agent "nope"`,
		},
		{
			name:        "malformed glob is an error",
			assignments: "claude = [\"[x\"]\n",
			valid:       []string{"code-review"},
			wantErr:     "bad skill pattern",
		},
		{
			name:        "disabled agent is skipped",
			assignments: "claude = [\"code-review\"]\n",
			valid:       []string{"code-review"},
			disabled:    true,
			want:        []string{"skip code-review in claude: agent disabled"},
			wantAfter:   map[string]State{"claude code-review": NotInstalled},
		},
		{
			name:        "agent without an assignment entry is untouched by prune",
			assignments: "agents = [\"code-review\"]\n",
			valid:       []string{"code-review"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				installFor(t, v, agentNamed(t, agents, "cursor"), "code-review")
			},
			opts:      SyncOptions{Prune: true},
			want:      []string{"install code-review in agents"},
			wantAfter: map[string]State{"cursor code-review": Synced, "agents code-review": Synced},
		},
		{
			name:        "prune off keeps unassigned skills",
			assignments: "claude = [\"code-review\"]\n",
			valid:       []string{"code-review", "obsidian-daily"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				installFor(t, v, agentNamed(t, agents, "claude"), "code-review", "obsidian-daily")
			},
			wantAfter: map[string]State{"claude obsidian-daily": Synced},
		},
		{
			name:        "prune uninstalls unassigned skills",
			assignments: "claude = [\"code-review\"]\n",
			valid:       []string{"code-review", "obsidian-daily"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				installFor(t, v, agentNamed(t, agents, "claude"), "code-review", "obsidian-daily")
			},
			opts:      SyncOptions{Prune: true},
			want:      []string{"uninstall obsidian-daily in claude"},
			wantAfter: map[string]State{"claude obsidian-daily": NotInstalled, "claude code-review": Synced},
		},
		{
			name:        "prune removes orphaned and missing entries",
			assignments: "claude = [\"code-review\"]\n",
			valid:       []string{"code-review", "obsidian-daily", "obsidian-notes"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				claude := agentNamed(t, agents, "claude")
				installFor(t, v, claude, "code-review", "obsidian-daily", "obsidian-notes")
				if err := os.RemoveAll(v.SkillDir("obsidian-daily")); err != nil {
					t.Fatal(err)
				}
				if err := os.RemoveAll(filepath.Join(claude.Path, "obsidian-notes")); err != nil {
					t.Fatal(err)
				}
			},
			opts:      SyncOptions{Prune: true},
			want:      []string{"uninstall obsidian-daily in claude", "uninstall obsidian-notes in claude"},
			wantAfter: map[string]State{"claude obsidian-daily": "", "claude obsidian-notes": NotInstalled},
		},
		{
			name:        "prune refuses a modified unassigned skill",
			assignments: "claude = [\"code-review\"]\n",
			valid:       []string{"code-review", "obsidian-daily"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				claude := agentNamed(t, agents, "claude")
				installFor(t, v, claude, "code-review", "obsidian-daily")
				writeTestFile(t, filepath.Join(claude.Path, "obsidian-daily", "notes.md"), "local edit\n")
			},
			opts:    SyncOptions{Prune: true},
			wantErr: "refusing to remove obsidian-daily in claude: modified",
		},
		{
			name:        "prune removes a modified unassigned skill with force",
			assignments: "claude = [\"code-review\"]\n",
			valid:       []string{"code-review", "obsidian-daily"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				claude := agentNamed(t, agents, "claude")
				installFor(t, v, claude, "code-review", "obsidian-daily")
				writeTestFile(t, filepath.Join(claude.Path, "obsidian-daily", "notes.md"), "local edit\n")
			},
			opts:      SyncOptions{Options: Options{Force: true}, Prune: true},
			want:      []string{"uninstall obsidian-daily in claude"},
			wantAfter: map[string]State{"claude obsidian-daily": NotInstalled},
		},
		{
			name:        "prune leaves unmanaged folders alone",
			assignments: "claude = [\"code-review\"]\n",
			valid:       []string{"code-review"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				writeSkillDir(t, filepath.Join(agentNamed(t, agents, "claude").Path, "hand-made"), "hand-made")
			},
			opts:      SyncOptions{Prune: true},
			want:      []string{"install code-review in claude"},
			wantAfter: map[string]State{"claude hand-made": Unmanaged},
		},
		{
			name:        "dry run plans prune and install without changing anything",
			assignments: "claude = [\"code-review\"]\n",
			valid:       []string{"code-review", "obsidian-daily"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				installFor(t, v, agentNamed(t, agents, "claude"), "obsidian-daily")
			},
			opts:      SyncOptions{Options: Options{DryRun: true}, Prune: true},
			want:      []string{"install code-review in claude", "uninstall obsidian-daily in claude"},
			wantAfter: map[string]State{"claude code-review": NotInstalled, "claude obsidian-daily": Synced},
		},
		{
			name:        "a refusal in one agent changes nothing in any agent",
			assignments: "claude = [\"code-review\"]\nagents = [\"obsidian-daily\"]\n",
			valid:       []string{"code-review", "obsidian-daily"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				writeSkillDir(t, filepath.Join(agentNamed(t, agents, "agents").Path, "obsidian-daily"), "obsidian-daily")
			},
			wantErr: "refusing to overwrite obsidian-daily in agents: unmanaged",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := syncVault(t, tt.assignments, tt.valid, tt.invalid)
			agents := syncAgents(t, !tt.disabled)
			if tt.prep != nil {
				tt.prep(t, v, agents)
			}
			before := snapshotAll(t, v, agents)

			dry := tt.opts
			dry.DryRun = true
			plan, err := Sync(v, agents, dry)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Sync() error = %v, want one containing %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("Sync() dry run error = %v", err)
			}
			if after := snapshotAll(t, v, agents); !reflect.DeepEqual(after, before) {
				t.Fatal("dry run changed the vault or an agent dir")
			}
			if tt.wantErr != "" {
				if _, err := Sync(v, agents, tt.opts); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Sync() error = %v, want one containing %q", err, tt.wantErr)
				}
				if after := snapshotAll(t, v, agents); !reflect.DeepEqual(after, before) {
					t.Fatal("refused sync changed the vault or an agent dir")
				}
				return
			}
			if got, want := planLines(plan), sortedCopy(tt.want); !slices.Equal(got, want) {
				t.Fatalf("plan = %v, want %v", got, want)
			}
			if !tt.opts.DryRun {
				actions, err := Sync(v, agents, tt.opts)
				if err != nil {
					t.Fatalf("Sync() error = %v", err)
				}
				if !reflect.DeepEqual(actions, plan) {
					t.Errorf("applied actions = %+v, want the plan %+v", actions, plan)
				}
			}
			checkAfter(t, v, agents, tt.wantAfter)
		})
	}
}

// checkAfter compares the status of each "agent skill" key with want. An empty
// want means the pair has no row at all.
func checkAfter(t *testing.T, v *vault.Vault, agents []config.Agent, want map[string]State) {
	t.Helper()
	rows, err := Status(v, agents, Filter{})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	got := make(map[string]State, len(rows))
	for _, r := range rows {
		got[r.Agent+" "+r.Skill] = r.Status
	}
	for key, state := range want {
		if got[key] != state {
			t.Errorf("after sync %s = %q, want %q", key, got[key], state)
		}
	}
}

func TestSyncSkipsDisabledAgentAndInstallsTheRest(t *testing.T) {
	v := syncVault(t, "claude = [\"code-review\"]\nagents = [\"code-review\"]\n", []string{"code-review"}, nil)
	agents := syncAgents(t, false)

	actions, err := Sync(v, agents, SyncOptions{Options: Options{DryRun: true}})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	want := []string{"install code-review in agents", "skip code-review in claude: agent disabled"}
	if got := planLines(actions); !slices.Equal(got, sortedCopy(want)) {
		t.Errorf("plan = %v, want %v", got, sortedCopy(want))
	}
}

func TestSyncDoesNothingWithoutAssignments(t *testing.T) {
	v := syncVault(t, "", []string{"code-review"}, nil)
	agents := syncAgents(t, true)
	installFor(t, v, agents[0], "code-review")
	before := snapshotAll(t, v, agents)

	actions, err := Sync(v, agents, SyncOptions{Prune: true})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if len(actions) != 0 {
		t.Errorf("actions = %v, want none", planLines(actions))
	}
	if after := snapshotAll(t, v, agents); !reflect.DeepEqual(after, before) {
		t.Error("sync without assignments changed something")
	}
}
