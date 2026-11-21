package tui

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"sapato/internal/config"
	"sapato/internal/vault"
)

// runConfig is the config of a run whose file is at a temp path, existing or not,
// with the built-in agents under a temp home and a vault that is not created yet.
func runConfig(t *testing.T, fileExists bool) config.Config {
	t.Helper()
	home := t.TempDir()
	cfg, err := config.Load(filepath.Join(home, "missing.toml"), home)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Path = filepath.Join(t.TempDir(), "config.toml")
	cfg.FileExists = fileExists
	cfg.Vault = filepath.Join(t.TempDir(), "vault")
	return cfg
}

// keyReader feeds keys one byte per read. Bubbletea reads a run of bytes in one read
// as a single paste, so keys must arrive one read at a time.
type keyReader struct{ keys string }

func (r *keyReader) Read(p []byte) (int, error) {
	if r.keys == "" {
		return 0, io.EOF
	}
	p[0] = r.keys[0]
	r.keys = r.keys[1:]
	return 1, nil
}

func TestRunWithoutVaultAsksBeforeFailing(t *testing.T) {
	cfg := runConfig(t, true)
	err := Run(cfg, &keyReader{keys: "n"}, &bytes.Buffer{})

	want := "no vault at " + cfg.Vault + "; run sapato init"
	if err == nil || err.Error() != want {
		t.Fatalf("Run() error = %v, want %q", err, want)
	}
	if ok, _ := vault.Exists(cfg.Vault); ok {
		t.Error("the vault was created")
	}
}

func TestRunCreatesConfigAndVault(t *testing.T) {
	cfg := runConfig(t, false)
	if err := Run(cfg, &keyReader{keys: "yyq"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if _, err := os.Stat(cfg.Path); err != nil {
		t.Errorf("config file: %v", err)
	}
	if _, err := vault.Open(cfg.Vault); err != nil {
		t.Errorf("vault.Open(created) error = %v", err)
	}
}

func TestRunOpensExistingVaultWithoutPrompt(t *testing.T) {
	cfg := runConfig(t, true)
	if err := vault.Init(cfg.Vault); err != nil {
		t.Fatal(err)
	}
	if err := Run(cfg, &keyReader{keys: "q"}, &bytes.Buffer{}); err != nil {
		t.Errorf("Run() error = %v", err)
	}
}
