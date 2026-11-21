package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setup isolates HOME and writes a config pointing the vault at a temp dir.
func setup(t *testing.T) (cfgPath, vaultDir string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	vaultDir = filepath.Join(dir, "vault")
	cfgPath = filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("vault = \""+vaultDir+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfgPath, vaultDir
}

func runCLI(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = Run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func writeSkill(t *testing.T, vaultDir, name, content string) {
	t.Helper()
	dir := filepath.Join(vaultDir, "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExitCodes(t *testing.T) {
	cfgPath, _ := setup(t)
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantErr  string
	}{
		{name: "config ok", args: []string{"--config", cfgPath, "config"}, wantCode: 0},
		{name: "unknown command is usage error", args: []string{"--config", cfgPath, "frobnicate"}, wantCode: 2, wantErr: "unknown command"},
		{name: "unknown flag is usage error", args: []string{"--config", cfgPath, "list", "--nope"}, wantCode: 2, wantErr: "unknown flag"},
		{name: "extra args to list is usage error", args: []string{"--config", cfgPath, "list", "x"}, wantCode: 2, wantErr: "unknown command"},
		{name: "extra args to init is usage error", args: []string{"--config", cfgPath, "init", "a", "b"}, wantCode: 2, wantErr: "accepts at most 1 arg"},
		{name: "unknown agent in config is runtime error", args: []string{"--config", writeBadConfig(t), "config"}, wantCode: 1, wantErr: "unknown agent"},
		{name: "list without vault is runtime error", args: []string{"--config", cfgPath, "list"}, wantCode: 1, wantErr: "open vault"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, stderr := runCLI(tt.args...)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stderr %q)", code, tt.wantCode, stderr)
			}
			if tt.wantErr != "" && !strings.Contains(stderr, tt.wantErr) {
				t.Errorf("stderr = %q, want containing %q", stderr, tt.wantErr)
			}
		})
	}
}

func writeBadConfig(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "bad.toml")
	if err := os.WriteFile(p, []byte("[agents.windsurf]\npath = \"/w\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestConfigCommand(t *testing.T) {
	cfgPath, vaultDir := setup(t)
	code, out, stderr := runCLI("--config", cfgPath, "config")
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q", code, stderr)
	}
	if !strings.Contains(out, "vault: "+vaultDir) {
		t.Errorf("output missing vault line:\n%s", out)
	}
	for _, id := range []string{"claude", "cursor", "pi", "opencode", "agents"} {
		if !strings.Contains(out, id) {
			t.Errorf("output missing agent %s:\n%s", id, out)
		}
	}
}

func TestInitAndList(t *testing.T) {
	cfgPath, vaultDir := setup(t)

	t.Run("dry-run changes nothing", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "--dry-run", "init")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.Contains(out, "would create vault at "+vaultDir) {
			t.Errorf("output = %q", out)
		}
		if _, err := os.Stat(vaultDir); !os.IsNotExist(err) {
			t.Errorf("dry-run created %s (err %v)", vaultDir, err)
		}
	})

	t.Run("init uses configured vault when no path given", func(t *testing.T) {
		code, _, stderr := runCLI("--config", cfgPath, "init")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if _, err := os.Stat(filepath.Join(vaultDir, "sapato.toml")); err != nil {
			t.Errorf("manifest not created: %v", err)
		}
	})

	t.Run("init refuses existing vault", func(t *testing.T) {
		code, _, stderr := runCLI("--config", cfgPath, "init")
		if code != 1 || !strings.Contains(stderr, "already exists") {
			t.Errorf("exit = %d, stderr %q; want 1 and already exists", code, stderr)
		}
	})

	writeSkill(t, vaultDir, "code-review", "---\nname: code-review\ndescription: Reviews code.\nmetadata:\n  version: 1.2.0\n---\n")
	writeSkill(t, vaultDir, "broken", "no frontmatter\n")

	t.Run("list shows valid and invalid skills", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "list")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) != 3 {
			t.Fatalf("got %d lines, want header + 2 skills:\n%s", len(lines), out)
		}
		if !strings.Contains(lines[1], "broken") || !strings.Contains(lines[1], "invalid: frontmatter: missing opening --- line") {
			t.Errorf("broken row = %q", lines[1])
		}
		if !strings.Contains(lines[2], "code-review") || !strings.Contains(lines[2], "1.2.0") || !strings.Contains(lines[2], "valid") {
			t.Errorf("code-review row = %q", lines[2])
		}
	})
}
