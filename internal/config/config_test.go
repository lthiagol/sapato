package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

// wantDefaults is the built-in agent list under home, in spec order.
func wantDefaults(home string) []Agent {
	claudeDir := filepath.Join(home, ".claude", "skills")
	agentsDir := filepath.Join(home, ".agents", "skills")
	return []Agent{
		{ID: "claude", Path: claudeDir},
		{ID: "cursor", Path: filepath.Join(home, ".cursor", "skills"), ReadDirs: []string{claudeDir}},
		{ID: "pi", Path: filepath.Join(home, ".pi", "agent", "skills"), ReadDirs: []string{agentsDir}},
		{ID: "opencode", Path: filepath.Join(home, ".config", "opencode", "skills"), ReadDirs: []string{claudeDir, agentsDir}},
		{ID: "agents", Path: agentsDir},
	}
}

func agentIDs(agents []Agent) string {
	ids := make([]string, 0, len(agents))
	for _, a := range agents {
		ids = append(ids, a.ID)
	}
	return strings.Join(ids, ",")
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		content string // written to the config path; empty means no file
		noFile  bool
		path    func(home string) string // explicit path arg; nil means ""
		wantErr string
		check   func(t *testing.T, home string, cfg Config)
	}{
		{
			name:   "missing file gives defaults",
			noFile: true,
			check: func(t *testing.T, home string, cfg Config) {
				if !reflect.DeepEqual(cfg.Agents, wantDefaults(home)) {
					t.Errorf("agents = %+v, want %+v", cfg.Agents, wantDefaults(home))
				}
				if cfg.FileExists || !cfg.DefaultAgents {
					t.Errorf("FileExists = %v, DefaultAgents = %v, want false, true", cfg.FileExists, cfg.DefaultAgents)
				}
				if want := filepath.Join(home, ".config", "sapato", "config.toml"); cfg.Path != want {
					t.Errorf("Path = %s, want %s", cfg.Path, want)
				}
			},
		},
		{
			name:    "empty file gives defaults but exists",
			content: "",
			check: func(t *testing.T, home string, cfg Config) {
				if !reflect.DeepEqual(cfg.Agents, wantDefaults(home)) {
					t.Errorf("agents = %+v, want defaults", cfg.Agents)
				}
				if !cfg.FileExists || !cfg.DefaultAgents {
					t.Errorf("FileExists = %v, DefaultAgents = %v, want true, true", cfg.FileExists, cfg.DefaultAgents)
				}
			},
		},
		{
			name:    "vault is read and tilde-expanded",
			content: "vault = \"~/notes/vault\"\n",
			check: func(t *testing.T, home string, cfg Config) {
				if want := filepath.Join(home, "notes", "vault"); cfg.Vault != want {
					t.Errorf("vault = %s, want %s", cfg.Vault, want)
				}
			},
		},
		{
			name:    "explicit agents table is authoritative",
			content: "[agents]\nclaude = \"/x/claude\"\n",
			check: func(t *testing.T, _ string, cfg Config) {
				want := []Agent{{ID: "claude", Path: "/x/claude"}}
				if !reflect.DeepEqual(cfg.Agents, want) {
					t.Errorf("agents = %+v, want %+v", cfg.Agents, want)
				}
				if cfg.DefaultAgents {
					t.Error("DefaultAgents = true with an [agents] table")
				}
			},
		},
		{
			name:    "empty agents table means no agents",
			content: "[agents]\n",
			check: func(t *testing.T, _ string, cfg Config) {
				if len(cfg.Agents) != 0 {
					t.Errorf("agents = %+v, want none", cfg.Agents)
				}
				if cfg.DefaultAgents {
					t.Error("DefaultAgents = true with an [agents] table")
				}
			},
		},
		{
			name:    "file order is kept, not alphabetical",
			content: "[agents]\nzeta = \"/z\"\nclaude = \"/c\"\nalpha = \"/a\"\n",
			check: func(t *testing.T, _ string, cfg Config) {
				if got := agentIDs(cfg.Agents); got != "zeta,claude,alpha" {
					t.Errorf("order = %s, want zeta,claude,alpha", got)
				}
			},
		},
		{
			name:    "dotted keys at the top level keep their order",
			content: "agents.zeta = \"/z\"\nagents.claude = \"/c\"\n",
			check: func(t *testing.T, _ string, cfg Config) {
				if got := agentIDs(cfg.Agents); got != "zeta,claude" {
					t.Errorf("order = %s, want zeta,claude", got)
				}
			},
		},
		{
			name:    "inline agents table keeps its order",
			content: "agents = { zeta = \"/z\", claude = \"/c\" }\n",
			check: func(t *testing.T, _ string, cfg Config) {
				if got := agentIDs(cfg.Agents); got != "zeta,claude" {
					t.Errorf("order = %s, want zeta,claude", got)
				}
			},
		},
		{
			name:    "plain path under a built-in name inherits its read dirs",
			content: "[agents]\ncursor = \"/c\"\n",
			check: func(t *testing.T, home string, cfg Config) {
				if got, want := cfg.Agents[0].ReadDirs, []string{filepath.Join(home, ".claude", "skills")}; !reflect.DeepEqual(got, want) {
					t.Errorf("cursor read dirs = %v, want %v", got, want)
				}
			},
		},
		{
			name:    "inline table reads replace the built-in read dirs",
			content: "[agents]\nopencode = { path = \"/o\", reads = [\"~/x\"] }\n",
			check: func(t *testing.T, home string, cfg Config) {
				if got, want := cfg.Agents[0].ReadDirs, []string{filepath.Join(home, "x")}; !reflect.DeepEqual(got, want) {
					t.Errorf("opencode read dirs = %v, want %v", got, want)
				}
			},
		},
		{
			name:    "inline table without reads has none, even under a built-in name",
			content: "[agents]\ncursor = { path = \"/c\" }\n",
			check: func(t *testing.T, _ string, cfg Config) {
				if got := cfg.Agents[0].ReadDirs; got != nil {
					t.Errorf("cursor read dirs = %v, want none", got)
				}
			},
		},
		{
			name:    "custom agent with inline reads",
			content: "[agents]\nwork = { path = \"/w\", reads = [\"/r1\", \"~/r2\"] }\nclaude = \"/c\"\n",
			check: func(t *testing.T, home string, cfg Config) {
				want := []Agent{
					{ID: "work", Path: "/w", ReadDirs: []string{"/r1", filepath.Join(home, "r2")}},
					{ID: "claude", Path: "/c"},
				}
				if !reflect.DeepEqual(cfg.Agents, want) {
					t.Errorf("agents = %+v, want %+v", cfg.Agents, want)
				}
			},
		},
		{
			name:    "tilde expansion in agent paths",
			content: "[agents]\ncursor = \"~/alt/cursor\"\n",
			check: func(t *testing.T, home string, cfg Config) {
				if got, want := cfg.Agents[0].Path, filepath.Join(home, "alt", "cursor"); got != want {
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
			check: func(t *testing.T, home string, cfg Config) {
				if want := filepath.Join(home, "custom.toml"); cfg.Path != want || !cfg.FileExists {
					t.Errorf("Path = %s, FileExists = %v, want %s, true", cfg.Path, cfg.FileExists, want)
				}
			},
		},
		{
			name:    "nested dotted key under agents is an error",
			content: "[agents]\nwork.path = \"/w\"\n",
			wantErr: `agents.work: use work = "path" directly under [agents]`,
		},
		{
			name:    "nested dotted key at the top level is an error",
			content: "agents.work.path = \"/w\"\n",
			wantErr: "agents.work: use work",
		},
		{
			name:    "agents key inside another table is not the agents table",
			content: "[ui]\nagents = { x = \"/p\" }\n",
			check: func(t *testing.T, home string, cfg Config) {
				if !reflect.DeepEqual(cfg.Agents, wantDefaults(home)) || !cfg.DefaultAgents {
					t.Errorf("agents = %+v, DefaultAgents = %v; want the built-in defaults", cfg.Agents, cfg.DefaultAgents)
				}
			},
		},
		{
			name:    "dotted agents key inside another table is not the agents table",
			content: "[ui]\nagents.x = \"/p\"\n",
			check: func(t *testing.T, home string, cfg Config) {
				if !reflect.DeepEqual(cfg.Agents, wantDefaults(home)) || !cfg.DefaultAgents {
					t.Errorf("agents = %+v, DefaultAgents = %v; want the built-in defaults", cfg.Agents, cfg.DefaultAgents)
				}
			},
		},
		{
			name:    "old table form is rejected with the new form",
			content: "[agents.claude]\npath = \"/x/claude\"\n",
			wantErr: "the [agents.claude] table form is replaced by claude = \"path\"",
		},
		{
			name:    "old table form with enabled is rejected too",
			content: "[agents.pi]\nenabled = false\n",
			wantErr: "the [agents.pi] table form is replaced",
		},
		{
			name:    "bad agent name is an error",
			content: "[agents]\nClaude = \"/c\"\n",
			wantErr: `agent name "Claude" must match`,
		},
		{
			name:    "agent name longer than 32 characters is an error",
			content: "[agents]\n" + strings.Repeat("a", 33) + " = \"/c\"\n",
			wantErr: "must match",
		},
		{
			name:    "duplicate path is an error",
			content: "[agents]\ncursor = \"/same\"\npi = \"/same/\"\n",
			wantErr: "cursor and pi both use /same",
		},
		{
			name:    "duplicate path after tilde expansion is an error",
			content: "[agents]\nclaude = \"~/x\"\nagents = \"~/x/\"\n",
			wantErr: "claude and agents both use",
		},
		{
			name:    "unknown inline key is an error",
			content: "[agents]\nwork = { path = \"/w\", enabled = false }\n",
			wantErr: `agents.work: unknown key "enabled"`,
		},
		{
			name:    "inline table without path is an error",
			content: "[agents]\nwork = { reads = [] }\n",
			wantErr: "agents.work: path must be a non-empty string",
		},
		{
			name:    "reads must be a list of paths",
			content: "[agents]\nwork = { path = \"/w\", reads = \"/r\" }\n",
			wantErr: "agents.work: reads must be a list of paths",
		},
		{
			name:    "value must be a path or an inline table",
			content: "[agents]\nwork = 3\n",
			wantErr: "agents.work: must be a path string or an inline table",
		},
		{
			name:    "empty agent path is an error",
			content: "[agents]\nclaude = \"\"\n",
			wantErr: "agents.claude: path must not be empty",
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
			name:    "ui defaults",
			content: "",
			check: func(t *testing.T, _ string, cfg Config) {
				if want := (UI{Theme: "catppuccin-mocha", Icons: "nerd"}); cfg.UI != want {
					t.Errorf("ui = %+v, want %+v", cfg.UI, want)
				}
			},
		},
		{
			name:    "ui overrides",
			content: "[ui]\ntheme = \"nord-light\"\nicons = \"ascii\"\n",
			check: func(t *testing.T, _ string, cfg Config) {
				if want := (UI{Theme: "nord-light", Icons: "ascii"}); cfg.UI != want {
					t.Errorf("ui = %+v, want %+v", cfg.UI, want)
				}
			},
		},
		{
			name:    "ui theme alone keeps the default icon set",
			content: "[ui]\ntheme = \"dracula\"\n",
			check: func(t *testing.T, _ string, cfg Config) {
				if want := (UI{Theme: "dracula", Icons: "nerd"}); cfg.UI != want {
					t.Errorf("ui = %+v, want %+v", cfg.UI, want)
				}
			},
		},
		{
			name:    "unknown theme is an error listing the valid names",
			content: "[ui]\ntheme = \"solarized\"\n",
			wantErr: `ui.theme: unknown theme "solarized" (valid: catppuccin-latte, catppuccin-frappe, catppuccin-macchiato, catppuccin-mocha, dracula, dracula-alucard, nord, nord-light)`,
		},
		{
			name:    "unknown icon set is an error listing the valid names",
			content: "[ui]\nicons = \"emoji\"\n",
			wantErr: `ui.icons: unknown icon set "emoji" (valid: nerd, unicode, ascii)`,
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
			if tt.check != nil {
				tt.check(t, home, cfg)
			}
		})
	}
}

