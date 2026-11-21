package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"sapato/internal/ops"
	"sapato/internal/tui/theme"
	"sapato/internal/vault"
)

// dialogMinW is the least width of a dialog block (spec §9.4).
const dialogMinW = 60

// dialogBlock centers a dialog as one block, as wide as its widest line, at least
// dialogMinW and at most the terminal. build renders the lines for a width, so a
// long value is cut to fit: it runs at the terminal width to measure the block,
// then at the block width.
func (m Model) dialogBlock(build func(w int) []string) []string {
	blockW := min(m.layoutWidth(), max(dialogMinW, maxWidth(build(m.layoutWidth()))))
	return m.centered(build(blockW), blockW)
}

// inputBlock centers a dialog whose input sets its width: inputWidth, which
// sizeInputs fits the editor and the path prompt to.
func (m Model) inputBlock(build func(w int) []string) []string {
	w := m.inputWidth()
	return m.centered(build(w), w)
}

// inputWidth is the width of the NEW editor and the path prompts: two thirds of
// the terminal, at least dialogMinW, never wider than the terminal.
func (m Model) inputWidth() int {
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

// adoptBody is the ADOPT picker: one row per unmanaged copy, with its agent and
// its folder. The rows are a window that follows the cursor.
func (m Model) adoptBody(w int) []string {
	rows := m.unmanaged()
	lines := []string{m.styles.bold.Render(truncate("Adopt unmanaged skills into the vault", w)), ""}
	if len(rows) == 0 {
		return append(lines, m.styles.dim.Render(truncate("No unmanaged skills in the configured agents.", w)))
	}
	nameW, agentW := 0, 0
	for _, r := range rows {
		nameW = max(nameW, lipgloss.Width(r.skill))
		agentW = max(agentW, lipgloss.Width(r.agent.ID))
	}
	nameW = min(nameW, max(5, w/2))
	end := min(len(rows), m.pickTop+m.pickRows())
	for i := m.pickTop; i < end; i++ {
		r := rows[i]
		selected := i == m.pick
		marker, nameStyle := "  ", lipgloss.NewStyle()
		if selected {
			marker, nameStyle = pad(m.icons.Marker, 2), m.styles.bold
		}
		head := m.on(selected, m.styles.accent).Render(marker) +
			m.on(selected, nameStyle).Render(pad(truncate(r.skill, nameW), nameW)) +
			m.spaces(2, selected) +
			m.on(selected, lipgloss.NewStyle()).Render(pad(r.agent.ID, agentW)) +
			m.spaces(2, selected)
		room := w - lipgloss.Width(head)
		lines = append(lines, head+m.on(selected, m.styles.dim).Render(truncate(r.path, room)))
	}
	return lines
}

// newBody is the NEW editor with its live check under it.
func (m Model) newBody(w int) []string {
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
	title, hint := "Import from", "a skill folder, a git URL or a .tar.gz path"
	if m.mode == modeExport {
		title, hint = "Export to", "a new file; an existing file is refused"
	}
	return []string{
		m.styles.bold.Render(truncate(title, w)),
		"",
		m.path.View(),
		"",
		m.styles.dim.Render(truncate(hint, w)),
	}
}

// themeBody is the THEME picker: one row per theme, the cursor on the previewed one.
func (m Model) themeBody(w int) []string {
	all := theme.All()
	lines := []string{m.styles.bold.Render(truncate("Theme", w)), ""}
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
