package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sapato/internal/vault"
)

// compatSetup isolates HOME, creates an empty vault with one skill, and returns
// the config path and the vault dir.
func compatSetup(t *testing.T) (cfgPath, vaultDir string) {
	t.Helper()
	cfgPath, vaultDir = setup(t)
	if err := vault.Init(vaultDir); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, vaultDir, "code-review", "---\nname: code-review\ndescription: Reviews code.\n---\n")
	return cfgPath, vaultDir
}

func TestCompatCommand(t *testing.T) {
	cfgPath, vaultDir := compatSetup(t)
	manifest := filepath.Join(vaultDir, "sapato.toml")
	read := func() string {
		t.Helper()
		data, err := os.ReadFile(manifest)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
		check    func(t *testing.T)
	}{
		{
			name:    "no agents prints all agents",
			args:    []string{"compat", "code-review"},
			wantOut: "code-review: all agents",
		},
		{
			name:    "agents set the list",
			args:    []string{"compat", "code-review", "claude", "cursor"},
			wantOut: "compat code-review: claude, cursor",
			check: func(t *testing.T) {
				if !strings.Contains(read(), "[skills.code-review]\ncompatible = [\"claude\", \"cursor\"]\n") {
					t.Errorf("sapato.toml = %q", read())
				}
			},
		},
		{
			name:    "list is printed after it is set",
			args:    []string{"compat", "code-review"},
			wantOut: "code-review: claude, cursor",
		},
		{
			name:    "dry run prints the change and writes nothing",
			args:    []string{"--dry-run", "compat", "code-review", "agents"},
			wantOut: "would compat code-review: agents",
			check: func(t *testing.T) {
				if strings.Contains(read(), "agents") {
					t.Errorf("dry run changed sapato.toml: %q", read())
				}
			},
		},
		{
			name:    "--all removes the table",
			args:    []string{"compat", "code-review", "--all"},
			wantOut: "compat code-review: all agents",
			check: func(t *testing.T) {
				if strings.Contains(read(), "skills.code-review") {
					t.Errorf("sapato.toml still has the table: %q", read())
				}
			},
		},
		{
			name:     "agents and --all together are a usage error",
			args:     []string{"compat", "code-review", "claude", "--all"},
			wantCode: 2,
			wantErr:  "use either agents or --all, not both",
		},
		{
			name:     "missing skill argument is a usage error",
			args:     []string{"compat"},
			wantCode: 2,
		},
		{
			name:     "unknown skill is an error",
			args:     []string{"compat", "no-such-skill", "claude"},
			wantCode: 1,
			wantErr:  "no skill in vault",
		},
		{
			name:     "bad agent name is an error",
			args:     []string{"compat", "code-review", "Claude"},
			wantCode: 1,
			wantErr:  "agent name \"Claude\" is invalid",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantCode := tt.wantCode
			code, out, stderr := runCLI(append([]string{"--config", cfgPath}, tt.args...)...)
			if code != wantCode {
				t.Fatalf("exit = %d, want %d (stderr %q)", code, wantCode, stderr)
			}
			if tt.wantOut != "" && strings.TrimSpace(out) != tt.wantOut {
				t.Errorf("stdout = %q, want %q", out, tt.wantOut)
			}
			if tt.wantErr != "" && !strings.Contains(stderr, tt.wantErr) {
				t.Errorf("stderr = %q, want containing %q", stderr, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t)
			}
		})
	}
}

func TestInstallAllSkipsIncompatible(t *testing.T) {
	cfgPath, _ := compatSetup(t)
	if code, _, stderr := runCLI("--config", cfgPath, "compat", "code-review", "claude"); code != 0 {
		t.Fatalf("compat exit = %d, stderr %q", code, stderr)
	}

	code, out, stderr := runCLI("--config", cfgPath, "install", "code-review", "--all")
	if code != 0 {
		t.Fatalf("install --all exit = %d, stderr %q", code, stderr)
	}
	if !strings.Contains(out, "install code-review in claude") {
		t.Errorf("output = %q, want the install in claude", out)
	}
	if !strings.Contains(out, "skip code-review in cursor: incompatible") {
		t.Errorf("output = %q, want the incompatible skip in cursor", out)
	}

	code, _, stderr = runCLI("--config", cfgPath, "install", "code-review", "-a", "cursor")
	if code != 1 || !strings.Contains(stderr, "refusing to install code-review in cursor: incompatible (use --force)") {
		t.Errorf("explicit install in cursor: exit %d, stderr %q", code, stderr)
	}
}

func TestStatusJSONCompatible(t *testing.T) {
	cfgPath, _ := compatSetup(t)
	if code, _, stderr := runCLI("--config", cfgPath, "compat", "code-review", "claude"); code != 0 {
		t.Fatalf("compat exit = %d, stderr %q", code, stderr)
	}

	code, out, stderr := runCLI("--config", cfgPath, "status", "--json", "code-review")
	if code != 0 {
		t.Fatalf("status exit = %d, stderr %q", code, stderr)
	}
	for _, want := range []string{
		`"agent": "claude",` + "\n" + `    "status": "not-installed",` + "\n" + `    "compatible": true`,
		`"agent": "cursor",` + "\n" + `    "status": "incompatible",` + "\n" + `    "compatible": false`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status --json lacks %q in:\n%s", want, out)
		}
	}
}
