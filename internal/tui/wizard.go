package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"sapato/internal/ops"
	"sapato/internal/vault"
)

// The NEW wizard asks for a name, a description and the agents the skill works with
// (spec §9.4). ctrl+s scaffolds the skill from the template (spec §7.2), then offers
// to open it in the editor. tab switches to the paste mode, and each mode keeps the
// text typed in it.

// newWizard is the NEW wizard: the name and description inputs and the agents the
// skill works with, by configured agent index. focus is the field that takes keys:
// 0 the name, 1 the description, and 2 and up the agents in order.
type newWizard struct {
	name, desc textinput.Model
	checked    []bool
	focus      int
}

// editDoneMsg reports that the editor opened from the NEW wizard has exited.
type editDoneMsg struct {
	err error
}

// openWizard resets the wizard to an empty skill with every agent checked and the
// name focused.
func (m Model) openWizard() (Model, tea.Cmd) {
	checked := make([]bool, len(m.agents))
	for i := range checked {
		checked[i] = true
	}
	m.wiz = newWizard{name: newPathInput(), desc: newPathInput(), checked: checked}
	m.sizeInputs()
	cmd := m.focusWizard(0)
	return m, cmd
}

// focusWizard moves the wizard's focus to field i, clamped to the fields, and gives
// focus to its input. It returns the command of that input.
func (m *Model) focusWizard(i int) tea.Cmd {
	z := &m.wiz
	z.focus = clampIndex(i, 2+len(z.checked))
	z.name.Blur()
	z.desc.Blur()
	switch z.focus {
	case 0:
		return z.name.Focus()
	case 1:
		return z.desc.Focus()
	}
	return nil
}

// switchNew flips NEW between the wizard and the paste mode. Each keeps its own text.
func (m Model) switchNew() (Model, tea.Cmd) {
	if m.newPaste {
		m.newPaste = false
		m.area.Blur()
		cmd := m.focusWizard(m.wiz.focus)
		return m, cmd
	}
	m.newPaste = true
	m.wiz.name.Blur()
	m.wiz.desc.Blur()
	return m, m.area.Focus()
}

// wizardKey types into the wizard. ctrl+s scaffolds the skill, tab switches to the
// paste mode, and esc closes NEW. The arrows move between fields, enter moves down,
// and space toggles the agent under the cursor. Other keys go to the name or the
// description.
func (m Model) wizardKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.save):
		return m.startScaffold()
	case key.Matches(msg, m.keys.tab):
		return m.switchNew()
	case key.Matches(msg, m.keys.esc):
		return m.escNew()
	case key.Matches(msg, m.keys.fieldUp):
		cmd := m.focusWizard(m.wiz.focus - 1)
		return m, cmd
	case key.Matches(msg, m.keys.fieldDown), key.Matches(msg, m.keys.enter):
		cmd := m.focusWizard(m.wiz.focus + 1)
		return m, cmd
	}
	if z := &m.wiz; z.focus >= 2 {
		if key.Matches(msg, m.keys.mark) {
			z.checked[z.focus-2] = !z.checked[z.focus-2]
		}
		return m, nil
	}
	var cmd tea.Cmd
	in := &m.wiz.name
	if m.wiz.focus == 1 {
		in = &m.wiz.desc
	}
	*in, cmd = in.Update(msg)
	return m, cmd
}

// escNew closes NEW. Text typed in either mode is a confirm first.
func (m Model) escNew() (Model, tea.Cmd) {
	if !m.newTyped() {
		m.mode = modeNormal
		return m, nil
	}
	m.pending = pending{act: action{op: opDiscard, back: modeNew}}
	m.mode = modeConfirm
	return m, nil
}

// newTyped reports whether NEW holds text in either mode.
func (m Model) newTyped() bool {
	return strings.TrimSpace(m.area.Value()) != "" ||
		strings.TrimSpace(m.wiz.name.Value()) != "" ||
		strings.TrimSpace(m.wiz.desc.Value()) != ""
}

// pickedAgents are the IDs of the checked agents, in agent order.
func (m Model) pickedAgents() []string {
	var picked []string
	for i, a := range m.agents {
		if m.wiz.checked[i] {
			picked = append(picked, a.ID)
		}
	}
	return picked
}

// startScaffold plans the scaffold of the wizard's skill. Every agent checked means no
// compatible list, as the compat table is then not written (spec §3). At least one
// agent must be checked.
func (m Model) startScaffold() (Model, tea.Cmd) {
	picked := m.pickedAgents()
	if len(picked) == 0 {
		m.notify(noteError, "error", "pick at least one agent")
		return m, nil
	}
	act := action{
		op:     opScaffold,
		skill:  strings.TrimSpace(m.wiz.name.Value()),
		desc:   strings.TrimSpace(m.wiz.desc.Value()),
		compat: ops.NormalizeCompat(picked, agentNames(m.agents)),
		back:   modeNew,
	}
	return m, planCmd(m.backend, act)
}

// nameCheck says what the wizard's name is: a hint while it is empty, the rule it
// breaks, the vault skill it would replace, or that it is valid.
func (m Model) nameCheck() (string, lipgloss.Style) {
	name := strings.TrimSpace(m.wiz.name.Value())
	switch {
	case name == "":
		return "lowercase letters, digits and dashes", m.styles.dim
	case !vault.ValidName(name):
		return "invalid: lowercase letters, digits and dashes, at most 64", m.styles.fail
	case m.inVault(name):
		return "in the vault: save asks to replace it", m.styles.warn
	}
	return "valid: " + name, m.styles.glyph[ops.Synced]
}

// descCheck counts the wizard's description against its limit. It is required, and
// the count turns red when it is empty or too long.
func (m Model) descCheck() (string, lipgloss.Style) {
	n := utf8.RuneCountInString(strings.TrimSpace(m.wiz.desc.Value()))
	text := fmt.Sprintf("%d/%d", n, vault.MaxDescription)
	switch {
	case n == 0:
		return text + " · required", m.styles.fail
	case n > vault.MaxDescription:
		return text + " · too long", m.styles.fail
	}
	return text, m.styles.dim
}

// editorName is the editor command the confirm names: the command line without the
// file.
func (m Model) editorName(path string) string {
	argv := m.editArgv(path)
	return strings.Join(argv[:len(argv)-1], " ")
}

// onEdited refreshes the grid once the editor has exited. An editor that fails says so.
func (m Model) onEdited(msg editDoneMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.notify(noteError, "error", "editor: "+msg.err.Error())
	}
	return m, m.loadCmd(false)
}

// execEditor runs argv in the terminal, which the editor takes over until it exits.
func execEditor(argv []string) tea.Cmd {
	cmd := exec.Command(argv[0], argv[1:]...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return editDoneMsg{err: err} })
}
