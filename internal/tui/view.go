package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"

	"sapato/internal/config"
	"sapato/internal/ops"
)

const (
	// matrixChrome is the number of lines the matrix view takes besides its rows.
	matrixChrome = 6
	// diffChrome is the number of lines the diff view takes besides the pager.
	diffChrome = 2
)

// scrollHint is a help-only entry for the pager's scroll keys.
var scrollHint = key.NewBinding(key.WithHelp("↑↓ pgup/pgdn", "scroll"))

func (m Model) View() string {
	switch m.mode {
	case modeConfirm:
		return m.confirmView()
	case modeDiff:
		return m.diffView()
	case modeHelp:
		return m.helpView()
	}
	return m.matrixView()
}

func (m Model) matrixView() string {
	nameW, colW := m.layout()
	lines := []string{
		m.styles.bold.Render("sapato") + m.styles.dim.Render(fmt.Sprintf(" · %d skills · %d agents", len(m.skills), len(m.agents))),
		m.headerLine(nameW, colW),
	}
	lines = append(lines, m.rowLines(nameW, colW)...)
	lines = append(lines, m.filterLine(), m.detailLine(), m.resultLine(), m.hintLine(m.keys.matrixHints()))
	return m.fit(lines)
}

// layout returns the width of the skill column and of each agent column.
func (m Model) layout() (nameW int, colW []int) {
	colW = make([]int, len(m.agents))
	total := 0
	for i, a := range m.agents {
		colW[i] = max(lipgloss.Width(strings.ToUpper(a.ID)), 5) + 2
		total += colW[i]
	}
	nameW = lipgloss.Width("SKILL")
	for _, s := range m.skills {
		nameW = max(nameW, lipgloss.Width(s))
	}
	if m.width > 0 {
		// Two columns go to the cursor marker.
		nameW = min(nameW, max(5, m.width-2-total))
	}
	return nameW, colW
}

func (m Model) headerLine(nameW int, colW []int) string {
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", 2))
	b.WriteString(m.styles.dim.Render(pad("SKILL", nameW)))
	for i, a := range m.agents {
		b.WriteString(m.styles.dim.Render(center(strings.ToUpper(a.ID), colW[i])))
	}
	return b.String()
}

// rowLines returns the visible rows, padded with blank lines to the height of
// the window, so the footer stays at the bottom.
func (m Model) rowLines(nameW int, colW []int) []string {
	vis := m.visibleRows()
	var lines []string
	switch {
	case len(m.agents) == 0:
		lines = append(lines, m.styles.dim.Render("No enabled agents. Enable one in the config."))
	case len(m.skills) == 0:
		lines = append(lines, m.styles.dim.Render("No skills in the vault or the agent folders."))
	}
	end := min(len(m.skills), m.top+vis)
	for i := m.top; i < end; i++ {
		lines = append(lines, m.rowLine(i, nameW, colW))
	}
	for len(lines) < vis {
		lines = append(lines, "")
	}
	return lines
}

func (m Model) rowLine(i int, nameW int, colW []int) string {
	skill := m.skills[i]
	selectedRow := i == m.row
	marker, nameStyle := "  ", lipgloss.NewStyle()
	if selectedRow {
		marker, nameStyle = "> ", m.styles.bold
	}
	var b strings.Builder
	b.WriteString(marker)
	b.WriteString(nameStyle.Render(pad(truncate(skill, nameW), nameW)))
	for j, a := range m.agents {
		b.WriteString(m.cellText(skill, a, selectedRow && j == m.col, colW[j]))
	}
	return b.String()
}

// cellText renders one cell, centered in w columns. A cell with no status is blank.
func (m Model) cellText(skill string, agent config.Agent, selected bool, w int) string {
	glyph, style := " ", lipgloss.NewStyle()
	if state, ok := m.cells[cell{skill, agent.ID}]; ok {
		glyph = "?"
		if info, known := stateInfoOf(state); known {
			glyph = info.glyph
		}
		style = m.styles.glyph[state]
	}
	if selected {
		style = style.Reverse(true)
	}
	return center(style.Render(glyph), w)
}

func (m Model) filterLine() string {
	switch {
	case m.mode == modeFilter:
		return m.filter.View()
	case m.query != "":
		return m.styles.dim.Render("filter: " + m.query + "  (/ to edit, esc to clear)")
	}
	return ""
}

func (m Model) detailLine() string {
	skill, agent, state, ok := m.selected()
	if !ok {
		return ""
	}
	what := "absent"
	if state != "" {
		what = string(state)
	}
	return m.styles.dim.Render(fmt.Sprintf("%s in %s: %s  %s", skill, agent.ID, what, filepath.Join(agent.Path, skill)))
}

func (m Model) resultLine() string {
	if m.resultErr {
		return m.styles.fail.Render(m.result)
	}
	return m.result
}

// hintLine lists the bindings as "key description" pairs.
func (m Model) hintLine(bindings []key.Binding) string {
	parts := make([]string, 0, len(bindings))
	for _, b := range bindings {
		h := b.Help()
		parts = append(parts, h.Key+" "+h.Desc)
	}
	return m.styles.dim.Render(strings.Join(parts, " · "))
}