func TestLoadDefaultVault(t *testing.T) {
	home := t.TempDir()
	cfg, err := Load("", home)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "sapato-vault"); cfg.Vault != want {
		t.Errorf("vault = %s, want %s", cfg.Vault, want)
	}
}

func TestPath(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "home", "u")
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "empty is the default file", path: "", want: filepath.Join(home, ".config", "sapato", "config.toml")},
		{name: "absolute path is kept", path: "/etc/sapato.toml", want: "/etc/sapato.toml"},
		{name: "tilde is expanded", path: "~/alt.toml", want: filepath.Join(home, "alt.toml")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Path(tt.path, home); got != tt.want {
				t.Errorf("Path(%q) = %s, want %s", tt.path, got, tt.want)
			}
		})
	}
}

// defaultTextSpec is spec §2.1 byte for byte. A raw string cannot hold a backtick,
// so the spec's backticks are ^ here.
var defaultTextSpec = `# Sapato config. Generated by ^sapato config default^.

# Folder holding your skill vault (sapato.toml + skills/). ^~^ is expanded.
# Default: ~/sapato-vault
vault = "~/sapato-vault"

# The skill folders, per harness, that sapato manages.
# Order here = order in the UI. Comment a line out to stop managing it.
# No [agents] table = the built-in defaults below; an empty table = no agents.
# Names: lowercase letters, digits and -, starting with a letter.
# Example:
#   name = "path"
#   name = { path = "path", reads = ["other dir this harness also reads"] }
# ^reads^ is only used by ^sapato doctor^ to spot skills visible twice.
[agents]
claude   = "~/.claude/skills"
cursor   = "~/.cursor/skills"            # also reads ~/.claude/skills
pi       = "~/.pi/agent/skills"          # also reads ~/.agents/skills
opencode = "~/.config/opencode/skills"   # also reads ~/.claude/skills, ~/.agents/skills
agents   = "~/.agents/skills"
# codex  = "~/.codex/skills"
# work   = { path = "~/work/.claude/skills", reads = ["~/.agents/skills"] }

[ui]
# Icon set:
#   - nerd     Nerd Font glyphs (needs a Nerd Font in your terminal)
#   - unicode  plain Unicode symbols
#   - ascii    letters only
# Default: nerd
icons = "nerd"

# Color theme:
#   - catppuccin-latte (light)
#   - catppuccin-frappe
#   - catppuccin-macchiato
#   - catppuccin-mocha (default)
#   - dracula
#   - dracula-alucard (light)
#   - nord
#   - nord-light (light)
theme = "catppuccin-mocha"

# Skill assignments and per-skill compatibility live in the vault's sapato.toml.
`

