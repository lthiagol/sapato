package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"sapato/internal/ops"
	"sapato/internal/tui/theme"
	"sapato/internal/vault"
)

const (
	// dialogMinW is the least width of a dialog block (spec §9.4).
	dialogMinW = 60
	// confirmMinW is the least width of a confirm box (spec §9.4).
	confirmMinW = 40
)

// screenBody is a non-main screen (spec §9.4): the title band under the header rule,
// then the content as one centered block of blockWidth. The band is the title centered
// in bold and a full-width rule. build renders the lines for the block's width, so a
// long value is cut to fit.
func (m Model) screenBody(title string, build func(w int) []string) []string {
	blockW := m.blockWidth()
	band := []string{center(m.styles.bold.Render(truncate(title, m.layoutWidth())), m.width), m.rule()}
	return append(band, m.centered(build(blockW), blockW)...)
}

// confirmModal is a confirm over the screen it came from (spec §9.4): that screen,
// stripped of its styles and drawn in Overlay, with the confirm box centered over
// it. Before the terminal reports its size there is no screen to draw over, so the
// box alone is returned.
func (m Model) confirmModal() []string {
	box, boxW := m.confirmBox()
	if m.height == 0 {
		return m.centered(box, boxW)
	}
	under := m
	under.mode = m.pending.act.back
	lines := fitRows(under.body(), m.bodyHeight())
	top := max(0, (len(lines)-len(box))/2)
	x := max(0, (m.width-boxW)/2)
	out := make([]string, len(lines))
	for i, l := range lines {
		plain := ansi.Strip(l)
		if i < top || i >= top+len(box) {
			out[i] = m.styles.dim.Render(plain)
			continue
		}
		out[i] = m.styles.dim.Render(pad(ansi.Truncate(plain, x, ""), x)) +
			box[i-top] +
			m.styles.dim.Render(ansi.TruncateLeft(plain, x+boxW, ""))
	}
	return out
}

// confirmBox draws the confirm box: a rounded border around the confirm's body and
// its answer hints. The border carries the action as its title, in Red for a force
// prompt and Accent otherwise. The box is as wide as its widest line, at least
// confirmMinW and at most the terminal less four columns. It returns the rows and
// their width.
func (m Model) confirmBox() ([]string, int) {
	maxW := m.layoutWidth() - 4
	boxW := min(maxW, max(confirmMinW, maxWidth(m.confirmLines(maxW))+4))
	inner := boxW - 4
	st := m.confirmBorder()
	title := truncate(" "+confirmTitle(m.pending.act)+" ", boxW-5)
	rows := []string{st.Render("╭─") + st.Bold(true).Render(title) +
		st.Render(strings.Repeat("─", boxW-3-lipgloss.Width(title))+"╮")}
	for _, l := range m.confirmLines(inner) {
		rows = append(rows, st.Render("│")+" "+pad(l, inner)+" "+st.Render("│"))
	}
	return append(rows, st.Render("╰"+strings.Repeat("─", boxW-2)+"╯")), boxW
}

// confirmBorder is the style of a confirm box's border: Red for a force prompt,
// Accent for a plain one.
func (m Model) confirmBorder() lipgloss.Style {
	if m.pending.act.force {
		return m.styles.fail
	}
	return m.styles.focus
}

// confirmLines are the rows inside the confirm box at w columns: the confirm's body,
// then the answer hints.
func (m Model) confirmLines(w int) []string {
	return append(m.confirmBody(w), "", m.hintText([]key.Binding{m.keys.yes, m.keys.no}))
}

// confirmTitle is the action a confirm box names in its border.
func confirmTitle(a action) string {
	switch {
	case a.op == opDiscard:
		return "Discard"
	case a.op == opConfigInit:
		return "Create config"
	case a.op == opVaultInit:
		return "Create vault"
	case a.op == opEdit:
		return "Open in editor"
	case a.op == opScaffold && a.force:
		return "Force create"
	case a.force:
		return "Force " + a.op
	}
	return capitalize(a.op)
}

