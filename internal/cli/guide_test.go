package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"sapato/internal/vault"
)

func TestGuidePrintsToStdout(t *testing.T) {
	cfgPath, vaultDir := vaultSetup(t)
	code, out, stderr := runCLI("--config", cfgPath, "guide")
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q", code, stderr)
	}
	if !strings.HasPrefix(out, "---\nname: sapato\n") {
		t.Errorf("output does not start with the frontmatter: %q", out[:min(len(out), 40)])
	}
	if !strings.Contains(out, "Vault path: "+vaultDir+"\n") {
		t.Errorf("output does not name the vault path %s", vaultDir)
	}
	claude := strings.Index(out, "- claude: ")
	cursor := strings.Index(out, "- cursor: ")
	if claude < 0 || cursor < claude {
		t.Errorf("agents are not listed in config order: claude at %d, cursor at %d", claude, cursor)
	}
	if _, err := os.Stat(filepath.Join(vaultDir, "skills", "sapato")); !os.IsNotExist(err) {
		t.Errorf("printing the guide wrote a skill (err %v)", err)
	}
}

func TestGuideUsageErrors(t *testing.T) {
	cfgPath, _ := vaultSetup(t)
	code, _, _ := runCLI("--config", cfgPath, "guide", "--force")
	if code != 2 {
		t.Errorf("--force without --install: exit = %d, want 2", code)
	}
}

func TestGuideInstallCommand(t *testing.T) {
	cfgPath, vaultDir := vaultSetup(t)
	skillDir := filepath.Join(vaultDir, "skills", "sapato")
	skillFile := filepath.Join(skillDir, "SKILL.md")

	t.Run("dry run plans the install and changes nothing", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "--dry-run", "guide", "--install")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.Contains(out, "would create sapato: "+skillDir) {
			t.Errorf("output = %q", out)
		}
		if _, err := os.Stat(skillFile); !os.IsNotExist(err) {
			t.Errorf("dry run wrote the guide (err %v)", err)
		}
	})

	t.Run("install writes the guide", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "guide", "--install")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.Contains(out, "create sapato: "+skillDir) {
			t.Errorf("output = %q", out)
		}
		v, err := vault.Open(vaultDir)
		if err != nil {
			t.Fatal(err)
		}
		skills, err := v.Skills()
		if err != nil {
			t.Fatal(err)
		}
		i := slices.IndexFunc(skills, func(s vault.Skill) bool { return s.Name == "sapato" })
		if i < 0 {
			t.Fatal("sapato is not in the vault after install")
		}
		if !skills[i].Valid {
			t.Errorf("installed guide is invalid: %v", skills[i].Problems)
		}
	})

	t.Run("install again is a no-op", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "guide", "--install")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.Contains(out, "skip sapato: already up to date") {
			t.Errorf("output = %q", out)
		}
	})

	t.Run("different content is refused, --force replaces it", func(t *testing.T) {
		if err := os.WriteFile(skillFile, []byte("---\nname: sapato\ndescription: Mine.\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := runCLI("--config", cfgPath, "guide", "--install")
		if code != 1 || !strings.Contains(stderr, "--force") {
			t.Errorf("exit = %d, stderr %q; want 1 with a --force hint", code, stderr)
		}
		code, out, stderr := runCLI("--config", cfgPath, "guide", "--install", "--force")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.Contains(out, "create sapato: ") {
			t.Errorf("output = %q", out)
		}
		if got := string(mustReadFile(t, skillFile)); !strings.Contains(got, "Vault path: "+vaultDir) {
			t.Errorf("force did not replace the guide")
		}
	})
}

func TestGuideInstallResolvesRelativeVault(t *testing.T) {
	cfgPath, vaultDir := vaultSetup(t)
	t.Chdir(filepath.Dir(vaultDir))
	if err := os.WriteFile(cfgPath, []byte("vault = \""+filepath.Base(vaultDir)+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI("--config", cfgPath, "guide", "--install")
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q", code, stderr)
	}
	got := string(mustReadFile(t, filepath.Join(vaultDir, "skills", "sapato", "SKILL.md")))
	if !strings.Contains(got, "Vault path: "+vaultDir+"\n") {
		t.Errorf("installed guide does not name the absolute vault path %s", vaultDir)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
