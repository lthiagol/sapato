package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sapato/internal/config"
	"sapato/internal/tui/icons"
	"sapato/internal/tui/theme"
)

func TestConfigShowsUI(t *testing.T) {
	tests := []struct {
		name    string
		content string // config file; empty means the vault-only file from setup
		args    []string
		want    string // theme and icons lines, printed right after the vault line
	}{
		{name: "defaults", want: "theme: catppuccin-mocha\nicons: nerd"},
		{
			name:    "config file values",
			content: "[ui]\ntheme = \"dracula\"\nicons = \"unicode\"\n",
			want:    "theme: dracula\nicons: unicode",
		},
		{name: "flags override", args: []string{"--theme", "nord", "--icons", "ascii"}, want: "theme: nord\nicons: ascii"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfgPath, _ := setup(t)
			if tt.content != "" {
				if err := os.WriteFile(cfgPath, []byte(tt.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			args := append(append([]string{"--config", cfgPath}, tt.args...), "config")
			code, stdout, stderr := runCLI(args...)
			if code != 0 {
				t.Fatalf("exit code = %d, stderr %q", code, stderr)
			}
			if !strings.HasPrefix(stdout, "vault: ") {
				t.Fatalf("stdout = %q, want vault line first", stdout)
			}
			if !strings.Contains(stdout, tt.want+"\n\n") {
				t.Errorf("stdout = %q, want %q after the vault line", stdout, tt.want)
			}
		})
	}
}

func TestConfigInit(t *testing.T) {
	tests := []struct {
		name     string
		existing string   // config file written first; empty means none
		args     []string // after "config init"
		wantCode int
		wantErr  string
		wantFile bool   // whether the file must exist afterwards
		wantOut  string // stdout prefix when non-empty
	}{
		{name: "writes the starter", wantCode: 0, wantFile: true, wantOut: "wrote config "},
		{name: "refuses an existing file", existing: "vault = \"/keep\"\n", wantCode: 1, wantErr: "already exists", wantFile: true},
		{name: "force overwrites", existing: "vault = \"/keep\"\n", args: []string{"--force"}, wantCode: 0, wantFile: true, wantOut: "wrote config "},
		{name: "dry-run writes nothing", args: []string{"--dry-run"}, wantCode: 0, wantFile: false, wantOut: "# Sapato config."},
		{name: "extra args are usage error", args: []string{"x"}, wantCode: 2, wantErr: "unknown command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			path := filepath.Join(t.TempDir(), "nested", "config.toml")
			if tt.existing != "" {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(tt.existing), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			args := append([]string{"--config", path, "config", "init"}, tt.args...)
			code, stdout, stderr := runCLI(args...)
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr %q)", code, tt.wantCode, stderr)
			}
			if tt.wantErr != "" && !strings.Contains(stderr, tt.wantErr) {
				t.Errorf("stderr = %q, want containing %q", stderr, tt.wantErr)
			}
			if tt.wantOut != "" && !strings.HasPrefix(stdout, tt.wantOut) {
				t.Errorf("stdout = %q, want prefix %q", stdout, tt.wantOut)
			}
			_, statErr := os.Stat(path)
			if exists := statErr == nil; exists != tt.wantFile {
				t.Errorf("file exists = %t, want %t", exists, tt.wantFile)
			}
			if tt.existing != "" && tt.wantCode == 1 {
				if data, _ := os.ReadFile(path); string(data) != tt.existing {
					t.Errorf("refused init changed the file to %q", data)
				}
			}
		})
	}
}

func TestConfigInitWritesDefaultsAndNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(t.TempDir(), "config.toml")
	if code, _, stderr := runCLI("--config", path, "config", "init"); code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range append(theme.Names(), icons.Names()...) {
		if !strings.Contains(string(data), name) {
			t.Errorf("starter does not mention %q", name)
		}
	}
	got, err := config.Load(path, home)
	if err != nil {
		t.Fatalf("Load(written) error = %v", err)
	}
	want, err := config.Load(filepath.Join(t.TempDir(), "missing.toml"), home)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Agents, want.Agents) || got.Vault != want.Vault || got.UI != want.UI {
		t.Errorf("Load(written) = %+v, want defaults %+v", got, want)
	}
	if got.DefaultAgents {
		t.Error("written file has no [agents] table, want its own agent list")
	}
}

