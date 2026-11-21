package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCommand(t *testing.T) {
	cfgPath, claudeDir := statusSetup(t)

	t.Run("dry-run prints the plan and changes nothing", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "--dry-run", "install", "mocked-skill-00004", "-a", "claude")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.Contains(out, "would install mocked-skill-00004 in claude") {
			t.Errorf("output = %q", out)
		}
		if _, err := os.Stat(filepath.Join(claudeDir, "mocked-skill-00004")); !os.IsNotExist(err) {
			t.Errorf("dry run installed the skill (err %v)", err)
		}
	})

	t.Run("install with --all copies into configured agents", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "install", "mocked-skill-00004", "--all")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.Contains(out, "install mocked-skill-00004 in claude") {
			t.Errorf("output = %q", out)
		}
		if _, err := os.Stat(filepath.Join(claudeDir, "mocked-skill-00004", "SKILL.md")); err != nil {
			t.Errorf("skill not installed: %v", err)
		}
	})

	t.Run("refused install exits 1 and names --force", func(t *testing.T) {
		code, _, stderr := runCLI("--config", cfgPath, "install", "mocked-skill-00003", "-a", "claude")
		if code != 1 || !strings.Contains(stderr, ": modified") || !strings.Contains(stderr, "--force") {
			t.Errorf("exit = %d, stderr %q; want 1 with modified and --force", code, stderr)
		}
	})

	t.Run("--force overwrites a modified copy", func(t *testing.T) {
		code, _, stderr := runCLI("--config", cfgPath, "install", "mocked-skill-00003", "-a", "claude", "--force")
		if code != 0 {
			t.Errorf("exit = %d, stderr %q", code, stderr)
		}
	})

	t.Run("glob skips invalid skills and prints the skip", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "install", "invalid-no*", "-a", "claude")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.Contains(out, "skip invalid-no-frontmatter in claude: invalid") {
			t.Errorf("output = %q", out)
		}
	})
}

func TestInstallUsageErrors(t *testing.T) {
	cfgPath, _ := statusSetup(t)
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no agent flag", args: []string{"install", "mocked-skill-00004"}, wantErr: "choose agents"},
		{name: "agent and all", args: []string{"install", "mocked-skill-00004", "-a", "claude", "--all"}, wantErr: "not both"},
		{name: "no skill", args: []string{"install", "-a", "claude"}, wantErr: "requires at least 1 arg"},
		{name: "unknown agent", args: []string{"install", "mocked-skill-00004", "-a", "windsurf"}, wantErr: "unknown agent"},
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

func TestUninstallCommand(t *testing.T) {
	cfgPath, claudeDir := statusSetup(t)

	t.Run("refuses unmanaged even with --force", func(t *testing.T) {
		code, _, stderr := runCLI("--config", cfgPath, "uninstall", "hand-made-skill", "-a", "claude", "--force")
		if code != 1 || !strings.Contains(stderr, ": unmanaged") {
			t.Errorf("exit = %d, stderr %q; want 1 with unmanaged", code, stderr)
		}
		if _, err := os.Stat(filepath.Join(claudeDir, "hand-made-skill", "SKILL.md")); err != nil {
			t.Errorf("unmanaged folder was removed: %v", err)
		}
	})

	t.Run("glob skips unmanaged and prints the skip", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "uninstall", "h*", "-a", "claude", "--force")
		if code != 0 || !strings.Contains(out, "skip hand-made-skill in claude: unmanaged") {
			t.Errorf("exit = %d, out %q, stderr %q; want 0 with the skip line", code, out, stderr)
		}
		if _, err := os.Stat(filepath.Join(claudeDir, "hand-made-skill", "SKILL.md")); err != nil {
			t.Errorf("unmanaged folder was removed: %v", err)
		}
	})

	t.Run("removes a synced copy", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "uninstall", "mocked-skill-00001", "-a", "claude")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.Contains(out, "uninstall mocked-skill-00001 in claude") {
			t.Errorf("output = %q", out)
		}
		if _, err := os.Stat(filepath.Join(claudeDir, "mocked-skill-00001")); !os.IsNotExist(err) {
			t.Errorf("folder still there (err %v)", err)
		}
	})

	t.Run("skill not installed reports nothing to do", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "uninstall", "mocked-skill-00004", "-a", "claude")
		if code != 0 || strings.TrimSpace(out) != "nothing to do" {
			t.Errorf("exit = %d, out %q, stderr %q", code, out, stderr)
		}
	})
}
