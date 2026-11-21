package ops

import (
	"errors"
	"path/filepath"
	"testing"

	"sapato/internal/config"
	"sapato/internal/vault"
)

func TestRefusalsNeedForce(t *testing.T) {
	tests := []struct {
		name      string
		prep      func(t *testing.T, agent config.Agent)
		run       func(v *vault.Vault, agent config.Agent) error
		wantErr   string
		wantForce bool
	}{
		{
			name: "install over a modified copy",
			run: func(v *vault.Vault, agent config.Agent) error {
				_, err := Install(v, []config.Agent{agent}, []string{"mocked-skill-00003"}, Options{})
				return err
			},
			wantErr:   "install: refusing to overwrite mocked-skill-00003 in claude: modified (use --force)",
			wantForce: true,
		},
		{
			name: "install over an unmanaged folder",
			prep: func(t *testing.T, agent config.Agent) {
				writeTestFile(t, filepath.Join(agent.Path, "mocked-skill-00004", "SKILL.md"), "mine")
			},
			run: func(v *vault.Vault, agent config.Agent) error {
				_, err := Install(v, []config.Agent{agent}, []string{"mocked-skill-00004"}, Options{})
				return err
			},
			wantErr:   "install: refusing to overwrite mocked-skill-00004 in claude: unmanaged (use --force)",
			wantForce: true,
		},
		{
			name: "uninstall of a modified copy",
			run: func(v *vault.Vault, agent config.Agent) error {
				_, err := Uninstall(v, []config.Agent{agent}, []string{"mocked-skill-00003"}, Options{})
				return err
			},
			wantErr:   "uninstall: refusing to remove mocked-skill-00003 in claude: modified (use --force)",
			wantForce: true,
		},
		{
			name: "uninstall of an unmanaged folder is never overridable",
			run: func(v *vault.Vault, agent config.Agent) error {
				_, err := Uninstall(v, []config.Agent{agent}, []string{"hand-made-skill"}, Options{Force: true})
				return err
			},
			wantErr:   "uninstall: refusing to remove hand-made-skill in claude: unmanaged",
			wantForce: false,
		},
		{
			name: "adopt of a name already in the vault",
			prep: func(t *testing.T, agent config.Agent) {
				writeSkillDir(t, filepath.Join(agent.Path, "mocked-skill-00004"), "mocked-skill-00004")
			},
			run: func(v *vault.Vault, agent config.Agent) error {
				_, err := Adopt(v, agent, "mocked-skill-00004", Options{})
				return err
			},
			wantErr:   "adopt mocked-skill-00004 in claude: already in the vault (use --force)",
			wantForce: true,
		},
		{
			name: "adopt of a folder that is not unmanaged is not overridable",
			run: func(v *vault.Vault, agent config.Agent) error {
				_, err := Adopt(v, agent, "mocked-skill-00001", Options{Force: true})
				return err
			},
			wantErr:   "adopt mocked-skill-00001 in claude: synced, want unmanaged",
			wantForce: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := fixtureVault(t)
			agent := fixtureAgent(t, "claude")
			if tt.prep != nil {
				tt.prep(t, agent)
			}
			err := tt.run(v, agent)
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
			if got := errors.Is(err, ErrNeedsForce); got != tt.wantForce {
				t.Errorf("errors.Is(err, ErrNeedsForce) = %t, want %t", got, tt.wantForce)
			}
		})
	}
}
