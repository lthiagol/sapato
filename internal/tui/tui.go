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
// reads keys from in and draws to out. The grid shows the enabled agents,
// painted with cfg.UI's theme and icon set. NO_COLOR, when set to a non-empty
// value, turns colors off.
func Run(cfg config.Config, in io.Reader, out io.Writer) error {
	th, ok := theme.ByName(cfg.UI.Theme)
	if !ok {
		return fmt.Errorf("unknown theme %q", cfg.UI.Theme)
	}
	set, ok := icons.ByName(cfg.UI.Icons)
	if !ok {
		return fmt.Errorf("unknown icon set %q", cfg.UI.Icons)
	}
	v, err := vault.Open(cfg.Vault)
	if err != nil {
		return err
	}
	var agents []config.Agent
	for _, a := range cfg.Agents {
		if a.Enabled {
			agents = append(agents, a)
		}
	}
	m := New(opsBackend{v}, agents, th, set, os.Getenv("NO_COLOR") != "")
	p := tea.NewProgram(m, tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
