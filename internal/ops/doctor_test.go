package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"sapato/internal/config"
	"sapato/internal/vault"
)

// findingLines renders findings as sorted "severity kind agent skill" lines. An
// empty agent or skill is "-".
func findingLines(findings []Finding) []string {
	out := make([]string, 0, len(findings))
	for _, f := range findings {
		out = append(out, fmt.Sprintf("%s %s %s %s", f.Severity, f.Kind, orDashStr(f.Agent), orDashStr(f.Skill)))
	}
	slices.Sort(out)
	return out
}

func orDashStr(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func TestDoctor(t *testing.T) {
	tests := []struct {
		name    string
		valid   []string
		invalid []string
		prep    func(t *testing.T, v *vault.Vault, agents []config.Agent)
		want    []string
	}{
		{
			name:  "clean vault and targets have no findings",
			valid: []string{"code-review"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				installFor(t, v, agentNamed(t, agents, "claude"), "code-review")
			},
		},
		{
			name:    "invalid vault skill is an error",
			valid:   []string{"code-review"},
			invalid: []string{"broken"},
			want:    []string{"error invalid-skill - broken"},
		},
		{
			name:  "corrupt lock is an error and the other agents are still checked",
			valid: []string{"code-review"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				installFor(t, v, agentNamed(t, agents, "agents"), "code-review")
				writeTestFile(t, filepath.Join(agentNamed(t, agents, "claude").Path, ".sapato-lock.json"), "{not json")
				if err := os.RemoveAll(v.SkillDir("code-review")); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"error corrupt-lock claude -", "warn orphaned agents code-review"},
		},
		{
			name:  "modified installed copy is a warning",
			valid: []string{"code-review"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				claude := agentNamed(t, agents, "claude")
				installFor(t, v, claude, "code-review")
				writeTestFile(t, filepath.Join(claude.Path, "code-review", "notes.md"), "local edit\n")
			},
			want: []string{"warn modified claude code-review"},
		},
		{
			name:  "missing folder is a warning",
			valid: []string{"code-review"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				claude := agentNamed(t, agents, "claude")
				installFor(t, v, claude, "code-review")
				if err := os.RemoveAll(filepath.Join(claude.Path, "code-review")); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"warn missing claude code-review"},
		},
		{
			name:  "orphaned copy is a warning",
			valid: []string{"code-review"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				installFor(t, v, agentNamed(t, agents, "claude"), "code-review")
				if err := os.RemoveAll(v.SkillDir("code-review")); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"warn orphaned claude code-review"},
		},
		{
			name:  "skill visible to cursor from its own dir and from claude is a warning",
			valid: []string{"code-review"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				installFor(t, v, agentNamed(t, agents, "claude"), "code-review")
				writeSkillDir(t, filepath.Join(agentNamed(t, agents, "cursor").Path, "code-review"), "code-review")
			},
			want: []string{"warn overlap cursor code-review"},
		},
		{
			name:  "opencode reads claude and agents, so each shared skill is reported per dir",
			valid: []string{"code-review"},
			prep: func(t *testing.T, v *vault.Vault, agents []config.Agent) {
				installFor(t, v, agentNamed(t, agents, "claude"), "code-review")
				installFor(t, v, agentNamed(t, agents, "agents"), "code-review")
				writeSkillDir(t, filepath.Join(agentNamed(t, agents, "opencode").Path, "code-review"), "code-review")
			},
			want: []string{"warn overlap opencode code-review", "warn overlap opencode code-review"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := syncVault(t, "", tt.valid, tt.invalid)
			agents := syncAgents(t, true)
			if tt.prep != nil {
				tt.prep(t, v, agents)
			}
			before := snapshotAll(t, v, agents)

			findings, err := Doctor(v, agents)
			if err != nil {
				t.Fatalf("Doctor() error = %v", err)
			}
			if got, want := findingLines(findings), sortedCopy(tt.want); !slices.Equal(got, want) {
				t.Errorf("findings = %v, want %v", got, want)
			}
			for _, f := range findings {
				if f.Message == "" {
					t.Errorf("finding %+v has no message", f)
				}
			}
			if after := snapshotAll(t, v, agents); !reflect.DeepEqual(after, before) {
				t.Error("doctor changed the vault or an agent dir")
			}
		})
	}
}

func TestDoctorOverlapUsesFixedReadDirs(t *testing.T) {
	t.Run("cursor path that is a symlink to its read dir is not an overlap", func(t *testing.T) {
		v := syncVault(t, "", []string{"code-review"}, nil)
		agents := syncAgents(t, true)
		claude := agentNamed(t, agents, "claude")
		cursor := agentNamed(t, agents, "cursor")
		installFor(t, v, claude, "code-review")
		if err := os.RemoveAll(cursor.Path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(claude.Path, cursor.Path); err != nil {
			t.Fatal(err)
		}

		findings, err := Doctor(v, agents)
		if err != nil {
			t.Fatalf("Doctor() error = %v", err)
		}
		if len(findings) != 0 {
			t.Errorf("findings = %v, want none", findingLines(findings))
		}
	})

	t.Run("read dirs that resolve to one dir are reported once", func(t *testing.T) {
		v := syncVault(t, "", []string{"code-review"}, nil)
		agents := syncAgents(t, true)
		claude := agentNamed(t, agents, "claude")
		agentsDir := agentNamed(t, agents, "agents")
		opencode := agentNamed(t, agents, "opencode")
		installFor(t, v, agentsDir, "code-review")
		writeSkillDir(t, filepath.Join(opencode.Path, "code-review"), "code-review")
		if err := os.RemoveAll(claude.Path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(agentsDir.Path, claude.Path); err != nil {
			t.Fatal(err)
		}

		findings, err := Doctor(v, agents)
		if err != nil {
			t.Fatalf("Doctor() error = %v", err)
		}
		if got := findingLines(findings); !slices.Equal(got, []string{"warn overlap opencode code-review"}) {
			t.Errorf("findings = %v, want one overlap", got)
		}
	})

	t.Run("overridden claude path still overlaps with the default claude dir", func(t *testing.T) {
		v := syncVault(t, "", []string{"code-review"}, nil)
		home := t.TempDir()
		custom := filepath.Join(t.TempDir(), "custom-claude")
		cfgPath := filepath.Join(home, "config.toml")
		writeTestFile(t, cfgPath, "[agents.claude]\npath = \""+custom+"\"\n")
		cfg, err := config.Load(cfgPath, home)
		if err != nil {
			t.Fatal(err)
		}
		agents := cfg.Agents
		defaultClaude := filepath.Join(home, ".claude", "skills")
		writeSkillDir(t, filepath.Join(defaultClaude, "code-review"), "code-review")
		writeSkillDir(t, filepath.Join(agentNamed(t, agents, "cursor").Path, "code-review"), "code-review")

		findings, err := Doctor(v, agents)
		if err != nil {
			t.Fatalf("Doctor() error = %v", err)
		}
		if got := findingLines(findings); !slices.Equal(got, []string{"warn overlap cursor code-review"}) {
			t.Fatalf("findings = %v, want one overlap", got)
		}
		if !strings.Contains(findings[0].Message, defaultClaude) {
			t.Errorf("message %q does not name the fixed read dir %s", findings[0].Message, defaultClaude)
		}
	})
}

func TestDoctorFixtures(t *testing.T) {
	v := fixtureVault(t)
	agents := []config.Agent{fixtureAgent(t, "claude"), fixtureAgent(t, "agents"), fixtureAgent(t, "cursor")}

	findings, err := Doctor(v, agents)
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	want := []string{
		"error invalid-skill - Invalid-Uppercase",
		"error invalid-skill - invalid-long-description",
		"error invalid-skill - invalid-missing-description",
		"error invalid-skill - invalid-name-mismatch",
		"error invalid-skill - invalid-no-frontmatter",
		"error invalid-skill - invalid-no-skill-md",
		"warn missing claude mocked-skill-00098",
		"warn modified claude mocked-skill-00003",
		"warn orphaned claude mocked-skill-00099",
	}
	if got := findingLines(findings); !slices.Equal(got, sortedCopy(want)) {
		t.Errorf("findings = %v, want %v", got, want)
	}
}
