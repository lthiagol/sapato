package tui

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"sapato/internal/config"
	"sapato/internal/ops"
	"sapato/internal/tui/theme"
	"sapato/internal/vault"
)

// The dialogs are the overlay modes that take input: ADOPT, NEW, IMPORT, EXPORT
// and THEME. Their work goes through the same plan, confirm and apply path as the
// screen keys, so a refusal that --force overrides gets the force prompt.

// opExport and opDiscard are the actions the dialogs add to the ops ones. An export
// writes a file, and a discard drops the text of the NEW editor. Neither is an
// ops.Action.
const (
	opExport  = "export"
	opDiscard = "discard"
)

// exportNameMsg carries the default file name of an export.
type exportNameMsg struct {
	name string
	err  error
}

// unmanagedRow is one unmanaged copy that the ADOPT picker lists.
type unmanagedRow struct {
	skill string
	agent config.Agent
	path  string
}

// newEditor returns the NEW editor: no prompt, no line numbers, no length limit,
// and a static cursor, so the model needs no blink commands.
func newEditor() textarea.Model {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.Cursor.SetMode(cursor.CursorStatic)
	return ta
}

// newPathInput returns the input of the IMPORT and EXPORT prompts.
func newPathInput() textinput.Model {
	p := textinput.New()
	p.Prompt = "> "
	p.Cursor.SetMode(cursor.CursorStatic)
	return p
}

// sizeInputs fits the editor and the path inputs to their dialog block. The path
// inputs draw their cursor one column past their width, so that column is left out.
func (m *Model) sizeInputs() {
	m.area.SetWidth(m.inputWidth())
	m.area.SetHeight(max(1, m.bodyHeight()-2))
	for _, in := range []*textinput.Model{&m.path, &m.fixup.name, &m.fixup.desc} {
		in.Width = max(1, m.inputWidth()-lipgloss.Width(in.Prompt)-1)
	}
}

// openAdopt opens the ADOPT picker with its cursor on the first copy.
func (m Model) openAdopt() Model {
	m.mode = modeAdopt
	m.pick, m.pickTop = 0, 0
	return m
}

// unmanaged lists the unmanaged copies in the configured agents, by skill name and
// then agent order. It lists every name, whatever the grid filter is.
func (m Model) unmanaged() []unmanagedRow {
	var rows []unmanagedRow
	for _, name := range m.names {
		for _, a := range m.agents {
			if m.cells[cell{name, a.ID}] == ops.Unmanaged {
				rows = append(rows, unmanagedRow{skill: name, agent: a, path: filepath.Join(a.Path, name)})
			}
		}
	}
	return rows
}

// adoptKey moves the picker cursor, adopts the copy under it on enter, and closes
// the picker on esc. The picker stays open after an adopt, so the copies left can
// be adopted too.
func (m Model) adoptKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	rows := m.unmanaged()
	switch {
	case key.Matches(msg, m.keys.up):
		m.pick = clampIndex(m.pick-1, len(rows))
	case key.Matches(msg, m.keys.down):
		m.pick = clampIndex(m.pick+1, len(rows))
	case key.Matches(msg, m.keys.enter):
		if len(rows) == 0 {
			return m, nil
		}
		r := rows[clampIndex(m.pick, len(rows))]
		return m, planCmd(m.backend, action{op: ops.OpAdopt, skill: r.skill, agents: []config.Agent{r.agent}, back: modeAdopt})
	case key.Matches(msg, m.keys.esc):
		m.mode = modeNormal
	}
	m.scrollPick()
	return m, nil
}

// pickRows is how many picker rows fit under the title and its gap. Before the
// first size message it is every row.
func (m Model) pickRows() int {
	if m.height == 0 {
		return max(1, len(m.unmanaged()))
	}
	return max(1, m.bodyHeight()-2)
}

// scrollPick moves the window of picker rows so the cursor row is shown.
func (m *Model) scrollPick() {
	scrollWindow(&m.pickTop, m.pick, m.pickRows())
}

// openNew opens the NEW editor, empty.
func (m Model) openNew() (Model, tea.Cmd) {
	m.mode = modeNew
	m.area.Reset()
	return m, m.area.Focus()
}

// newKey types into the editor. ctrl+s saves its text as a skill. esc closes the
// editor, after a confirm when the text is not empty.
func (m Model) newKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.save):
		return m.startCreate()
	case key.Matches(msg, m.keys.esc):
		if strings.TrimSpace(m.area.Value()) == "" {
			m.mode = modeNormal
			return m, nil
		}
		m.pending = pending{act: action{op: opDiscard, back: modeNew}}
		m.mode = modeConfirm
		return m, nil
	}
	var cmd tea.Cmd
	m.area, cmd = m.area.Update(msg)
	return m, cmd
}