// confirmView shows a plain confirm, or a force prompt in warning style.
func (m Model) confirmView() string {
	p := m.pending
	var lines []string
	if p.act.force {
		lines = []string{
			m.styles.fail.Render(fmt.Sprintf("Force %s %s in %s?", p.act.op, p.act.skill, p.act.agent.ID)),
			"",
			"  " + p.reason,
			"",
			m.styles.fail.Render(effect(p.act)),
		}
	} else {
		lines = []string{m.styles.bold.Render(fmt.Sprintf("%s %s in %s?", p.act.op, p.act.skill, p.act.agent.ID)), ""}
		for _, a := range p.plan {
			lines = append(lines, "  "+describe(a))
		}
		lines = append(lines, "", m.styles.dim.Render(effect(p.act)))
	}
	lines = append(lines, "", m.hintLine([]key.Binding{m.keys.yes, m.keys.no}))
	return m.fit(lines)
}

func (m Model) diffView() string {
	lines := []string{m.styles.bold.Render(m.diffTitle)}
	lines = append(lines, strings.Split(m.diff.View(), "\n")...)
	lines = append(lines, m.hintLine([]key.Binding{m.keys.closeView, scrollHint}))
	return m.fit(lines)
}

func (m Model) helpView() string {
	lines := []string{m.styles.bold.Render("Help"), "", m.styles.bold.Render("Cells")}
	for _, st := range states {
		lines = append(lines, fmt.Sprintf("  %s  %s %s", m.styles.glyph[st.state].Render(st.glyph), pad(string(st.state), 13), st.desc))
	}
	lines = append(lines, "", m.styles.bold.Render("Keys"))
	for _, b := range append(m.keys.matrixHints(), m.keys.esc) {
		h := b.Help()
		lines = append(lines, fmt.Sprintf("  %s %s", pad(h.Key, 16), h.Desc))
	}
	lines = append(lines, fmt.Sprintf("  %s %s", pad("ctrl+c", 16), "quit from any view"))
	lines = append(lines, "", m.styles.dim.Render("esc or ? closes this view"))
	return m.fit(lines)
}

// renderChanges lists the changed files, then their text diffs. Added and
// removed lines are colored.
func (m Model) renderChanges(changes []ops.FileChange) string {
	if len(changes) == 0 {
		return "no differences"
	}
	var lines []string
	for _, c := range changes {
		note := ""
		switch {
		case c.Binary:
			note = " (binary differs)"
		case c.Mode:
			note = " (mode differs)"
		}
		lines = append(lines, fmt.Sprintf("%s %s%s", c.Kind, c.Rel, note))
	}
	for _, c := range changes {
		if c.Diff == "" {
			continue
		}
		lines = append(lines, "")
		for _, line := range strings.Split(strings.TrimRight(c.Diff, "\n"), "\n") {
			lines = append(lines, m.colorDiffLine(line))
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) colorDiffLine(line string) string {
	switch {
	case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		return m.styles.bold.Render(line)
	case strings.HasPrefix(line, "+"):
		return m.styles.added.Render(line)
	case strings.HasPrefix(line, "-"):
		return m.styles.removed.Render(line)
	case strings.HasPrefix(line, "@@"):
		return m.styles.dim.Render(line)
	}
	return line
}

// fit cuts each line to the width of the terminal, so no line wraps.
func (m Model) fit(lines []string) string {
	if m.width > 0 {
		clip := lipgloss.NewStyle().MaxWidth(m.width)
		for i, l := range lines {
			lines[i] = clip.Render(l)
		}
	}
	return strings.Join(lines, "\n")
}

// stateInfoOf returns the legend entry of state. ok is false for an unknown state.
func stateInfoOf(state ops.State) (info stateInfo, ok bool) {
	for _, st := range states {
		if st.state == state {
			return st, true
		}
	}
	return stateInfo{}, false
}

// describe renders one planned action for the confirm screen.
func describe(a ops.Action) string {
	if a.Op == ops.OpSkip {
		return fmt.Sprintf("skip %s in %s: %s", a.Skill, a.Agent, a.Reason)
	}
	return fmt.Sprintf("%s %s", a.Op, a.Path)
}

// effect says what a confirmed action does to local state. A force prompt
// states what is lost.
func effect(a action) string {
	switch a.op {
	case ops.OpUninstall:
		if a.force {
			return "Local edits are lost with the installed copy and its lock entry."
		}
		return "Removes the installed copy and its lock entry. The vault copy stays."
	case ops.OpUpdate:
		return "Overwrites local edits in the installed copy."
	case ops.OpInstall:
		return "Overwrites the existing folder, including anything in it."
	case ops.OpAdopt:
		return fmt.Sprintf("Replaces the vault copy of %s with this folder.", a.skill)
	}
	return ""
}

// truncate cuts s to w columns, ending with an ellipsis when it had to cut.
func truncate(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	if w <= 1 {
		return string(r[:max(w, 0)])
	}
	return string(r[:w-1]) + "…"
}

// pad appends spaces so s is w columns wide.
func pad(s string, w int) string {
	return s + strings.Repeat(" ", max(0, w-lipgloss.Width(s)))
}

// center pads s with spaces on both sides to w columns.
func center(s string, w int) string {
	gap := max(0, w-lipgloss.Width(s))
	left := gap / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", gap-left)
}