// blockWidth is the width of the content block of every titled screen and step: two
// thirds of the terminal, at least dialogMinW, never wider than the terminal.
func (m Model) blockWidth() int {
	return min(m.layoutWidth(), max(dialogMinW, m.layoutWidth()*2/3))
}

// centered lays the lines in a block blockW wide, centered in the terminal. Each
// line is padded to the block, so the block is as wide as it says, not only as wide
// as its longest text.
func (m Model) centered(lines []string, blockW int) []string {
	indent := m.leftPad(blockW)
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = indent + pad(l, blockW)
	}
	return out
}

// addChoice is a row of ADD: its key, its name and what it does. off dims the row.
type addChoice struct {
	key, name, desc string
	off             bool
}

// addBody is ADD: the three choices, one row each, left-aligned in the block like every
// titled screen. The Adopt row is dimmed and says there is nothing to adopt while no
// unmanaged skill is listed.
func (m Model) addBody(w int) []string {
	adopt := addChoice{key: "a", name: "Adopt", desc: fmt.Sprintf("%d unmanaged skills in your agents", len(m.copies))}
	switch len(m.copies) {
	case 0:
		adopt.desc, adopt.off = "no unmanaged skills", true
	case 1:
		adopt.desc = "1 unmanaged skill in your agents"
	}
	choices := []addChoice{
		{key: "c", name: "Create", desc: "write a new skill (wizard)"},
		{key: "i", name: "Import", desc: "from a folder, file, git or URL"},
		adopt,
	}
	rows := make([]string, len(choices))
	for i, c := range choices {
		rows[i] = ansi.Truncate(m.addRow(c, i == m.addCur), w, "…")
	}
	return rows
}

// addRow is one choice of ADD: the cursor marker, the key, the name and the description.
// The name lines up across the rows. An off row is dimmed and has no marker.
func (m Model) addRow(c addChoice, selected bool) string {
	if c.off {
		return m.styles.dim.Render("  " + c.key + "  " + pad(c.name, 9) + c.desc)
	}
	marker, nameStyle := "  ", m.styles.text
	if selected {
		marker, nameStyle = pad(m.icons.Marker, 2), m.styles.bold
	}
	return m.on(selected, m.styles.accent).Render(marker) +
		m.on(selected, m.styles.hintKey).Render(c.key) +
		m.spaces(2, selected) +
		m.on(selected, nameStyle).Render(pad(c.name, 9)) +
		m.on(selected, m.styles.subtext).Render(c.desc)
}

// adoptBody is the ADOPT picker: a filter row over the agents, then a heading per
// agent with its unmanaged copies. Each copy shows its mark, its name, a conflict tag
// when its name is unmanaged with different hashes, and its folder. The lines are a
// window that follows the cursor.
func (m Model) adoptBody(w int) []string {
	copies := m.pickCopies()
	lines := []string{m.filterRow(w), ""}
	if len(copies) == 0 {
		return append(lines, m.styles.dim.Render(truncate("No unmanaged skills in the configured agents.", w)))
	}
	nameW := 0
	for _, c := range copies {
		nameW = max(nameW, lipgloss.Width(c.Skill))
	}
	nameW = min(nameW, max(5, w/2))
	layout := pickLines(copies)
	end := min(len(layout), min(m.pickTop, len(layout))+m.pickRows())
	for _, l := range layout[min(m.pickTop, len(layout)):end] {
		if l.copy < 0 {
			lines = append(lines, m.styles.bold.Render(truncate(l.agent, w)))
			continue
		}
		lines = append(lines, m.copyRow(copies[l.copy], l.copy == m.pick, nameW, w))
	}
	return lines
}

