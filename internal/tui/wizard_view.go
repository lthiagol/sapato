package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// wizardBody is the NEW wizard: the name and the description, each with its check,
// then the agents the skill works with. The focused field is bold.
func (m Model) wizardBody(w int) []string {
	z := m.wiz
	nameText, nameStyle := m.nameCheck()
	descText, descStyle := m.descCheck()
	lines := []string{
		m.styles.bold.Render(truncate("New skill", w)),
		"",
		m.fieldLabel(z.focus == 0, "name", w),
		z.name.View(),
		nameStyle.Render(truncate(nameText, w)),
		"",
		m.fieldLabel(z.focus == 1, "description", w),
		z.desc.View(),
		descStyle.Render(truncate(descText, w)),
		"",
		m.fieldLabel(z.focus >= 2, "compatible agents", w),
	}
	for i, a := range m.agents {
		selected := z.focus == 2+i
		marker, nameStyle := "  ", lipgloss.NewStyle()
		if selected {
			marker, nameStyle = pad(m.icons.Marker, 2), m.styles.bold
		}
		box := "  "
		if z.checked[i] {
			box = pad(m.icons.Mark, 2)
		}
		lines = append(lines, m.on(selected, m.styles.accent).Render(marker)+
			m.on(selected, m.styles.accent).Render(box)+
			m.on(selected, nameStyle).Render(truncate(a.ID, w-4)))
	}
	return append(lines, "", m.agentsLine(w))
}

// fieldLabel is the label of a wizard field: bold while the field has the focus, dim
// otherwise.
func (m Model) fieldLabel(focused bool, text string, w int) string {
	if focused {
		return m.styles.bold.Render(truncate(text, w))
	}
	return m.styles.dim.Render(truncate(text, w))
}

// agentsLine says how many agents are checked. None is an error, and every agent
// means the skill has no compatible list.
func (m Model) agentsLine(w int) string {
	picked := len(m.pickedAgents())
	switch {
	case picked == 0:
		return m.styles.fail.Render(truncate("pick at least one agent", w))
	case picked == len(m.agents):
		return m.styles.dim.Render(truncate("every agent, no compatible list", w))
	}
	return m.styles.dim.Render(truncate(fmt.Sprintf("%d of %d agents", picked, len(m.agents)), w))
}