func TestDefaultTextMatchesSpec(t *testing.T) {
	want := strings.ReplaceAll(defaultTextSpec, "^", "`")
	if got := DefaultText(); got != want {
		t.Errorf("DefaultText() differs from spec §2.1:\n%s", got)
	}
}

func TestDefaultTextListMarks(t *testing.T) {
	text := DefaultText()
	tests := []struct {
		name      string
		heading   string   // comment line before the list
		want      []string // list lines, marks included
		wantNames []string // the names the registry gives, in list order
	}{
		{
			name:    "themes with light and default marks",
			heading: "# Color theme:\n",
			want: []string{
				"#   - catppuccin-latte (light)",
				"#   - catppuccin-frappe",
				"#   - catppuccin-macchiato",
				"#   - catppuccin-mocha (default)",
				"#   - dracula",
				"#   - dracula-alucard (light)",
				"#   - nord",
				"#   - nord-light (light)",
			},
			wantNames: ThemeNames(),
		},
		{
			name:    "icon sets with descriptions",
			heading: "# Icon set:\n",
			want: []string{
				"#   - nerd     Nerd Font glyphs (needs a Nerd Font in your terminal)",
				"#   - unicode  plain Unicode symbols",
				"#   - ascii    letters only",
			},
			wantNames: IconNames(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := strings.Index(text, tt.heading)
			if start < 0 {
				t.Fatalf("text lacks %q", tt.heading)
			}
			var got []string
			for _, line := range strings.Split(text[start+len(tt.heading):], "\n") {
				if !strings.HasPrefix(line, "#   - ") {
					break
				}
				got = append(got, line)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("list = %q, want %q", got, tt.want)
			}
			if !slices.Equal(tt.wantNames, namesOf(got)) {
				t.Errorf("registered names = %v, want the list's names %v", tt.wantNames, namesOf(got))
			}
		})
	}
}

