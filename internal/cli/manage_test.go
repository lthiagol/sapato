package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sapato/internal/testfixtures"
)

// manageSetup points the vault and the claude target at copies of the fixtures
// and returns the config path, the vault and the claude skills dir.
func manageSetup(t *testing.T) (cfgPath, vaultDir, claudeDir string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	vaultDir = testfixtures.CopyTree(t, filepath.Join(testfixtures.Root(), "skill-db"))
	claudeDir = testfixtures.CopyTree(t, filepath.Join(testfixtures.Root(), "targets", "claude"))
	cfgPath = filepath.Join(t.TempDir(), "config.toml")
	content := "vault = \"" + vaultDir + "\"\n\n[agents]\nclaude = \"" + claudeDir + "\"\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfgPath, vaultDir, claudeDir
}

func TestImportCommand(t *testing.T) {
	cfgPath, vaultDir, _ := manageSetup(t)

	fresh := filepath.Join(t.TempDir(), "fresh-skill")
	if err := os.MkdirAll(fresh, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fresh, "SKILL.md"), []byte("---\nname: fresh-skill\ndescription: Fresh.\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("dry-run prints the plan and changes nothing", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "--dry-run", "import", fresh)
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.HasPrefix(out, "would import fresh-skill: ") {
			t.Errorf("output = %q", out)
		}
		if _, err := os.Stat(filepath.Join(vaultDir, "skills", "fresh-skill")); !os.IsNotExist(err) {
			t.Errorf("dry run copied the skill (err %v)", err)
		}
	})

	t.Run("import copies the folder into the vault", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "import", fresh)
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.HasPrefix(out, "import fresh-skill: ") {
			t.Errorf("output = %q", out)
		}
		if _, err := os.Stat(filepath.Join(vaultDir, "skills", "fresh-skill", "SKILL.md")); err != nil {
			t.Errorf("skill not in vault: %v", err)
		}
	})

	t.Run("name already in the vault is refused and names --force", func(t *testing.T) {
		src := testfixtures.CopyTree(t, filepath.Join(testfixtures.Root(), "skill-db", "skills", "mocked-skill-00004"))
		code, _, stderr := runCLI("--config", cfgPath, "import", src)
		if code != 1 || !strings.Contains(stderr, "already in the vault") || !strings.Contains(stderr, "--force") {
			t.Errorf("exit = %d, stderr %q; want 1 naming --force", code, stderr)
		}
	})

	t.Run("name differing from the frontmatter is refused", func(t *testing.T) {
		code, _, stderr := runCLI("--config", cfgPath, "import", fresh, "--name", "other")
		if code != 1 || !strings.Contains(stderr, "does not match frontmatter name") {
			t.Errorf("exit = %d, stderr %q", code, stderr)
		}
	})

	t.Run("missing argument is a usage error", func(t *testing.T) {
		if code, _, _ := runCLI("--config", cfgPath, "import"); code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
	})
}

func TestAdoptCommand(t *testing.T) {
	cfgPath, vaultDir, claudeDir := manageSetup(t)

	t.Run("missing --agent is a usage error", func(t *testing.T) {
		code, _, stderr := runCLI("--config", cfgPath, "adopt", "hand-made-skill")
		if code != 2 || !strings.Contains(stderr, "--agent") {
			t.Errorf("exit = %d, stderr %q", code, stderr)
		}
	})

	t.Run("unknown agent is a usage error", func(t *testing.T) {
		if code, _, _ := runCLI("--config", cfgPath, "adopt", "hand-made-skill", "-a", "nope"); code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
	})

	t.Run("managed skill is refused", func(t *testing.T) {
		code, _, stderr := runCLI("--config", cfgPath, "adopt", "mocked-skill-00001", "-a", "claude")
		if code != 1 || !strings.Contains(stderr, "synced, want unmanaged") {
			t.Errorf("exit = %d, stderr %q", code, stderr)
		}
	})

	t.Run("adopt copies the folder into the vault", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "adopt", "hand-made-skill", "-a", "claude")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		want := "adopt hand-made-skill in claude: " + filepath.Join(claudeDir, "hand-made-skill")
		if strings.TrimSpace(out) != want {
			t.Errorf("output = %q, want %q", out, want)
		}
		if _, err := os.Stat(filepath.Join(vaultDir, "skills", "hand-made-skill", "SKILL.md")); err != nil {
			t.Errorf("skill not in vault: %v", err)
		}
	})
}

