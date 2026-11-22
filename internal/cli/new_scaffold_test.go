package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// stubEditor replaces runEditor for a test. The argv it receives is recorded.
func stubEditor(t *testing.T) *[]string {
	t.Helper()
	var argv []string
	prev := runEditor
	runEditor = func(a []string) error {
		argv = a
		return nil
	}
	t.Cleanup(func() { runEditor = prev })
	return &argv
}

func TestNewScaffoldUsageErrors(t *testing.T) {
	cfgPath, _ := vaultSetup(t)
	file := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(file, []byte(cliNewSkill), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		args []string
	}{
		{name: "file and --name", args: []string{"new", file, "--name", "x", "--description", "d"}},
		{name: "--name without --description", args: []string{"new", "--name", "x"}},
		{name: "--description without --name", args: []string{"new", "--description", "d"}},
		{name: "--compatible without --name", args: []string{"new", "--compatible", "claude"}},
		{name: "--edit without --name", args: []string{"new", "--edit"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, stderr := runCLI(append([]string{"--config", cfgPath}, tt.args...)...)
			if code != 2 {
				t.Errorf("exit = %d, want 2 (stderr %q)", code, stderr)
			}
		})
	}
}

func TestNewScaffoldCommand(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	cfgPath, vaultDir := vaultSetup(t)
	skillDir := filepath.Join(vaultDir, "skills", "new-skill")

	t.Run("dry run plans the scaffold and changes nothing", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "--dry-run", "new", "--name", "new-skill", "--description", "A new skill.")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.Contains(out, "would create new-skill") {
			t.Errorf("output = %q", out)
		}
		if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
			t.Errorf("dry run created the skill (err %v)", err)
		}
	})

	t.Run("scaffold writes the template", func(t *testing.T) {
		code, out, stderr := runCLI("--config", cfgPath, "new", "--name", "new-skill", "--description", "A new skill.")
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q", code, stderr)
		}
		if !strings.Contains(out, "create new-skill: "+skillDir) {
			t.Errorf("output = %q", out)
		}
		data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(data), "---\nname: new-skill\ndescription: A new skill.\n---\n") {
			t.Errorf("SKILL.md starts %q", data)
		}
	})

	t.Run("existing name is refused without --force", func(t *testing.T) {
		code, _, stderr := runCLI("--config", cfgPath, "new", "--name", "new-skill", "--description", "Again.")
		if code != 1 || !strings.Contains(stderr, "already in the vault (use --force)") {
			t.Errorf("exit = %d, stderr %q", code, stderr)
		}
	})

	t.Run("--force replaces it", func(t *testing.T) {
		code, _, stderr := runCLI("--config", cfgPath, "new", "--force", "--name", "new-skill", "--description", "Again.")
		if code != 0 {
			t.Errorf("exit = %d, stderr %q", code, stderr)
		}
	})

	t.Run("invalid description is a runtime error", func(t *testing.T) {
		code, _, stderr := runCLI("--config", cfgPath, "new", "--name", "other-skill", "--description", "")
		if code != 1 || !strings.Contains(stderr, "description must be a non-empty string") {
			t.Errorf("exit = %d, stderr %q", code, stderr)
		}
	})
}

func TestNewScaffoldCompatible(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	cfgPath, vaultDir := vaultSetup(t)
	toml := filepath.Join(vaultDir, "sapato.toml")
	tests := []struct {
		name       string
		compatible string
		wantTable  bool
	}{
		{name: "a subset writes the table", compatible: "claude,cursor", wantTable: true},
		{name: "every configured agent writes no table", compatible: "claude,cursor,pi,opencode,agents", wantTable: false},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := fmt.Sprintf("compat-skill-%d", i)
			before, err := os.ReadFile(toml)
			if err != nil {
				t.Fatal(err)
			}
			code, _, stderr := runCLI("--config", cfgPath, "new", "--name", name, "--description", "Compat.", "--compatible", tt.compatible)
			if code != 0 {
				t.Fatalf("exit = %d, stderr %q", code, stderr)
			}
			after, err := os.ReadFile(toml)
			if err != nil {
				t.Fatal(err)
			}
			hasTable := strings.Contains(string(after), "[skills."+name+"]")
			if hasTable != tt.wantTable {
				t.Errorf("table present = %t, want %t:\n%s", hasTable, tt.wantTable, after)
			}
			if !tt.wantTable && string(after) != string(before) {
				t.Errorf("sapato.toml changed without a table:\n%s", after)
			}
		})
	}
}

func TestNewScaffoldEdit(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	tests := []struct {
		name   string
		visual string
		editor string
		want   string
	}{
		{name: "VISUAL wins", visual: "hx -w", editor: "nano", want: "hx"},
		{name: "EDITOR when VISUAL is unset", editor: "nano", want: "nano"},
		{name: "vi when neither is set", want: "vi"},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("VISUAL", tt.visual)
			t.Setenv("EDITOR", tt.editor)
			argv := stubEditor(t)
			cfgPath, vaultDir := vaultSetup(t)
			name := fmt.Sprintf("edit-skill-%d", i)
			code, _, stderr := runCLI("--config", cfgPath, "new", "--edit", "--name", name, "--description", "Edit me.")
			if code != 0 {
				t.Fatalf("exit = %d, stderr %q", code, stderr)
			}
			want := []string{tt.want, filepath.Join(vaultDir, "skills", name, "SKILL.md")}
			if tt.visual == "hx -w" {
				want = []string{"hx", "-w", want[1]}
			}
			if !slices.Equal(*argv, want) {
				t.Errorf("editor argv = %q, want %q", *argv, want)
			}
		})
	}
}

func TestNewScaffoldEditSkippedOnDryRun(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	argv := stubEditor(t)
	cfgPath, _ := vaultSetup(t)
	code, _, stderr := runCLI("--config", cfgPath, "--dry-run", "new", "--edit", "--name", "dry-edit", "--description", "Dry.")
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q", code, stderr)
	}
	if *argv != nil {
		t.Errorf("editor ran on a dry run: %q", *argv)
	}
}