// startCreate plans the save of the editor's text. The skill takes the name in
// its frontmatter.
func (m Model) startCreate() (Model, tea.Cmd) {
	content := []byte(m.area.Value())
	act := action{op: ops.OpCreate, skill: vault.ParseContent(content).Name, content: content, back: modeNew}
	return m, planCmd(m.backend, act)
}

// openImport opens the IMPORT prompt, empty.
func (m Model) openImport() (Model, tea.Cmd) {
	m.mode = modeImport
	m.path.SetValue("")
	return m, m.path.Focus()
}

// openExport asks for the default file name of the open skill. The EXPORT prompt
// opens on that name.
func (m Model) openExport() (Model, tea.Cmd) {
	if !m.inVault(m.open) {
		m.notify(noteError, "error", m.open+" is not in the vault")
		return m, nil
	}
	return m, exportNameCmd(m.backend, m.open)
}

// exportNameCmd asks the Backend for the default file name of an export of skill.
func exportNameCmd(b Backend, skill string) tea.Cmd {
	return func() tea.Msg {
		name, err := b.ExportName(skill)
		return exportNameMsg{name: name, err: err}
	}
}

// onExportName opens the EXPORT prompt on the default name, or says why the skill
// cannot be exported.
func (m Model) onExportName(msg exportNameMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.notify(noteError, "error", msg.err.Error())
		return m, nil
	}
	m.mode = modeExport
	m.path.SetValue(msg.name)
	m.path.CursorEnd()
	return m, m.path.Focus()
}

// pathKey types into the IMPORT or EXPORT prompt. enter runs it, and esc closes it.
// ctrl+o opens the IMPORT file picker.
func (m Model) pathKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.enter):
		if m.mode == modeImport {
			return m.startImport()
		}
		return m.runExport()
	case key.Matches(msg, m.keys.openFile) && m.mode == modeImport:
		return m.openBrowse(), nil
	case key.Matches(msg, m.keys.esc):
		m.mode = modeNormal
		return m, nil
	}
	var cmd tea.Cmd
	m.path, cmd = m.path.Update(msg)
	return m, cmd
}

// runExport writes the open skill to the typed path. Export has no dry run and no
// force: an existing file is refused.
func (m Model) runExport() (Model, tea.Cmd) {
	dest := strings.TrimSpace(m.path.Value())
	if dest == "" {
		m.notify(noteError, "error", "export needs a path")
		return m, nil
	}
	return m, applyCmd(m.backend, action{op: opExport, skill: m.open, path: dest})
}

// startRemove plans the removal of the open skill from the vault.
func (m Model) startRemove() (Model, tea.Cmd) {
	if !m.inVault(m.open) {
		m.notify(noteError, "error", m.open+" is not in the vault")
		return m, nil
	}
	return m, planCmd(m.backend, action{op: ops.OpRemove, skill: m.open})
}

// openTheme opens the THEME picker with its cursor on the theme in use. Moving the
// cursor previews a theme. esc restores the theme that was in use.
func (m Model) openTheme() Model {
	m.mode = modeTheme
	m.themePrev = m.th
	m.themeCur = max(0, slices.IndexFunc(theme.All(), func(t theme.Theme) bool { return t.Name == m.th.Name }))
	m.themeTop = 0
	m.scrollTheme()
	return m
}

// themeKey moves the cursor, which previews the theme under it. enter keeps the
// preview for this session. esc restores the theme that was in use. The config
// file is never written.
func (m Model) themeKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	all := theme.All()
	switch {
	case key.Matches(msg, m.keys.up):
		m.themeCur = clampIndex(m.themeCur-1, len(all))
		m.setTheme(all[m.themeCur])
	case key.Matches(msg, m.keys.down):
		m.themeCur = clampIndex(m.themeCur+1, len(all))
		m.setTheme(all[m.themeCur])
	case key.Matches(msg, m.keys.enter):
		m.mode = modeNormal
		m.notify(noteInfo, "theme", m.th.Name)
	case key.Matches(msg, m.keys.esc):
		m.setTheme(m.themePrev)
		m.mode = modeNormal
	}
	m.scrollTheme()
	return m, nil
}

// setTheme paints the screen in th from now on.
func (m *Model) setTheme(th theme.Theme) {
	m.th = th
	m.styles = newStyles(th, m.styles.noColor)
}

// themeRows is how many theme rows fit under the title and its gap. Before the
// first size message it is every theme.
func (m Model) themeRows() int {
	if m.height == 0 {
		return len(theme.All())
	}
	return max(1, m.bodyHeight()-2)
}

// scrollTheme moves the window of theme rows so the cursor row is shown.
func (m *Model) scrollTheme() {
	scrollWindow(&m.themeTop, m.themeCur, m.themeRows())
}
