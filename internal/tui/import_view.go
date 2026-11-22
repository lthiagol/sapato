package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"sapato/internal/ops"
)

// importListBody is the IMPORT list of several skills: one row per candidate, with
// its folder, its mark and its name, or the first problem when it is invalid. An
// invalid row is dimmed. The rows are a window that follows the cursor.
func (m Model) importListBody(w int) []string {
	l := m.multi
	lines := []string{m.styles.bold.Render(truncate("Several skills in "+l.src, w)), ""}
	relW := 0
	for _, c := range l.cands {
		relW = max(relW, lipgloss.Width(c.Rel))
	}
	relW = min(relW, max(5, w/2))
	end := min(len(l.cands), l.top+m.listRows())
	for i := l.top; i < end; i++ {
		c := l.cands[i]
		selected := i == l.cur
		marker, nameStyle := "  ", lipgloss.NewStyle()
		if selected {
			marker, nameStyle = pad(m.icons.Marker, 2), m.styles.bold
		}
		mark := "  "
		if l.marks[c.Rel] {
			mark = pad(m.icons.Mark, 2)
		}
		status, statusStyle := m.candStatus(c)
		if !c.Valid {
			nameStyle = m.styles.dim
		}
		head := m.on(selected, m.styles.accent).Render(marker) +
			m.on(selected, m.styles.accent).Render(mark) +
			m.on(selected, nameStyle).Render(pad(truncate(c.Rel, relW), relW)) +
			m.spaces(2, selected)
		room := w - lipgloss.Width(head)
		lines = append(lines, head+m.on(selected, statusStyle).Render(truncate(status, room)))
	}
	marked := 0
	for _, c := range l.cands {
		if l.marks[c.Rel] {
			marked++
		}
	}
	return append(lines, "", m.styles.dim.Render(truncate(fmt.Sprintf("%d of %d marked", marked, len(l.cands)), w)))
}

// candStatus is the status of candidate c in the list and its style: its name, or
// the first problem, dimmed, when it is invalid.
func (m Model) candStatus(c ops.Candidate) (string, lipgloss.Style) {
	if !c.Valid {
		return invalidText(c), m.styles.dim
	}
	return c.Name, m.styles.text
}

// fixBody is the fix-up form: the problems of the skill, then its editable name and
// description. A problem longer than w is cut with an ellipsis.
func (m Model) fixBody(w int) []string {
	f := m.fixup
	lines := []string{m.styles.bold.Render(truncate("Fix the skill before it is imported", w)), ""}
	for _, p := range f.cand.Problems {
		lines = append(lines, m.styles.fail.Render(truncate(p, w)))
	}
	return append(lines,
		"",
		m.styles.dim.Render(truncate("name", w)),
		f.name.View(),
		"",
		m.styles.dim.Render(truncate("description", w)),
		f.desc.View(),
	)
}

// browseBody is the file picker: the folder it lists, then one row per entry. Folders
// end in a slash. Files that IMPORT does not take are dimmed.
func (m Model) browseBody(w int) []string {
	b := m.browse
	lines := []string{
		m.styles.bold.Render(truncate("Choose a source", w)),
		m.styles.dim.Render(truncate(b.dir, w)),
		"",
	}
	if len(b.entries) == 0 {
		return append(lines, m.styles.dim.Render(truncate("(empty folder)", w)))
	}
	end := min(len(b.entries), b.top+m.browseRows())
	for i := b.top; i < end; i++ {
		e := b.entries[i]
		selected := i == b.cur
		marker, nameStyle := "  ", m.styles.text
		if selected {
			marker, nameStyle = pad(m.icons.Marker, 2), m.styles.bold
		}
		if !e.choosable {
			nameStyle = m.styles.dim
		}
		label := e.name
		if e.dir {
			label += "/"
		}
		lines = append(lines, m.on(selected, m.styles.accent).Render(marker)+
			m.on(selected, nameStyle).Render(truncate(label, w-2)))
	}
	return lines
}