// namesOf returns the names of list lines like "#   - name (light)".
func namesOf(lines []string) []string {
	var names []string
	for _, line := range lines {
		f := strings.Fields(strings.TrimPrefix(line, "#   - "))
		if len(f) > 0 {
			names = append(names, f[0])
		}
	}
	return names
}

func TestDefaultTextLoadsToDefaults(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := WriteDefault(path, false); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path, home)
	if err != nil {
		t.Fatalf("Load(default) error = %v", err)
	}
	want, err := Load(filepath.Join(t.TempDir(), "missing.toml"), home)
	if err != nil {
		t.Fatalf("Load(missing) error = %v", err)
	}
	if got.Vault != filepath.Join(home, "sapato-vault") {
		t.Errorf("vault = %s, want the default", got.Vault)
	}
	if !reflect.DeepEqual(got.Agents, want.Agents) {
		t.Errorf("agents = %+v, want defaults %+v", got.Agents, want.Agents)
	}
	if got.UI != want.UI || got.Vault != want.Vault {
		t.Errorf("ui/vault = %+v/%s, want %+v/%s", got.UI, got.Vault, want.UI, want.Vault)
	}
	if got.DefaultAgents {
		t.Error("default text has an [agents] table, DefaultAgents = true")
	}
}

func TestWriteDefault(t *testing.T) {
	tests := []struct {
		name     string
		existing string // written first; empty means no file
		force    bool
		wantErr  string
		want     string // content after the call when no error; "" means DefaultText
	}{
		{name: "creates parent dirs and the file"},
		{name: "refuses an existing file", existing: "old\n", wantErr: "already exists", want: "old\n"},
		{name: "force overwrites", existing: "old\n", force: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "nested", "dir", "config.toml")
			if tt.existing != "" {
				writeConfig(t, path, tt.existing)
			}
			err := WriteDefault(path, tt.force)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("WriteDefault() error = %v, want containing %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("WriteDefault() error = %v", err)
			}
			want := tt.want
			if want == "" {
				want = DefaultText()
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(data) != want {
				t.Errorf("content = %q, want %q", data, want)
			}
		})
	}
}
