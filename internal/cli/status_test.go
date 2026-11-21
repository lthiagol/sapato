package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sapato/internal/testfixtures"
)

// statusSetup points the vault and the claude target at copies of the fixtures
// and returns the config path. HOME is isolated, so other agents see nothing.
func statusSetup(t *testing.T) (cfgPath, claudeDir string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	vaultDir := testfixtures.CopyTree(t, filepath.Join(testfixtures.Root(), "skill-db"))
	claudeDir = testfixtures.CopyTree(t, filepath.Join(testfixtures.Root(), "targets", "claude"))
	cfgPath = filepath.Join(t.TempDir(), "config.toml")
	content := "vault = \"" + vaultDir + "\"\n\n[agents]\nclaude = \"" + claudeDir + "\"\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfgPath, claudeDir
}

func TestStatusJSON(t *testing.T) {
	cfgPath, _ := statusSetup(t)
	code, out, stderr := runCLI("--config", cfgPath, "status", "-a", "claude", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q", code, stderr)
	}
	var rows []struct {
		Skill  string `json:"skill"`
		Agent  string `json:"agent"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if len(rows) != 15 {
		t.Errorf("got %d rows, want 15 for claude", len(rows))
	}
	got := make(map[string]string, len(rows))
	for _, r := range rows {
		got[r.Skill] = r.Status
	}
	want := map[string]string{
		"mocked-skill-00001":    "synced",
		"mocked-skill-00002":    "outdated",
		"mocked-skill-00003":    "modified",
		"mocked-skill-00098":    "missing",
		"mocked-skill-00099":    "orphaned",
		"hand-made-skill":       "unmanaged",
		"mocked-skill-00004":    "not-installed",
		"invalid-name-mismatch": "invalid",
	}
	for skill, status := range want {
		if got[skill] != status {
			t.Errorf("%s = %q, want %q", skill, got[skill], status)
		}
	}
}

func TestStatusJSONEmpty(t *testing.T) {
	cfgPath, _ := statusSetup(t)
	code, out, stderr := runCLI("--config", cfgPath, "status", "-a", "claude", "no-such-skill", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q", code, stderr)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("output = %q, want []", out)
	}
}

func TestStatusTable(t *testing.T) {
	cfgPath, _ := statusSetup(t)
	code, out, stderr := runCLI("--config", cfgPath, "status", "-a", "claude", "mocked-skill-0000[1-3]")
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q", code, stderr)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want header + 3 skills:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "SKILL") || !strings.Contains(lines[0], "CLAUDE") {
		t.Errorf("header = %q", lines[0])
	}
	for i, status := range []string{"synced", "outdated", "modified"} {
		if !strings.Contains(lines[i+1], status) {
			t.Errorf("row %d = %q, want %s", i, lines[i+1], status)
		}
	}
}

func TestStatusDefaultAgentsAreConfigured(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	vaultDir := testfixtures.CopyTree(t, filepath.Join(testfixtures.Root(), "skill-db"))
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte("vault = \""+vaultDir+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := runCLI("--config", cfgPath, "status", "mocked-skill-00001")
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q", code, stderr)
	}
	header := strings.Split(out, "\n")[0]
	for _, col := range []string{"CLAUDE", "CURSOR", "PI", "OPENCODE", "AGENTS"} {
		if !strings.Contains(header, col) {
			t.Errorf("header %q missing column %s", header, col)
		}
	}
}

func TestStatusErrors(t *testing.T) {
	cfgPath, claudeDir := statusSetup(t)

	tests := []struct {
		name     string
		args     []string
		prep     func(t *testing.T)
		wantCode int
		wantErr  string
	}{
		{
			name:     "unknown agent is usage error",
			args:     []string{"--config", cfgPath, "status", "-a", "windsurf"},
			wantCode: 2,
			wantErr:  `unknown agent "windsurf"`,
		},
		{
			name:     "bad glob is runtime error",
			args:     []string{"--config", cfgPath, "status", "["},
			wantCode: 1,
			wantErr:  "bad skill pattern",
		},
		{
			name: "corrupt lock names the file",
			args: []string{"--config", cfgPath, "status", "-a", "claude"},
			prep: func(t *testing.T) {
				if err := os.WriteFile(filepath.Join(claudeDir, ".sapato-lock.json"), []byte("{"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantCode: 1,
			wantErr:  filepath.Join(claudeDir, ".sapato-lock.json"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.prep != nil {
				tt.prep(t)
			}
			code, _, stderr := runCLI(tt.args...)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stderr %q)", code, tt.wantCode, stderr)
			}
			if !strings.Contains(stderr, tt.wantErr) {
				t.Errorf("stderr = %q, want containing %q", stderr, tt.wantErr)
			}
		})
	}
}
