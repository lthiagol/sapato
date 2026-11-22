package tui

import (
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"sapato/internal/config"
	"sapato/internal/tui/icons"
	"sapato/internal/tui/theme"
	"sapato/internal/vault"
)

// Run opens the UI on the vault in cfg and blocks until the user quits. It
// reads keys from in and draws to out. The grid shows every configured agent,
// painted with cfg.UI's theme and icon set. Without a config file it first asks
// whether to create one; then, without a vault, whether to create it. Quitting
// with no vault open is an error. NO_COLOR, when set to a non-empty value, turns
// colors off.
func Run(cfg config.Config, in io.Reader, out io.Writer) error {
	th, ok := theme.ByName(cfg.UI.Theme)
	if !ok {
		return fmt.Errorf("unknown theme %q", cfg.UI.Theme)
	}
	set, ok := icons.ByName(cfg.UI.Icons)
	if !ok {
		return fmt.Errorf("unknown icon set %q", cfg.UI.Icons)
	}
	b, err := openBackend(cfg)
	if err != nil {
		return err
	}
	m := model(cfg, b, th, set)
	p := tea.NewProgram(m, tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	if fm, ok := final.(Model); ok && fm.backend == nil {
		return fmt.Errorf("no vault at %s; run sapato init", cfg.Vault)
	}
	return nil
}

// openBackend returns the backend on the vault in cfg. It is nil, with no error,
// when the vault does not exist yet: the UI then offers to create it. A vault that
// exists but does not open is an error.
func openBackend(cfg config.Config) (Backend, error) {
	ok, err := vault.Exists(cfg.Vault)
	if err != nil || !ok {
		return nil, err
	}
	v, err := vault.Open(cfg.Vault)
	if err != nil {
		return nil, err
	}
	return opsBackend{v: v, agents: cfg.Agents}, nil
}

// model is the UI for cfg on backend b, which is nil when the vault is missing.
// Without a config file it opens with the first-launch prompt (see offerConfig),
// and the vault prompt follows it, or opens alone when only the vault is missing.
func model(cfg config.Config, b Backend, th theme.Theme, set icons.Set) Model {
	m := New(b, cfg.Agents, th, set, os.Getenv("NO_COLOR") != "")
	m.home, _ = os.UserHomeDir()
	if b == nil {
		m.noVault = cfg.Vault
	}
	switch {
	case !cfg.FileExists:
		m = m.offerConfig(cfg.Path)
	case b == nil:
		m = m.offerVault()
	}
	return m
}
