package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sapato/internal/vault"
)

// syncSetup makes a vault in a temp dir with the given assignments and valid
// skills, and points the config at it. HOME is isolated, so the agents' default
// dirs are under it. It returns the config path, the vault dir and claude's dir.
func syncSetup(t *testing.T, assignments string, skills ...string) (cfgPath, vaultDir, claudeDir string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := t.TempDir()
	vaultDir = filepath.Join(dir, "vault")
	if err := vault.Init(vaultDir); err != nil {
		t.Fatal(err)
	}
	manifest := "format = 1\n\n[assignments]\n" + assignments
	if err := os.WriteFile(filepath.Join(vaultDir, "sapato.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range skills {
		writeSkill(t, vaultDir, name, "---\nname: "+name+"\ndescription: A skill.\n---\n")
	}
	cfgPath = filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("vault = \""+vaultDir+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfgPath, vaultDir, filepath.Join(home, ".claude", "skills")
}

func TestSyncCommand(t *testing.T) {
	t.Run("installs assigned skills", func(t *testing.T) {
		cfgPath, _, claudeDir := syncSetup(t, "claude = [\"code-review\"]\n", "code-review")
		code, out, stderr := runCLI("--config", cfgPath, "sync")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		want := "install code-review in claude: " + filepath.Join(claudeDir, "code-review") + "\n"
		if out != want {
			t.Errorf("output = %q, want %q", out, want)
		}
	})

	t.Run("dry run says would and changes nothing", func(t *testing.T) {
		cfgPath, _, claudeDir := syncSetup(t, "claude = [\"code-review\"]\n", "code-review")
		code, out, _ := runCLI("--config", cfgPath, "--dry-run", "sync")
		if code != 0 || !strings.HasPrefix(out, "would install code-review in claude") {
			t.Errorf("exit = %d, output %q", code, out)
		}
		if _, err := os.Stat(filepath.Join(claudeDir, "code-review")); !os.IsNotExist(err) {
			t.Errorf("dry run created the skill folder (err %v)", err)
		}
	})

	t.Run("refused install is an error", func(t *testing.T) {
		cfgPath, _, claudeDir := syncSetup(t, "claude = [\"code-review\"]\n", "code-review")
		if err := os.MkdirAll(filepath.Join(claudeDir, "code-review"), 0o755); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := runCLI("--config", cfgPath, "sync")
		if code != 1 || !strings.Contains(stderr, "refusing to overwrite code-review in claude") {
			t.Errorf("exit = %d, stderr %q", code, stderr)
		}
	})

	t.Run("unknown agent in assignments is an error", func(t *testing.T) {
		cfgPath, _, _ := syncSetup(t, "nope = [\"code-review\"]\n", "code-review")
		code, _, stderr := runCLI("--config", cfgPath, "sync")
		if code != 1 || !strings.Contains(stderr, `unknown agent "nope"`) {
			t.Errorf("exit = %d, stderr %q", code, stderr)
		}
	})

	t.Run("prune uninstalls unassigned skills", func(t *testing.T) {
		cfgPath, _, claudeDir := syncSetup(t, "claude = [\"code-review\"]\n", "code-review", "obsidian-daily")
		if code, _, stderr := runCLI("--config", cfgPath, "sync"); code != 0 {
			t.Fatalf("setup sync exit = %d, stderr %q", code, stderr)
		}
		// obsidian-daily is installed by hand, outside the assignments.
		if code, _, stderr := runCLI("--config", cfgPath, "install", "obsidian-daily", "-a", "claude"); code != 0 {
			t.Fatalf("setup install exit = %d, stderr %q", code, stderr)
		}
		code, out, stderr := runCLI("--config", cfgPath, "sync", "--prune")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		want := "uninstall obsidian-daily in claude: " + filepath.Join(claudeDir, "obsidian-daily") + "\n"
		if out != want {
			t.Errorf("output = %q, want %q", out, want)
		}
	})

	t.Run("extra arguments are a usage error", func(t *testing.T) {
		cfgPath, _, _ := syncSetup(t, "")
		if code, _, _ := runCLI("--config", cfgPath, "sync", "code-review"); code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
	})
}

func TestDoctorCommand(t *testing.T) {
	t.Run("clean vault reports no problems", func(t *testing.T) {
		cfgPath, _, _ := syncSetup(t, "", "code-review")
		code, out, stderr := runCLI("--config", cfgPath, "doctor")
		if code != 0 || out != "no problems found\n" {
			t.Errorf("exit = %d, output %q, stderr %q", code, out, stderr)
		}
	})

	t.Run("warnings alone exit 0", func(t *testing.T) {
		cfgPath, vaultDir, _ := syncSetup(t, "", "code-review")
		if code, _, stderr := runCLI("--config", cfgPath, "install", "code-review", "-a", "claude"); code != 0 {
			t.Fatalf("setup install exit = %d, stderr %q", code, stderr)
		}
		if err := os.RemoveAll(filepath.Join(vaultDir, "skills", "code-review")); err != nil {
			t.Fatal(err)
		}
		code, out, stderr := runCLI("--config", cfgPath, "doctor")
		if code != 0 || !strings.Contains(out, "warn: code-review in claude: installed but no longer in the vault") {
			t.Errorf("exit = %d, output %q, stderr %q", code, out, stderr)
		}
	})

	t.Run("an error exits 1", func(t *testing.T) {
		cfgPath, vaultDir, _ := syncSetup(t, "", "code-review")
		writeSkill(t, vaultDir, "broken", "# no frontmatter\n")
		code, out, stderr := runCLI("--config", cfgPath, "doctor")
		if code != 1 || !strings.Contains(out, "error: vault skill broken is invalid") {
			t.Errorf("exit = %d, output %q", code, out)
		}
		if !strings.Contains(stderr, "doctor found 1 error(s)") {
			t.Errorf("stderr = %q", stderr)
		}
	})
}
