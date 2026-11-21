package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateCommand(t *testing.T) {
	cfgPath, claudeDir := statusSetup(t)
	outdated := filepath.Join(claudeDir, "mocked-skill-00002", "SKILL.md")

	t.Run("dry-run prints the plan and changes nothing", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "--dry-run", "update")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.Contains(out, "would update mocked-skill-00002 in claude") || !strings.Contains(out, "skip mocked-skill-00003 in claude: modified") {
			t.Errorf("output = %q", out)
		}
		data, err := os.ReadFile(outdated)
		if err != nil || !strings.Contains(string(data), `"1.1.0"`) {
			t.Errorf("dry run changed the copy (err %v)", err)
		}
	})

	t.Run("updates outdated and skips modified", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "update")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.Contains(out, "update mocked-skill-00002 in claude") || !strings.Contains(out, "skip mocked-skill-00003 in claude: modified") {
			t.Errorf("output = %q", out)
		}
		data, err := os.ReadFile(outdated)
		if err != nil || !strings.Contains(string(data), `"1.2.0"`) {
			t.Errorf("copy not updated (err %v)", err)
		}
	})

	t.Run("--force re-installs modified", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "update", "-a", "claude", "--force")
		if code != 0 || !strings.Contains(out, "update mocked-skill-00003 in claude") {
			t.Errorf("exit = %d, out %q, stderr %q", code, out, stderr)
		}
	})

	t.Run("nothing outdated reports nothing to do", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "update")
		if code != 0 || strings.TrimSpace(out) != "nothing to do" {
			t.Errorf("exit = %d, out %q, stderr %q", code, out, stderr)
		}
	})
}

func TestUpdateUsageErrors(t *testing.T) {
	cfgPath, _ := statusSetup(t)
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "agent and all", args: []string{"update", "-a", "claude", "--all"}, wantErr: "not both"},
		{name: "unknown agent", args: []string{"update", "-a", "windsurf"}, wantErr: "unknown agent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, stderr := runCLI(append([]string{"--config", cfgPath}, tt.args...)...)
			if code != 2 || !strings.Contains(stderr, tt.wantErr) {
				t.Errorf("exit = %d, stderr %q; want 2 containing %q", code, stderr, tt.wantErr)
			}
		})
	}
}