// copyRow is one copy of the ADOPT picker, cut to w: its markers, its name, a conflict
// tag when it has one, then its folder.
func (m Model) copyRow(c ops.UnmanagedCopy, selected bool, nameW, w int) string {
	marker, nameStyle := "  ", lipgloss.NewStyle()
	if selected {
		marker, nameStyle = pad(m.icons.Marker, 2), m.styles.bold
	}
	mark := "  "
	if m.pickMarks[cell{c.Skill, c.Agent}] {
		mark = pad(m.icons.Mark, 2)
	}
	head := m.on(selected, m.styles.accent).Render(marker) +
		m.on(selected, m.styles.accent).Render(mark) +
		m.on(selected, nameStyle).Render(pad(truncate(c.Skill, nameW), nameW)) +
		m.spaces(2, selected)
	if c.Conflict {
		head += m.on(selected, m.styles.warn).Render("conflict") + m.spaces(2, selected)
	}
	room := w - lipgloss.Width(head)
	return head + m.on(selected, m.styles.dim).Render(truncate(c.Path, room))
}

// filterRow lists the agents the ADOPT filter switches between, All first, and marks
// the one it shows. A row too wide for w is cut to plain text.
func (m Model) filterRow(w int) string {
	names := []string{"All"}
	for _, a := range m.agents {
		names = append(names, a.ID)
	}
	sep := m.styles.dim.Render(" · ")
	parts := make([]string, len(names))
	for i, n := range names {
		if i == m.pickFilter {
			parts[i] = m.styles.hintKey.Render(n)
		} else {
			parts[i] = m.styles.dim.Render(n)
		}
	}
	row := strings.Join(parts, sep)
	if lipgloss.Width(row) > w {
		return m.styles.dim.Render(truncate(strings.Join(names, " · "), w))
	}
	return row
}

// newBody is NEW: the wizard, or the paste mode's editor with its live check under it.
func (m Model) newBody(w int) []string {
	if !m.newPaste {
		return m.wizardBody(w)
	}
	text, style := m.newCheck()
	return append(strings.Split(m.area.View(), "\n"), "", style.Render(truncate(text, w)))
}

// newCheck says what the editor's text is: a hint while it is empty, the name of
// the skill when it is valid, or the first problem.
func (m Model) newCheck() (string, lipgloss.Style) {
	text := m.area.Value()
	if strings.TrimSpace(text) == "" {
		return "paste a complete SKILL.md", m.styles.dim
	}
	s := vault.ParseContent([]byte(text))
	if s.Valid {
		return "valid: " + s.Name, m.styles.glyph[ops.Synced]
	}
	return "invalid: " + s.Problems[0], m.styles.fail
}

// pathBody is the IMPORT or EXPORT prompt: its input and what the input takes.
func (m Model) pathBody(w int) []string {
	hint := "a skill folder, a git URL or a .tar.gz path"
	if m.mode == modeExport {
		hint = "a new file; an existing file is refused"
	}
	return []string{
		m.path.View(),
		"",
		m.styles.dim.Render(truncate(hint, w)),
	}
}

// themeBody is the THEME picker: one row per theme, the cursor on the previewed one.
func (m Model) themeBody(w int) []string {
	all := theme.All()
	var lines []string
	end := min(len(all), m.themeTop+m.themeRows())
	for i := m.themeTop; i < end; i++ {
		th := all[i]
		selected := i == m.themeCur
		marker, nameStyle := "  ", lipgloss.NewStyle()
		if selected {
			marker, nameStyle = pad(m.icons.Marker, 2), m.styles.bold
		}
		note := ""
		if th.Name == m.themePrev.Name {
			note = "  (in use)"
		}
		name := truncate(th.Name, w-2)
		lines = append(lines, m.on(selected, m.styles.accent).Render(marker)+
			m.on(selected, nameStyle).Render(name)+
			m.on(selected, m.styles.dim).Render(truncate(note, w-2-lipgloss.Width(name))))
	}
	return lines
}
