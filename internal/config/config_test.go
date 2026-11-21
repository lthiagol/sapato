package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func defaultAgents(home string) []Agent {
	return []Agent{
		{ID: "claude", Path: filepath.Join(home, ".claude", "skills"), Enabled: true},
		{ID: "cursor", Path: filepath.Join(home, ".cursor", "skills"), Enabled: true},
		{ID: "pi", Path: filepath.Join(home, ".pi", "agent", "skills"), Enabled: true},
		{ID: "opencode", Path: filepath.Join(home, ".config", "opencode", "skills"), Enabled: true},
		{ID: "agents", Path: filepath.Join(home, ".agents", "skills"), Enabled: true},
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name      string
		content   string // written to the config path; empty means no file
		noFile    bool
		path      func(home string) string // explicit path arg; nil means ""
		wantErr   string
		wantVault func(home string) string
		check     func(t *testing.T, home string, cfg Config)
	}{
		{
			name:   "missing file gives defaults",
			noFile: true,
			wantVault: func(home string) string {
				return filepath.Join(home, "sapato-vault")
			},
			check: func(t *testing.T, home string, cfg Config) {
				if want := defaultAgents(home); !reflect.DeepEqual(cfg.Agents, want) {
					t.Errorf("agents = %+v, want %+v", cfg.Agents, want)
				}
			},
		},
		{
			name:    "empty file gives defaults",
			content: "",
			wantVault: func(home string) string {
				return filepath.Join(home, "sapato-vault")
			},
		},
		{
			name:    "vault and agent overrides",
			content: "vault = \"/data/vault\"\n\n[agents.claude]\npath = \"/x/claude\"\nenabled = false\n\n[agents.pi]\nenabled = false\n",
			wantVault: func(string) string {
				return "/data/vault"
			},
			check: func(t *testing.T, home string, cfg Config) {
				want := defaultAgents(home)
				want[0] = Agent{ID: "claude", Path: "/x/claude", Enabled: false}
				want[2].Enabled = false
				if !reflect.DeepEqual(cfg.Agents, want) {
					t.Errorf("agents = %+v, want %+v", cfg.Agents, want)
				}
			},
		},
		{
			name:    "agent order is built-in order regardless of file order",
			content: "[agents.agents]\nenabled = false\n[agents.claude]\nenabled = false\n",
			check: func(t *testing.T, home string, cfg Config) {
				ids := make([]string, 0, len(cfg.Agents))
				for _, a := range cfg.Agents {
					ids = append(ids, a.ID)
				}
				if got := strings.Join(ids, ","); got != "claude,cursor,pi,opencode,agents" {
					t.Errorf("order = %s", got)
				}
			},
		},
		{
			name:    "tilde expansion in vault and agent paths",
			content: "vault = \"~/notes/vault\"\n\n[agents.cursor]\npath = \"~/alt/cursor\"\n",
			wantVault: func(home string) string {
				return filepath.Join(home, "notes", "vault")
			},
			check: func(t *testing.T, home string, cfg Config) {
				if got, want := cfg.Agents[1].Path, filepath.Join(home, "alt", "cursor"); got != want {
					t.Errorf("cursor path = %s, want %s", got, want)
				}
			},
		},
		{
			name:    "explicit path is read and tilde-expanded",
			content: "vault = \"/v\"\n",
			path: func(home string) string {
				return filepath.Join("~", "custom.toml")
			},
			wantVault: func(string) string { return "/v" },
		},
		{
			name:    "unknown agent is an error",
			content: "[agents.windsurf]\npath = \"/w\"\n",
			wantErr: `unknown agent "windsurf"`,
		},
		{
			name:    "invalid toml is an error",
			content: "vault = [\n",
			wantErr: "parse config",
		},
		{
			name:    "wrong type is an error",
			content: "vault = 3\n",
			wantErr: "parse config",
		},
		{
			name:    "empty vault is an error",
			content: "vault = \"\"\n",
			wantErr: "vault must not be empty",
		},
		{
			name:    "empty agent path is an error",
			content: "[agents.claude]\npath = \"\"\n",
			wantErr: "agents.claude.path must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			path := ""
			if tt.path != nil {
				path = tt.path(home)
			}
			if !tt.noFile {
				cfgPath := filepath.Join(home, ".config", "sapato", "config.toml")
				if path != "" {
					cfgPath = filepath.Join(home, "custom.toml")
				}
				writeConfig(t, cfgPath, tt.content)
			}

			cfg, err := Load(path, home)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			wantVault := filepath.Join(home, "sapato-vault")
			if tt.wantVault != nil {
				wantVault = tt.wantVault(home)
			}
			if cfg.Vault != wantVault {
				t.Errorf("vault = %s, want %s", cfg.Vault, wantVault)
			}
			if tt.check != nil {
				tt.check(t, home, cfg)
			}
		})
	}
}
