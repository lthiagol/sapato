package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"sapato/internal/vault"
)

func TestVaultPromptOnlyWhenVaultMissing(t *testing.T) {
	th, set := testLook(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	tests := []struct {
		name        string
		fileExists  bool
		vaultExists bool
		wantMode    mode
		wantOp      string
	}{
		{name: "config and vault present show no prompt", fileExists: true, vaultExists: true, wantMode: modeNormal},
		{name: "missing vault alone is offered", fileExists: true, vaultExists: false, wantMode: modeConfirm, wantOp: opVaultInit},
		{name: "missing config comes before the vault", fileExists: false, vaultExists: false, wantMode: modeConfirm, wantOp: opConfigInit},
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

func TestVaultPromptAsksWithThePath(t *testing.T) {
	th, set := testLook(t)
	cfg := promptConfig(t, filepath.Join(t.TempDir(), "config.toml"), true, false)
	m := sized(t, promptModel(t, cfg, th, set), 160, 40)
	if view := m.View(); !strings.Contains(view, "No vault at "+cfg.Vault+". Create it?") {
		t.Errorf("view lacks the prompt:\n%s", view)
	}
}

func TestVaultPromptFollowsConfigPrompt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	th, set := testLook(t)
	tests := []struct {
		key      string
		wantFile bool
		wantNote string
	}{
		{key: "y", wantFile: true, wantNote: "config created"},
		{key: "n", wantFile: false, wantNote: "cancelled"},
	}
	for _, tt := range tests {
		t.Run("config "+tt.key, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			m := promptModel(t, promptConfig(t, path, false, false), th, set)
			m = press(t, m, tt.key)

			if _, err := os.Stat(path); (err == nil) != tt.wantFile {
				t.Errorf("config file exists = %t, want %t", err == nil, tt.wantFile)
			}
			if m.mode != modeConfirm || m.pending.act.op != opVaultInit {
				t.Errorf("mode = %v, prompt op = %q; want the vault prompt next", m.mode, m.pending.act.op)
			}
			if last := m.lastNote(); last.title != tt.wantNote {
				t.Errorf("note = %+v, want %q", last, tt.wantNote)
			}
		})
	}
}

func TestVaultPromptYesCreatesVault(t *testing.T) {
	th, set := testLook(t)
	cfg := promptConfig(t, filepath.Join(t.TempDir(), "config.toml"), true, false)
	m := promptModel(t, cfg, th, set)
	m = press(t, m, "y")

	if _, err := vault.Open(cfg.Vault); err != nil {
		t.Fatalf("vault.Open(created) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Vault, "skills")); err != nil {
		t.Errorf("skills dir: %v", err)
	}
	if m.backend == nil || m.mode != modeNormal || m.noVault != "" {
		t.Errorf("backend nil = %t, mode = %v, noVault = %q; want the grid on the vault", m.backend == nil, m.mode, m.noVault)
	}
	// The grid loads after y. A failed load would be the newest note.
	if last := m.lastNote(); last.title != "vault created" || last.message != cfg.Vault || last.kind != noteSuccess {
		t.Errorf("note = %+v, want vault created at %s", last, cfg.Vault)
	}
}

func TestVaultPromptAnyOtherKeyExits(t *testing.T) {
	th, set := testLook(t)
	for _, k := range []string{"n", "x", "esc"} {
		t.Run(k, func(t *testing.T) {
			cfg := promptConfig(t, filepath.Join(t.TempDir(), "config.toml"), true, false)
			m := promptModel(t, cfg, th, set)
			if _, quit := send(t, m, keyMsg(k)); !quit {
				t.Error("the key did not quit")
			}
			if ok, _ := vault.Exists(cfg.Vault); ok {
				t.Error("the vault was created")
			}
		})
	}
}

func TestVaultPromptFailureKeepsPrompt(t *testing.T) {
	th, set := testLook(t)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := promptConfig(t, filepath.Join(t.TempDir(), "config.toml"), true, false)
	cfg.Vault = filepath.Join(blocker, "vault")
	m := model(cfg, nil, th, set)
	m.tick = func(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return nil }
	m, quit := send(t, m, keyMsg("y"))

	if quit {
		t.Error("a failed create quit the program")
	}
	if !m.lastErr() || m.mode != modeConfirm || m.backend != nil {
		t.Errorf("note = %+v, mode = %v, backend nil = %t; want the error and the prompt still up", m.lastNote(), m.mode, m.backend == nil)
	}
}
