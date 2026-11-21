package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sapato/internal/testfixtures"
)

const cliNewSkill = "---\nname: cli-new\ndescription: Made from the CLI.\n---\n"

// vaultSetup isolates HOME and points a config at a copy of the fixture vault.
func vaultSetup(t *testing.T) (cfgPath, vaultDir string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	vaultDir = testfixtures.CopyTree(t, filepath.Join(testfixtures.Root(), "skill-db"))
	cfgPath = filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte("vault = \""+vaultDir+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfgPath, vaultDir
}

func TestNewCommand(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	cfgPath, vaultDir := vaultSetup(t)
	file := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(file, []byte(cliNewSkill), 0o644); err != nil {
		t.Fatal(err)
	}
	vaultSkill := func() string { return filepath.Join(vaultDir, "skills", "cli-new") }

	t.Run("dry run plans the create and changes nothing", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "--dry-run", "new", file)
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.Contains(out, "would create cli-new") {
			t.Errorf("output = %q", out)
		}
		if _, err := os.Stat(vaultSkill()); !os.IsNotExist(err) {
			t.Errorf("dry run created the skill (err %v)", err)
		}
	})

	t.Run("file creates the skill", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "new", file)
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.Contains(out, "create cli-new: "+vaultSkill()) {
			t.Errorf("output = %q", out)
		}
		if _, err := os.Stat(filepath.Join(vaultSkill(), "SKILL.md")); err != nil {
			t.Errorf("SKILL.md not created: %v", err)
		}
	})

	t.Run("existing name is refused without --force", func(t *testing.T) {
		code, _, stderr := runCLI("--config", cfgPath, "new", file)
		if code != 1 || !strings.Contains(stderr, "already in the vault (use --force)") {
			t.Errorf("exit = %d, stderr %q", code, stderr)
		}
	})

	t.Run("--force replaces it", func(t *testing.T) {
		code, _, stderr := runCLI("--config", cfgPath, "new", "--force", file)
		if code != 0 {
			t.Errorf("exit = %d, stderr %q", code, stderr)
		}
	})

	t.Run("missing file is a runtime error", func(t *testing.T) {
		code, _, stderr := runCLI("--config", cfgPath, "new", filepath.Join(t.TempDir(), "absent.md"))
		if code != 1 || !strings.Contains(stderr, "read ") {
			t.Errorf("exit = %d, stderr %q", code, stderr)
		}
	})

	t.Run("extra args are a usage error", func(t *testing.T) {
		code, _, _ := runCLI("--config", cfgPath, "new", file, "more")
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
	})
}

func TestExportCommand(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	cfgPath, _ := vaultSetup(t)

	t.Run("output flag writes the tarball", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "one.skill.tar.gz")
		code, stdout, stderr := runCLI("--config", cfgPath, "export", "mocked-skill-00004", "-o", out)
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.Contains(stdout, "wrote "+out) {
			t.Errorf("output = %q", stdout)
		}
		if _, err := os.Stat(out); err != nil {
			t.Errorf("tarball not written: %v", err)
		}
	})

	t.Run("existing output file is refused", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "taken.skill.tar.gz")
		if err := os.WriteFile(out, []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := runCLI("--config", cfgPath, "export", "mocked-skill-00004", "-o", out)
		if code != 1 || !strings.Contains(stderr, "file exists") {
			t.Errorf("exit = %d, stderr %q", code, stderr)
		}
		if data, _ := os.ReadFile(out); string(data) != "keep" {
			t.Errorf("existing file changed to %q", data)
		}
	})

	t.Run("default name comes from version and hash", func(t *testing.T) {
		t.Chdir(t.TempDir())
		code, _, stderr := runCLI("--config", cfgPath, "export", "mocked-skill-00002")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if _, err := os.Stat("mocked-skill-00002-1.2.0.skill.tar.gz"); err != nil {
			t.Errorf("default-named tarball not written: %v", err)
		}
	})

	t.Run("several skills use the date in the default name", func(t *testing.T) {
		t.Chdir(t.TempDir())
		code, _, stderr := runCLI("--config", cfgPath, "export", "mocked-skill-0000[12]")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		want := "skills-" + time.Now().UTC().Format("2006-01-02") + ".skill.tar.gz"
		if _, err := os.Stat(want); err != nil {
			t.Errorf("tarball %s not written: %v", want, err)
		}
	})

	t.Run("dry run names the file and writes nothing", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		code, out, stderr := runCLI("--config", cfgPath, "--dry-run", "export", "mocked-skill-00002")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.Contains(out, "would write mocked-skill-00002-1.2.0.skill.tar.gz") {
			t.Errorf("output = %q", out)
		}
		if left, _ := os.ReadDir(dir); len(left) != 0 {
			t.Errorf("dry run wrote %d files", len(left))
		}
	})

	t.Run("glob skips invalid skills and says so", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "all.skill.tar.gz")
		code, stdout, stderr := runCLI("--config", cfgPath, "export", "invalid-missing-*", "mocked-skill-00004", "-o", out)
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.Contains(stdout, "skip invalid-missing-description: invalid") {
			t.Errorf("output = %q", stdout)
		}
	})

	t.Run("no skills is a usage error", func(t *testing.T) {
		code, _, _ := runCLI("--config", cfgPath, "export")
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
	})
}

func TestImportArchiveCommand(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	cfgPath, _ := vaultSetup(t)
	archive := filepath.Join(t.TempDir(), "one.skill.tar.gz")
	if code, _, stderr := runCLI("--config", cfgPath, "export", "mocked-skill-00003", "-o", archive); code != 0 {
		t.Fatalf("export exit = %d, stderr %q", code, stderr)
	}

	// The fixture vault already holds mocked-skill-00003, so the import needs --force.
	code, _, stderr := runCLI("--config", cfgPath, "import", archive)
	if code != 1 || !strings.Contains(stderr, "already in the vault (use --force)") {
		t.Errorf("exit = %d, stderr %q", code, stderr)
	}

	code, out, stderr := runCLI("--config", cfgPath, "import", "--force", archive)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q", code, stderr)
	}
	if !strings.Contains(out, "import mocked-skill-00003: ") {
		t.Errorf("output = %q", out)
	}
}