func TestConfigShowsAgents(t *testing.T) {
	tests := []struct {
		name    string
		content string // config file; empty means no file
		want    []string
		notes   []string // substrings the output must contain
	}{
		{
			name:  "no file lists the built-ins with a note",
			want:  []string{"claude", "cursor", "pi", "opencode", "agents"},
			notes: []string{"note: no config file at"},
		},
		{
			name:    "file without an agents table lists the built-ins with a note",
			content: "vault = \"/v\"\n",
			want:    []string{"claude", "cursor", "pi", "opencode", "agents"},
			notes:   []string{"has no [agents] table; built-in agents are in use"},
		},
		{
			name:    "agents table is listed in file order with its reads",
			content: "[agents]\nzeta = \"/z\"\ncursor = \"/c\"\nagents = { path = \"/a\", reads = [\"/r\"] }\n",
			want:    []string{"zeta", "cursor", "agents"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfgPath, _ := setup(t)
			if tt.content != "" {
				if err := os.WriteFile(cfgPath, []byte(tt.content), 0o644); err != nil {
					t.Fatal(err)
				}
			} else {
				cfgPath = filepath.Join(t.TempDir(), "absent.toml")
			}
			code, stdout, stderr := runCLI("--config", cfgPath, "config")
			if code != 0 {
				t.Fatalf("exit code = %d, stderr %q", code, stderr)
			}
			if strings.Contains(stdout, "ENABLED") {
				t.Errorf("stdout still has an ENABLED column:\n%s", stdout)
			}
			var ids []string
			for _, line := range strings.Split(stdout, "\n") {
				if f := strings.Fields(line); len(f) > 0 && f[0] != "AGENT" && f[0] != "vault:" && f[0] != "theme:" && f[0] != "icons:" && f[0] != "note:" {
					ids = append(ids, f[0])
				}
			}
			if !reflect.DeepEqual(ids, tt.want) {
				t.Errorf("agents = %v, want %v\n%s", ids, tt.want, stdout)
			}
			for _, note := range tt.notes {
				if !strings.Contains(stdout, note) {
					t.Errorf("stdout lacks note %q:\n%s", note, stdout)
				}
			}
			if len(tt.notes) == 0 && strings.Contains(stdout, "note:") {
				t.Errorf("stdout has a note but should not:\n%s", stdout)
			}
		})
	}
}

func TestConfigInitDefaultPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if code, _, stderr := runCLI("config", "init"); code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "sapato", "config.toml")); err != nil {
		t.Errorf("default config not written: %v", err)
	}
}

func TestConfigInitDryRunPrintsValidTOML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(t.TempDir(), "config.toml")
	code, stdout, stderr := runCLI("--config", path, "--dry-run", "config", "init")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}
	printed := filepath.Join(t.TempDir(), "printed.toml")
	if err := os.WriteFile(printed, []byte(stdout), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(printed, home)
	if err != nil {
		t.Fatalf("dry-run output is not a valid config: %v", err)
	}
	if got.UI.Theme != "catppuccin-mocha" || got.UI.Icons != "nerd" {
		t.Errorf("ui = %+v, want defaults", got.UI)
	}
}

func TestConfigDefaultPrintsWithoutWriting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(t.TempDir(), "config.toml")
	code, stdout, stderr := runCLI("--config", path, "config", "default")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}
	if stdout != config.DefaultText() {
		t.Errorf("stdout differs from the default text:\n%s", stdout)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("config default wrote %s (stat error %v)", path, err)
	}
}

func TestConfigInitWritesSameBytesAsDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, defaultOut, _ := runCLI("config", "default")
	path := filepath.Join(t.TempDir(), "config.toml")
	if code, _, stderr := runCLI("--config", path, "config", "init"); code != 0 {
		t.Fatalf("config init exit code = %d, stderr %q", code, stderr)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != defaultOut {
		t.Errorf("config init wrote %q, want the bytes of config default", written)
	}
	_, dryOut, _ := runCLI("--dry-run", "config", "init")
	if dryOut != defaultOut {
		t.Errorf("--dry-run printed %q, want the bytes of config default", dryOut)
	}
}