func TestRemoveCommand(t *testing.T) {
	cfgPath, vaultDir, _ := manageSetup(t)

	t.Run("installed skill is refused and names --force", func(t *testing.T) {
		code, _, stderr := runCLI("--config", cfgPath, "remove", "mocked-skill-00001")
		if code != 1 || !strings.Contains(stderr, "installed in claude") || !strings.Contains(stderr, "--force") {
			t.Errorf("exit = %d, stderr %q", code, stderr)
		}
	})

	t.Run("unknown skill is refused", func(t *testing.T) {
		code, _, stderr := runCLI("--config", cfgPath, "remove", "nope")
		if code != 1 || !strings.Contains(stderr, "no skill in vault") {
			t.Errorf("exit = %d, stderr %q", code, stderr)
		}
	})

	t.Run("dry-run prints the plan and changes nothing", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "--dry-run", "remove", "mocked-skill-00006")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.HasPrefix(out, "would remove mocked-skill-00006: ") {
			t.Errorf("output = %q", out)
		}
		if _, err := os.Stat(filepath.Join(vaultDir, "skills", "mocked-skill-00006")); err != nil {
			t.Errorf("dry run removed the skill: %v", err)
		}
	})

	t.Run("--force removes an installed skill", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "remove", "mocked-skill-00001", "--force")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.HasPrefix(out, "remove mocked-skill-00001: ") {
			t.Errorf("output = %q", out)
		}
		if _, err := os.Stat(filepath.Join(vaultDir, "skills", "mocked-skill-00001")); !os.IsNotExist(err) {
			t.Errorf("skill still in vault (err %v)", err)
		}
	})

	t.Run("missing argument is a usage error", func(t *testing.T) {
		if code, _, _ := runCLI("--config", cfgPath, "remove"); code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
	})
}

func TestImportFlags(t *testing.T) {
	cfgPath, vaultDir, _ := manageSetup(t)
	pack := filepath.Join(t.TempDir(), "pack")
	writeImportFile(t, filepath.Join(pack, "one", "pack-one", "SKILL.md"), "---\nname: pack-one\ndescription: One.\n---\n")
	writeImportFile(t, filepath.Join(pack, "two", "pack-two", "SKILL.md"), "---\nname: pack-two\ndescription: Two.\n---\n")
	repair := filepath.Join(t.TempDir(), "repair-me")
	writeImportFile(t, filepath.Join(repair, "SKILL.md"), "# Repair me\n")

	t.Run("--name with --all is a usage error", func(t *testing.T) {
		code, _, stderr := runCLI("--config", cfgPath, "import", pack, "--all", "--name", "x")
		if code != 2 || !strings.Contains(stderr, "--name does not apply with --all") {
			t.Errorf("exit = %d, stderr %q; want 2 naming --all", code, stderr)
		}
	})

	t.Run("invalid skill names --fix", func(t *testing.T) {
		code, _, stderr := runCLI("--config", cfgPath, "import", repair)
		if code != 1 || !strings.Contains(stderr, "--fix can repair it") {
			t.Errorf("exit = %d, stderr %q; want 1 suggesting --fix", code, stderr)
		}
	})

	t.Run("--fix with --description imports the repaired skill", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "import", repair, "--fix", "--description", "Repaired.")
		if code != 0 || !strings.HasPrefix(out, "import repair-me: ") {
			t.Fatalf("exit = %d, out %q, stderr %q", code, out, stderr)
		}
		data, err := os.ReadFile(filepath.Join(vaultDir, "skills", "repair-me", "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if want := "---\nname: repair-me\ndescription: Repaired.\n---\n# Repair me\n"; string(data) != want {
			t.Errorf("vault SKILL.md = %q, want %q", data, want)
		}
	})

	t.Run("--all imports every skill of the source", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "import", pack, "--all")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		for _, name := range []string{"pack-one", "pack-two"} {
			if !strings.Contains(out, "import "+name+": ") {
				t.Errorf("output %q does not import %s", out, name)
			}
		}
	})

	t.Run("several skills without --all list their paths", func(t *testing.T) {
		code, _, stderr := runCLI("--config", cfgPath, "--dry-run", "import", pack)
		if code != 1 || !strings.Contains(stderr, "one/pack-one, two/pack-two") {
			t.Errorf("exit = %d, stderr %q; want 1 listing the paths", code, stderr)
		}
	})
}

// writeImportFile writes content to path, creating its folders.
func writeImportFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
