package cli

import (
	"strings"
	"testing"
)

func TestDiffCommand(t *testing.T) {
	cfgPath, _ := statusSetup(t)

	t.Run("prints the file list and the unified diff", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "diff", "mocked-skill-00002", "-a", "claude")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		for _, want := range []string{"changed SKILL.md\n", "--- installed/SKILL.md\n", "+++ vault/SKILL.md\n", "-  version: \"1.1.0\"\n", "+  version: \"1.2.0\"\n"} {
			if !strings.Contains(out, want) {
				t.Errorf("output missing %q:\n%s", want, out)
			}
		}
	})

	t.Run("synced copy has no differences", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "diff", "mocked-skill-00001", "-a", "claude")
		if code != 0 || strings.TrimSpace(out) != "no differences" {
			t.Errorf("exit = %d, out %q, stderr %q", code, out, stderr)
		}
	})

	t.Run("not installed exits 1", func(t *testing.T) {
		code, _, stderr := runCLI("--config", cfgPath, "diff", "mocked-skill-00004", "-a", "claude")
		if code != 1 || !strings.Contains(stderr, "not installed") {
			t.Errorf("exit = %d, stderr %q", code, stderr)
		}
	})
}

func TestDiffUsageErrors(t *testing.T) {
	cfgPath, _ := statusSetup(t)
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no agent flag", args: []string{"diff", "mocked-skill-00002"}, wantErr: "choose an agent"},
		{name: "unknown agent", args: []string{"diff", "mocked-skill-00002", "-a", "windsurf"}, wantErr: "unknown agent"},
		{name: "no skill", args: []string{"diff", "-a", "claude"}, wantErr: "accepts 1 arg"},
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
