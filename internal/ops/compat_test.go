package ops

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"sapato/internal/config"
	"sapato/internal/vault"
)

// tomlText returns the sapato.toml of v.
func tomlText(t *testing.T, v *vault.Vault) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(v.Path, "sapato.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// setCompatFor sets the compatible list of skill in v for a test.
func setCompatFor(t *testing.T, v *vault.Vault, skill string, agents ...string) {
	t.Helper()
	if err := SetCompat(v, skill, agents, Options{}); err != nil {
		t.Fatalf("SetCompat(%s) error = %v", skill, err)
	}
}

func TestSetCompat(t *testing.T) {
	tests := []struct {
		name     string
		skill    string
		agents   []string // nil removes the table
		setFirst []string // list set before the call, when non-nil
		opts     Options
		wantErr  string
		want     []string // Compat after the call
	}{
		{
			name:   "list is set, names need not be configured",
			skill:  "mocked-skill-00001",
			agents: []string{"claude", "codex"},
			want:   []string{"claude", "codex"},
		},
		{
			name:   "duplicate names are kept once, in order",
			skill:  "mocked-skill-00001",
			agents: []string{"cursor", "claude", "cursor"},
			want:   []string{"cursor", "claude"},
		},
		{
			name:     "nil removes the table",
			skill:    "mocked-skill-00001",
			setFirst: []string{"claude"},
			agents:   nil,
			want:     nil,
		},
		{
			name:    "unknown skill is refused",
			skill:   "no-such-skill",
			agents:  []string{"claude"},
			wantErr: "no skill in vault",
		},
		{
			name:    "empty list is refused",
			skill:   "mocked-skill-00001",
			agents:  []string{},
			wantErr: "compatible list is empty",
		},
		{
			name:    "bad agent name is refused",
			skill:   "mocked-skill-00001",
			agents:  []string{"Claude"},
			wantErr: "agent name \"Claude\" is invalid",
		},
		{
			name:     "dry run changes nothing",
			skill:    "mocked-skill-00001",
			setFirst: []string{"claude"},
			agents:   []string{"cursor"},
			opts:     Options{DryRun: true},
			want:     []string{"claude"},
		},
		{
			name:   "dry run with nil changes nothing",
			skill:  "mocked-skill-00001",
			agents: nil,
			opts:   Options{DryRun: true},
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			if tt.setFirst != nil {
				setCompatFor(t, v, tt.skill, tt.setFirst...)
			}
			before := tomlText(t, v)
			err := SetCompat(v, tt.skill, tt.agents, tt.opts)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("SetCompat() error = %v, want containing %q", err, tt.wantErr)
				}
				if tomlText(t, v) != before {
					t.Error("a refused call changed sapato.toml")
				}
				return
			}
			if err != nil {
				t.Fatalf("SetCompat() error = %v", err)
			}
			if got := v.CompatAgents(tt.skill); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CompatAgents() = %v, want %v", got, tt.want)
			}
			if tt.opts.DryRun && tomlText(t, v) != before {
				t.Error("dry run changed sapato.toml")
			}
			reopened, err := vault.Open(v.Path)
			if err != nil {
				t.Fatal(err)
			}
			if got := reopened.CompatAgents(tt.skill); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("reopened CompatAgents() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompat(t *testing.T) {
	v := fixtureVault(t)
	setCompatFor(t, v, "mocked-skill-00001", "claude", "cursor")

	tests := []struct {
		name    string
		skill   string
		want    []string
		wantErr string
	}{
		{name: "list is returned", skill: "mocked-skill-00001", want: []string{"claude", "cursor"}},
		{name: "no table means every agent", skill: "mocked-skill-00002", want: nil},
		{name: "unknown skill is refused", skill: "no-such-skill", wantErr: "no skill in vault"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Compat(v, tt.skill)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Compat() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Compat() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Compat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToggleCompat(t *testing.T) {
	tests := []struct {
		name       string
		skill      string
		setFirst   []string // list set before the call, when non-nil
		agents     []string // agents to toggle
		configured []string
		wantErr    string
		want       []string // the list returned, and the one stored after
	}{
		{
			name:       "no list: toggling an agent off keeps the others",
			skill:      "mocked-skill-00001",
			agents:     []string{"cursor"},
			configured: []string{"claude", "cursor"},
			want:       []string{"claude"},
		},
		{
			name:       "toggling back on to every configured agent removes the table",
			skill:      "mocked-skill-00001",
			setFirst:   []string{"claude"},
			agents:     []string{"cursor"},
			configured: []string{"claude", "cursor"},
			want:       nil,
		},
		{
			name:       "several agents toggle in one call",
			skill:      "mocked-skill-00001",
			agents:     []string{"claude", "cursor"},
			configured: []string{"claude", "cursor", "codex"},
			want:       []string{"codex"},
		},
		{
			name:       "toggling on adds an agent to the list",
			skill:      "mocked-skill-00001",
			setFirst:   []string{"claude"},
			agents:     []string{"cursor"},
			configured: []string{"claude", "cursor", "codex"},
			want:       []string{"claude", "cursor"},
		},
		{
			name:       "names not configured here are kept",
			skill:      "mocked-skill-00001",
			setFirst:   []string{"codex"},
			agents:     []string{"claude"},
			configured: []string{"claude"},
			want:       []string{"codex", "claude"},
		},
		{
			name:       "removing the last compatible agent is refused",
			skill:      "mocked-skill-00001",
			agents:     []string{"claude"},
			configured: []string{"claude"},
			wantErr:    "compatible list is empty",
		},
		{
			name:       "unknown skill is refused",
			skill:      "no-such-skill",
			agents:     []string{"claude"},
			configured: []string{"claude", "cursor"},
			wantErr:    "no skill in vault",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			if tt.setFirst != nil {
				setCompatFor(t, v, tt.skill, tt.setFirst...)
			}
			before := tomlText(t, v)
			got, err := ToggleCompat(v, tt.skill, tt.agents, tt.configured)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ToggleCompat() error = %v, want containing %q", err, tt.wantErr)
				}
				if tomlText(t, v) != before {
					t.Error("a refused toggle changed sapato.toml")
				}
				return
			}
			if err != nil {
				t.Fatalf("ToggleCompat() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToggleCompat() = %v, want %v", got, tt.want)
			}
			if stored := v.CompatAgents(tt.skill); !reflect.DeepEqual(stored, tt.want) {
				t.Errorf("stored list = %v, want %v", stored, tt.want)
			}
		})
	}
}

func TestClassifyIncompatible(t *testing.T) {
	tests := []struct {
		name string
		p    presence
		want State
	}{
		{
			name: "not installed and incompatible",
			p:    presence{inVault: true, valid: true, incompatible: true},
			want: Incompatible,
		},
		{
			name: "invalid and incompatible is incompatible, not invalid",
			p:    presence{inVault: true, valid: false, incompatible: true},
			want: Incompatible,
		},
		{
			name: "unmanaged copy wins over incompatible",
			p:    presence{inVault: true, valid: true, incompatible: true, inTarget: true},
			want: Unmanaged,
		},
		{
			name: "locked copy keeps its status when incompatible",
			p:    presence{inVault: true, valid: true, incompatible: true, inLock: true, inTarget: true, lockHash: "h", targetHash: "h", vaultHash: "h"},
			want: Synced,
		},
		{
			name: "compatible and not installed stays not-installed",
			p:    presence{inVault: true, valid: true},
			want: NotInstalled,
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

func TestStatusCompatible(t *testing.T) {
	v := fixtureVault(t)
	setCompatFor(t, v, "mocked-skill-00001", "agents")
	setCompatFor(t, v, "mocked-skill-00006", "agents")
	agents := []config.Agent{fixtureAgent(t, "claude"), fixtureAgent(t, "agents")}

	rows, err := Status(v, agents, Filter{Skills: []string{"mocked-skill-00001", "mocked-skill-00006"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []Row{
		// Installed in claude, which the skill is not compatible with: status is kept.
		{Skill: "mocked-skill-00001", Agent: "claude", Status: Synced, Compatible: false},
		{Skill: "mocked-skill-00001", Agent: "agents", Status: Synced, Compatible: true},
		// Not installed in claude and not compatible: incompatible replaces not-installed.
		{Skill: "mocked-skill-00006", Agent: "claude", Status: Incompatible, Compatible: false},
		{Skill: "mocked-skill-00006", Agent: "agents", Status: NotInstalled, Compatible: true},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("Status() =\n%+v\nwant\n%+v", rows, want)
	}
}

func TestInstallIncompatible(t *testing.T) {
	tests := []struct {
		name        string
		agents      []string // agent ids of the fixture agents to install into
		skills      []string
		opts        Options
		wantErr     string
		wantInstall []string // agent ids that get the skill
		wantSkip    bool     // the call reports an incompatible skip
	}{
		{
			name:    "literal name without force is refused",
			agents:  []string{"claude"},
			skills:  []string{"mocked-skill-00006"},
			wantErr: "refusing to install mocked-skill-00006 in claude: incompatible (use --force)",
		},
		{
			name:    "refusal in one agent changes nothing in the others",
			agents:  []string{"agents", "claude"},
			skills:  []string{"mocked-skill-00006"},
			wantErr: "refusing to install mocked-skill-00006 in claude: incompatible",
		},
		{
			name:        "literal name with force installs",
			agents:      []string{"claude"},
			skills:      []string{"mocked-skill-00006"},
			opts:        Options{Force: true},
			wantInstall: []string{"claude"},
		},
		{
			name:     "glob skips and reports",
			agents:   []string{"claude"},
			skills:   []string{"mocked-skill-0000[6]"},
			wantSkip: true,
		},
		{
			name:     "SkipIncompatible skips a literal name",
			agents:   []string{"claude"},
			skills:   []string{"mocked-skill-00006"},
			opts:     Options{SkipIncompatible: true},
			wantSkip: true,
		},
		{
			name:        "compatible agent gets the skill",
			agents:      []string{"agents"},
			skills:      []string{"mocked-skill-00006"},
			wantInstall: []string{"agents"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			setCompatFor(t, v, "mocked-skill-00006", "agents")
			all := []config.Agent{fixtureAgent(t, "claude"), fixtureAgent(t, "agents")}
			var agents []config.Agent
			for _, id := range tt.agents {
				agents = append(agents, agentNamed(t, all, id))
			}
			before := make(map[string]map[string]string)
			for _, a := range all {
				before[a.ID] = snapshot(t, a.Path)
			}

			actions, err := Install(v, agents, tt.skills, tt.opts)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Install() error = %v, want containing %q", err, tt.wantErr)
				}
				var refusal *ForceRefusal
				if !errors.As(err, &refusal) {
					t.Errorf("error %v is not a ForceRefusal", err)
				}
				for _, a := range all {
					if !reflect.DeepEqual(snapshot(t, a.Path), before[a.ID]) {
						t.Errorf("refused call changed %s", a.ID)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Install() error = %v", err)
			}
			skipped := slices.ContainsFunc(actions, func(a Action) bool {
				return a.Op == OpSkip && a.Reason == "incompatible" && a.Skill == "mocked-skill-00006"
			})
			if skipped != tt.wantSkip {
				t.Errorf("incompatible skip reported = %v, want %v (actions %v)", skipped, tt.wantSkip, summarize(actions))
			}
			for _, a := range all {
				_, installed := snapshot(t, a.Path)[filepath.Join("mocked-skill-00006", "SKILL.md")]
				if want := slices.Contains(tt.wantInstall, a.ID); installed != want {
					t.Errorf("installed in %s = %v, want %v", a.ID, installed, want)
				}
			}
		})
	}
}

func TestSyncSkipsIncompatible(t *testing.T) {
	v := syncVault(t, "claude = [\"code-review\"]\nagents = [\"code-review\"]\n", []string{"code-review"}, nil)
	setCompatFor(t, v, "code-review", "agents")
	agents := syncAgents(t)

	actions, err := Sync(v, agents, SyncOptions{})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if !slices.Contains(planLines(actions), "skip code-review in claude: incompatible") {
		t.Errorf("plan = %v, want the incompatible skip", planLines(actions))
	}
	if _, err := os.Stat(filepath.Join(agentNamed(t, agents, "claude").Path, "code-review")); !os.IsNotExist(err) {
		t.Errorf("claude has code-review (stat err %v)", err)
	}
	if _, err := os.Stat(filepath.Join(agentNamed(t, agents, "agents").Path, "code-review", "SKILL.md")); err != nil {
		t.Errorf("agents lacks code-review: %v", err)
	}
}

func TestRemoveDeletesCompatTable(t *testing.T) {
	v := fixtureVault(t)
	setCompatFor(t, v, "mocked-skill-00005", "agents")
	setCompatFor(t, v, "mocked-skill-00006", "claude")

	if _, err := Remove(v, []config.Agent{fixtureAgent(t, "agents")}, "mocked-skill-00006", Options{}); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	text := tomlText(t, v)
	if strings.Contains(text, "mocked-skill-00006") {
		t.Errorf("sapato.toml still names the removed skill:\n%s", text)
	}
	if !strings.Contains(text, "[skills.mocked-skill-00005]") {
		t.Errorf("sapato.toml lost another skill's table:\n%s", text)
	}
	if got := v.CompatAgents("mocked-skill-00006"); got != nil {
		t.Errorf("CompatAgents after remove = %v, want nil", got)
	}
}

func TestDoctorCompatFindings(t *testing.T) {
	v := fixtureVault(t)
	setCompatFor(t, v, "mocked-skill-00001", "agents", "codex")
	claudeDir := fixtureAgent(t, "claude").Path
	agentsDir := fixtureAgent(t, "agents").Path
	agents := []config.Agent{
		{ID: "claude", Path: claudeDir, ReadDirs: []string{agentsDir}},
		{ID: "agents", Path: agentsDir},
	}

	findings, err := Doctor(v, agents)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, line := range findingLines(findings) {
		fields := strings.Fields(line)
		switch fields[1] {
		case string(FindingIncompatible), string(FindingCompatHere), string(FindingIncompatRead):
			got = append(got, line)
		}
	}
	want := []string{
		"info compat-not-configured-here codex mocked-skill-00001",
		"info incompatible-read claude mocked-skill-00001",
		"warn incompatible claude mocked-skill-00001",
	}
	slices.Sort(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("compat findings =\n%q\nwant\n%q", got, want)
	}
}

func TestExportImportCompatRoundTrip(t *testing.T) {
	src := fixtureVault(t)
	setCompatFor(t, src, "mocked-skill-00001", "claude", "cursor")
	path := exportTo(t, src, "mocked-skill-00001", "mocked-skill-00002")

	m := archiveManifest(t, mustRead(t, path))
	wantCompat := map[string][]string{"mocked-skill-00001": {"claude", "cursor"}}
	if !reflect.DeepEqual(m.Compatible, wantCompat) {
		t.Errorf("manifest compatible = %v, want %v", m.Compatible, wantCompat)
	}

	dst := emptyVault(t)
	if _, err := Import(dst, path, ImportOptions{}); err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	reopened, err := vault.Open(dst.Path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.CompatAgents("mocked-skill-00001"); !reflect.DeepEqual(got, []string{"claude", "cursor"}) {
		t.Errorf("imported CompatAgents = %v, want [claude cursor]", got)
	}
	if got := reopened.CompatAgents("mocked-skill-00002"); got != nil {
		t.Errorf("imported CompatAgents(00002) = %v, want nil", got)
	}
}

func TestImportArchiveCompat(t *testing.T) {
	plain := fixtureVault(t)
	plainExport := exportTo(t, plain, "mocked-skill-00001")
	noManifest := makeTarball(t, skillEntries("mocked-skill-00001")...)

	tests := []struct {
		name    string
		archive string
		want    []string // compat of mocked-skill-00001 in the target after a forced import
	}{
		{
			name:    "manifest without a compatible list clears the table",
			archive: plainExport,
			want:    nil,
		},
		{
			name:    "archive without a manifest keeps the table",
			archive: noManifest,
			want:    []string{"cursor"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			setCompatFor(t, v, "mocked-skill-00001", "cursor")
			if _, err := Import(v, tt.archive, ImportOptions{Options: Options{Force: true}}); err != nil {
				t.Fatalf("Import() error = %v", err)
			}
			if got := v.CompatAgents("mocked-skill-00001"); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CompatAgents() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckManifestCompat(t *testing.T) {
	skills := []vault.Skill{{Name: "x", Hash: "h"}}
	tests := []struct {
		name       string
		compatible map[string][]string
		wantErr    string
	}{
		{name: "valid list", compatible: map[string][]string{"x": {"claude"}}},
		{name: "no list", compatible: nil},
		{name: "list for a skill not in the archive", compatible: map[string][]string{"y": {"claude"}}, wantErr: "not in the archive"},
		{name: "empty list", compatible: map[string][]string{"x": {}}, wantErr: "compatible list is empty"},
		{name: "bad agent name", compatible: map[string][]string{"x": {"Claude"}}, wantErr: "agent name \"Claude\" is invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manifest{Format: manifestFormat, Skills: map[string]string{"x": "h"}, Compatible: tt.compatible}
			err := checkManifest(m, skills)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("checkManifest() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("checkManifest() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeCompat(t *testing.T) {
	configured := []string{"claude", "cursor", "pi"}
	tests := []struct {
		name   string
		agents []string
		want   []string
		isNil  bool
	}{
		{name: "nil stays nil", agents: nil, isNil: true},
		{name: "every configured agent is no table", agents: []string{"pi", "claude", "cursor"}, isNil: true},
		{name: "a subset is kept as given", agents: []string{"pi", "claude"}, want: []string{"pi", "claude"}},
		{name: "an empty list is kept, so it is refused later", agents: []string{}, want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeCompat(tt.agents, configured)
			if tt.isNil {
				if got != nil {
					t.Errorf("NormalizeCompat() = %q, want nil", got)
				}
				return
			}
			if got == nil || !slices.Equal(got, tt.want) {
				t.Errorf("NormalizeCompat() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
