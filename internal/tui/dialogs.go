package tui

import (
	"fmt"
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

// opExport, opDiscard, opScaffold and opEdit are the actions the dialogs add to the
// ops ones. An export writes a file, a discard drops the text of NEW, a scaffold writes
// the template of a skill from the wizard, and an edit opens the scaffolded file in the
// editor. Only a scaffold is an ops.Action.
const (
	opExport   = "export"
	opDiscard  = "discard"
	opScaffold = "scaffold"
	opEdit     = "edit"
)

// exportNameMsg carries the default file name of an export.
type exportNameMsg struct {
	name string
	err  error
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

// sizeInputs fits the editor and the path inputs to their block. The editor is sized
// for the paste mode of NEW, whatever is open when this runs, since its bar sets the
// rows left for it: the title band and the check line take four rows. The path inputs
// draw their cursor one column past their width, so that column is left out.
func (m *Model) sizeInputs() {
	m.area.SetWidth(m.blockWidth())
	paste := *m
	paste.mode, paste.newPaste = modeNew, true
	m.area.SetHeight(max(1, paste.bodyHeight()-4))
	for _, in := range []*textinput.Model{&m.path, &m.fixup.name, &m.fixup.desc, &m.wiz.name, &m.wiz.desc} {
		in.Width = max(1, m.blockWidth()-lipgloss.Width(in.Prompt)-1)
	}
}

// openAdopt opens the ADOPT picker on every agent, with its cursor on the first copy
// and nothing marked.
func (m Model) openAdopt() Model {
	m.mode = modeAdopt
	m.pick, m.pickTop, m.pickFilter = 0, 0, 0
	m.pickMarks = map[cell]bool{}
	return m
}

// pickCopies lists the copies the picker shows, in display order: grouped by agent in
// agent order, and limited to the agent the filter names.
func (m Model) pickCopies() []ops.UnmanagedCopy {
	var out []ops.UnmanagedCopy
	for i, a := range m.agents {
		if m.pickFilter != 0 && m.pickFilter != i+1 {
			continue
		}
		for _, c := range m.copies {
			if c.Agent == a.ID {
				out = append(out, c)
			}
		}
	}
	return out
}

// pickLine is a line of the picker: the heading of an agent, or the copy at index copy
// of pickCopies. A heading has copy -1.
type pickLine struct {
	agent string
	copy  int
}

// pickLines lays the copies out under a heading per agent, in display order.
func pickLines(copies []ops.UnmanagedCopy) []pickLine {
	var lines []pickLine
	for i, c := range copies {
		if i == 0 || copies[i-1].Agent != c.Agent {
			lines = append(lines, pickLine{agent: c.Agent, copy: -1})
		}
		lines = append(lines, pickLine{copy: i})
	}
	return lines
}

// pickedCopies are the copies enter adopts: the marked ones, in the order of the
// loaded copies, or the cursor's copy when none is marked.
func (m Model) pickedCopies() []ops.UnmanagedCopy {
	copies := m.pickCopies()
	if len(m.pickMarks) == 0 {
		if len(copies) == 0 {
			return nil
		}
		return []ops.UnmanagedCopy{copies[clampIndex(m.pick, len(copies))]}
	}
	var out []ops.UnmanagedCopy
	for _, c := range m.copies {
		if m.pickMarks[cell{c.Skill, c.Agent}] {
			out = append(out, c)
		}
	}
	return out
}

// adoptGroups groups the copies by agent, in agent order, into the adopts of an action.
func adoptGroups(agents []config.Agent, copies []ops.UnmanagedCopy) []adoptGroup {
	var groups []adoptGroup
	for _, a := range agents {
		var skills []string
		for _, c := range copies {
			if c.Agent == a.ID {
				skills = append(skills, c.Skill)
			}
		}
		if len(skills) > 0 {
			groups = append(groups, adoptGroup{from: a, skills: skills})
		}
	}
	return groups
}

// adoptNames are the distinct skill names of an adopt, in the order of its groups.
func adoptNames(act action) []string {
	var names []string
	for _, g := range act.adopts {
		for _, s := range g.skills {
			if !slices.Contains(names, s) {
				names = append(names, s)
			}
		}
	}
	return names
}

// adoptCount is how many skills an adopt takes.
func adoptCount(act action) int {
	return len(adoptNames(act))
}

// adoptPicked plans the adoption of the picked copies. The plan applies at once,
// unless ops refuses for want of --force.
func (m Model) adoptPicked() (Model, tea.Cmd) {
	picked := m.pickedCopies()
	if len(picked) == 0 {
		return m, nil
	}
	act := action{op: ops.OpAdopt, back: modeAdopt, adopts: adoptGroups(m.agents, picked)}
	return m, planCmd(m.backend, act)
}

// togglePick marks or unmarks copy c. Only one copy of a name can be marked, since an
// adopt takes one folder per name, so a copy whose name another marked copy has is
// refused with an error.
func (m Model) togglePick(c ops.UnmanagedCopy) Model {
	id := cell{c.Skill, c.Agent}
	if m.pickMarks[id] {
		delete(m.pickMarks, id)
		return m
	}
	for other := range m.pickMarks {
		if other.skill == c.Skill {
			m.notify(noteError, "error", fmt.Sprintf("%s is marked in %s already; mark one copy per name", c.Skill, other.agent))
			return m
		}
	}
	m.pickMarks[id] = true
	return m
}

// setPickFilter shows the agents of filter f, with the cursor on the first copy and
// nothing marked, so no mark is left on a copy the picker no longer shows.
func (m *Model) setPickFilter(f int) {
	m.pickFilter, m.pick, m.pickTop = f, 0, 0
	m.pickMarks = map[cell]bool{}
}

// adoptKey moves the picker cursor, switches the agent filter with tab, shift+tab or
// the arrows, marks the copy under the cursor with space, and adopts on enter. esc
// closes the picker. The picker stays open after an adopt, so the copies left can be
// adopted too.
func (m Model) adoptKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	copies := m.pickCopies()
	filters := len(m.agents) + 1
	switch {
	case key.Matches(msg, m.keys.up):
		m.pick = clampIndex(m.pick-1, len(copies))
	case key.Matches(msg, m.keys.down):
		m.pick = clampIndex(m.pick+1, len(copies))
	case key.Matches(msg, m.keys.agentNext):
		m.setPickFilter((m.pickFilter + 1) % filters)
	case key.Matches(msg, m.keys.agentPrev):
		m.setPickFilter((m.pickFilter + filters - 1) % filters)
	case key.Matches(msg, m.keys.mark):
		if len(copies) > 0 {
			m = m.togglePick(copies[clampIndex(m.pick, len(copies))])
		}
	case key.Matches(msg, m.keys.enter):
		return m.adoptPicked()
	case key.Matches(msg, m.keys.esc):
		m.mode = modeNormal
	}
	m.scrollPick()
	return m, nil
}

// pickRows is how many picker lines fit under the title band, the filter row and their
// gap. Before the first size message it is every line.
func (m Model) pickRows() int {
	if m.height == 0 {
		return max(1, len(pickLines(m.pickCopies())))
	}
	return max(1, m.bodyHeight()-4)
}

// scrollPick moves the window of picker lines so the cursor's copy is shown.
func (m *Model) scrollPick() {
	lines := pickLines(m.pickCopies())
	cursor := 0
	for i, l := range lines {
		if l.copy == m.pick {
			cursor = i
		}
	}
	scrollWindow(&m.pickTop, cursor, m.pickRows())
}

// The rows of ADD, top to bottom. Adopt is the last row.
const (
	rowCreate = iota
	rowImport
	rowAdopt
)

// openAdd opens ADD with its cursor on Create.
func (m Model) openAdd() Model {
	m.mode = modeAdd
	m.addCur = rowCreate
	return m
}

// adoptable reports whether ADOPT has a copy to show. The Adopt row of ADD is dimmed and
// skipped while it has none.
func (m Model) adoptable() bool {
	return len(m.copies) > 0
}

// addKey moves the cursor over the rows of ADD, skipping Adopt while it is not
// adoptable. enter opens the row under the cursor, and c, i and a open Create, Import
// and Adopt. esc returns to the main screen.
func (m Model) addKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	k := m.keys
	last := rowImport
	if m.adoptable() {
		last = rowAdopt
	}
	switch {
	case key.Matches(msg, k.up):
		m.addCur = max(0, m.addCur-1)
	case key.Matches(msg, k.down):
		m.addCur = min(m.addCur+1, last)
	case key.Matches(msg, k.enter):
		return m.addOpen(min(m.addCur, last))
	case key.Matches(msg, k.addCreate):
		return m.addOpen(rowCreate)
	case key.Matches(msg, k.addImport):
		return m.addOpen(rowImport)
	case key.Matches(msg, k.adopt):
		return m.addOpen(rowAdopt)
	case key.Matches(msg, k.esc):
		m.mode = modeNormal
	}
	return m, nil
}

// addOpen opens the screen of row. Adopt does nothing while there is no unmanaged skill.
func (m Model) addOpen(row int) (Model, tea.Cmd) {
	switch row {
	case rowCreate:
		return m.openNew()
	case rowImport:
		return m.openImport()
	}
	if !m.adoptable() {
		return m, nil
	}
	return m.openAdopt(), nil
}

// openNew opens NEW on the wizard, empty, and clears the paste mode's text.
func (m Model) openNew() (Model, tea.Cmd) {
	m.mode = modeNew
	m.newPaste = false
	m.area.Reset()
	return m.openWizard()
}

// newKey routes a key to the wizard, or to the paste mode's editor. In the paste mode
// ctrl+s saves the text as a skill, tab goes back to the wizard, and esc closes NEW as
// the wizard does.
func (m Model) newKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if !m.newPaste {
		return m.wizardKey(msg)
	}
	switch {
	case key.Matches(msg, m.keys.save):
		return m.startCreate()
	case key.Matches(msg, m.keys.tab):
		return m.switchNew()
	case key.Matches(msg, m.keys.esc):
		return m.escNew()
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

// themeRows is how many theme rows fit under the title band. Before the
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
