package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"sapato/internal/config"
	"sapato/internal/tui/icons"
	"sapato/internal/tui/theme"
	"sapato/internal/vault"
)

// promptConfig is the config of a run whose file is at path, existing or not. The
// vault is in a temp dir of its own, created first when vaultExists is set.
func promptConfig(t *testing.T, path string, fileExists, vaultExists bool) config.Config {
	t.Helper()
	vaultPath := filepath.Join(t.TempDir(), "vault")
	if vaultExists {
		if err := vault.Init(vaultPath); err != nil {
			t.Fatal(err)
		}
	}
	return config.Config{Path: path, FileExists: fileExists, Vault: vaultPath, Agents: []config.Agent{{ID: "claude", Path: filepath.Join(t.TempDir(), "claude")}}}
}

// promptModel is model with the backend Run would pass: a fake one when the vault
// exists, nil when it does not. The repaint timers are stubbed out as testModel does,
// so a notification does not hold the test for its lifetime.
func promptModel(t *testing.T, cfg config.Config, th theme.Theme, set icons.Set) Model {
	t.Helper()
	var b Backend
	ok, err := vault.Exists(cfg.Vault)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		b = &fakeBackend{}
	}
	m := model(cfg, b, th, set)
	m.tick = func(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return nil }
	return m
}

func TestFirstLaunchPromptOnlyWithoutConfigFile(t *testing.T) {
	th, set := testLook(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	tests := []struct {
		name        string
		fileExists  bool
		vaultExists bool
		wantMode    mode
		wantOp      string
	}{
		{name: "no file offers to create it", fileExists: false, vaultExists: true, wantMode: modeConfirm, wantOp: opConfigInit},
		{name: "existing file shows no prompt", fileExists: true, vaultExists: true, wantMode: modeNormal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := promptModel(t, promptConfig(t, path, tt.fileExists, tt.vaultExists), th, set)
			if m.mode != tt.wantMode {
				t.Fatalf("mode = %v, want %v", m.mode, tt.wantMode)
			}
			if m.pending.act.op != tt.wantOp {
				t.Errorf("prompt op = %q, want %q", m.pending.act.op, tt.wantOp)
			}
		})
	}
}

func TestFirstLaunchPromptAsksWithThePath(t *testing.T) {
	th, set := testLook(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	m := sized(t, promptModel(t, promptConfig(t, path, false, true), th, set), 160, 40)
	if view := m.View(); !strings.Contains(view, "No config found. Create "+path+"?") {
		t.Errorf("view lacks the prompt:\n%s", view)
	}
}

func TestFirstLaunchPromptYesWritesLoadableFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	th, set := testLook(t)
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	m := promptModel(t, promptConfig(t, path, false, true), th, set)
	m = press(t, m, "y")

	if m.mode != modeNormal {
		t.Errorf("mode = %v, want the grid", m.mode)
	}
	if last := m.lastNote(); last.title != "config created" || last.message != path || last.kind != noteSuccess {
		t.Errorf("note = %+v, want config created at %s", last, path)
	}
	got, err := config.Load(path, home)
	if err != nil {
		t.Fatalf("Load(written) error = %v", err)
	}
	want, err := config.Load(filepath.Join(t.TempDir(), "missing.toml"), home)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Agents, want.Agents) || got.UI != want.UI {
		t.Errorf("written config = %+v, want the defaults %+v", got, want)
	}
}

func TestFirstLaunchPromptNKeepsDefaultsAndWritesNothing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	th, set := testLook(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	m := promptModel(t, promptConfig(t, path, false, true), th, set)
	m = press(t, m, "n")

	if m.mode != modeNormal {
		t.Errorf("mode = %v, want the grid", m.mode)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("n wrote the config (err %v)", err)
	}
}

func TestFirstLaunchPromptShowsAWriteError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	th, set := testLook(t)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(blocker, "config.toml")
	m := promptModel(t, promptConfig(t, path, false, true), th, set)
	m = press(t, m, "y")

	if !m.lastErr() {
		t.Errorf("note = %+v, want an error", m.lastNote())
	}
}
