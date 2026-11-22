package ops

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sapato/internal/config"
	"sapato/internal/vault"
)

func TestGuideIsValidSkill(t *testing.T) {
	s := vault.ParseContent(Guide("/vaults/main", []config.Agent{{ID: "claude", Path: "/home/u/.claude/skills"}}))
	if !s.Valid {
		t.Fatalf("guide is invalid: %v", s.Problems)
	}
	if s.Name != "sapato" {
		t.Errorf("name = %q, want sapato", s.Name)
	}
}

func TestGuideContents(t *testing.T) {
	agents := []config.Agent{
		{ID: "cursor", Path: "/home/u/.cursor/skills"},
		{ID: "claude", Path: "/home/u/.claude/skills"},
		{ID: "agents", Path: "/home/u/.agents/skills"},
	}
	tests := []struct {
		name     string
		agents   []config.Agent
		contains []string
		order    []string // agent lines that must appear in this order
	}{
		{
			name:     "vault path and agents in config order",
			agents:   agents,
			contains: []string{"Vault path: /vaults/main\n", "- cursor: /home/u/.cursor/skills\n"},
			order: []string{
				"- cursor: /home/u/.cursor/skills\n",
				"- claude: /home/u/.claude/skills\n",
				"- agents: /home/u/.agents/skills\n",
			},
		},
		{
			name:     "no agents",
			contains: []string{"No agents are configured."},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := string(Guide("/vaults/main", tt.agents))
			for _, want := range tt.contains {
				if !strings.Contains(text, want) {
					t.Errorf("guide does not contain %q", want)
				}
			}
			last := -1
			for _, line := range tt.order {
				i := strings.Index(text, line)
				if i < 0 {
					t.Fatalf("guide does not contain %q", line)
				}
				if i < last {
					t.Errorf("%q is out of order", line)
				}
				last = i
			}
		})
	}
}

func TestGuideInstall(t *testing.T) {
	agents := []config.Agent{{ID: "claude", Path: "/home/u/.claude/skills"}}

	tests := []struct {
		name    string
		prepare func(t *testing.T, v *vault.Vault)
		opts    Options
		wantOp  string
		wantErr bool
		force   bool // a refusal that --force overrides
		// wantGuide and after check the SKILL.md afterwards: wantGuide the guide
		// of the vault, after any other content. Neither set means no check.
		wantGuide bool
		after     string
	}{
		{
			name:      "fresh vault gets the guide",
			wantOp:    OpCreate,
			wantGuide: true,
		},
		{
			name: "identical copy is left alone",
			prepare: func(t *testing.T, v *vault.Vault) {
				mustInstallGuide(t, v, agents)
			},
			wantOp:    OpSkip,
			wantGuide: true,
		},
		{
			name: "different content is refused",
			prepare: func(t *testing.T, v *vault.Vault) {
				writeGuideFile(t, v, "---\nname: sapato\ndescription: Mine.\n---\n")
			},
			wantErr: true,
			force:   true,
		},
		{
			name: "an extra file in the folder counts as different",
			prepare: func(t *testing.T, v *vault.Vault) {
				mustInstallGuide(t, v, agents)
				writeGuideExtra(t, v)
			},
			wantErr: true,
			force:   true,
		},
		{
			name: "force replaces different content",
			prepare: func(t *testing.T, v *vault.Vault) {
				writeGuideFile(t, v, "---\nname: sapato\ndescription: Mine.\n---\n")
			},
			opts:      Options{Force: true},
			wantOp:    OpCreate,
			wantGuide: true,
		},
		{
			name:   "dry run on a fresh vault changes nothing",
			opts:   Options{DryRun: true},
			wantOp: OpCreate,
		},
		{
			name: "dry run on different content changes nothing",
			prepare: func(t *testing.T, v *vault.Vault) {
				writeGuideFile(t, v, "---\nname: sapato\ndescription: Mine.\n---\n")
			},
			opts:   Options{Force: true, DryRun: true},
			wantOp: OpCreate,
			after:  "---\nname: sapato\ndescription: Mine.\n---\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := emptyVault(t)
			if tt.prepare != nil {
				tt.prepare(t, v)
			}
			before := snapshot(t, v.Path)

			action, err := GuideInstall(v, agents, tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Fatal("GuideInstall() succeeded, want a refusal")
				}
				if got := errors.Is(err, ErrNeedsForce); got != tt.force {
					t.Errorf("errors.Is(err, ErrNeedsForce) = %v, want %v", got, tt.force)
				}
				if after := snapshot(t, v.Path); !reflect.DeepEqual(after, before) {
					t.Error("vault changed on a refused install")
				}
				return
			}
			if err != nil {
				t.Fatalf("GuideInstall() error = %v", err)
			}
			if action.Op != tt.wantOp || action.Skill != "sapato" || action.Path != v.SkillDir("sapato") {
				t.Errorf("action = %+v, want op %s for sapato at %s", action, tt.wantOp, v.SkillDir("sapato"))
			}
			if tt.opts.DryRun {
				if after := snapshot(t, v.Path); !reflect.DeepEqual(after, before) {
					t.Error("vault changed on a dry run")
				}
			}
			if !tt.wantGuide && tt.after == "" {
				return
			}
			want := tt.after
			if tt.wantGuide {
				want = string(Guide(v.Path, agents))
			}
			got, err := os.ReadFile(filepath.Join(v.SkillDir("sapato"), "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != want {
				t.Errorf("SKILL.md does not match the expected content (%d bytes, want %d)", len(got), len(want))
			}
		})
	}
}

// mustInstallGuide writes the guide into v for a test.
func mustInstallGuide(t *testing.T, v *vault.Vault, agents []config.Agent) {
	t.Helper()
	if _, err := GuideInstall(v, agents, Options{}); err != nil {
		t.Fatalf("GuideInstall() error = %v", err)
	}
}

// writeGuideFile replaces the vault's sapato SKILL.md with content for a test.
func writeGuideFile(t *testing.T, v *vault.Vault, content string) {
	t.Helper()
	dir := v.SkillDir("sapato")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeGuideExtra adds a file next to the vault's sapato SKILL.md for a test.
func writeGuideExtra(t *testing.T, v *vault.Vault) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(v.SkillDir("sapato"), "notes.md"), []byte("extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
