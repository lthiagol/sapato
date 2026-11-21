package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"sapato/internal/vault"
)

// opConfigInit is the action of the first-launch prompt that writes the starter
// config. It is not an ops.Action.
const opConfigInit = "config-init"

// opVaultInit is the action of the prompt that creates the missing vault. It is
// not an ops.Action.
const opVaultInit = "vault-init"

// offerConfig opens the first-launch prompt, a plain confirm that offers to write
// the starter config at path. y writes it; n keeps the defaults for this run.
func (m Model) offerConfig(path string) Model {
	m.pending = pending{act: action{op: opConfigInit, path: path}}
	m.mode = modeConfirm
	return m
}

// configKey answers the first-launch prompt. The vault prompt follows when the
// vault is missing, whatever was answered here.
func (m Model) configKey(msg tea.KeyMsg, act action) Model {
	if key.Matches(msg, m.keys.yes) {
		m = m.writeConfig(act.path)
	} else {
		m.notify(noteInfo, "cancelled", subject(act))
	}
	if m.noVault != "" {
		m = m.offerVault()
	}
	return m
}

// writeConfig writes the starter config at path for the first-launch prompt, and
// notifies the result.
func (m Model) writeConfig(path string) Model {
	if err := WriteStarterConfig(path, false); err != nil {
		m.notify(noteError, "error", err.Error())
		return m
	}
	m.notify(noteSuccess, "config created", path)
	return m
}

// offerVault opens the vault prompt, a plain confirm that offers to create the
// vault at noVault, as sapato init does. The grid has nothing to show until it exists.
func (m Model) offerVault() Model {
	m.pending = pending{act: action{op: opVaultInit, path: m.noVault}}
	m.mode = modeConfirm
	return m
}

// vaultKey answers the vault prompt. y creates the vault and opens the grid on it.
// Any other key exits, so Run reports the missing vault. A key that leaves the
// prompt up is no use here: no vault is open for it to act on.
func (m Model) vaultKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if !key.Matches(msg, m.keys.yes) {
		return m, tea.Quit
	}
	return m.createVault()
}

// createVault runs the code of sapato init on the missing vault, then opens it.
// A failure is shown and the prompt stays up.
func (m Model) createVault() (Model, tea.Cmd) {
	path := m.noVault
	if err := vault.Init(path); err != nil {
		m.notify(noteError, "error", err.Error())
		return m, nil
	}
	v, err := vault.Open(path)
	if err != nil {
		m.notify(noteError, "error", err.Error())
		return m, nil
	}
	m.backend = opsBackend{v: v, agents: m.agents}
	m.noVault = ""
	m.pending = pending{}
	m.mode = modeNormal
	m.notify(noteSuccess, "vault created", path)
	return m, m.loadCmd(false)
}
