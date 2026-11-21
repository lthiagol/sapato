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

// artW is the width of the widest art line.
var artW = maxWidth(sapatoArt)

// minTallHeight is the terminal height from which the full header shows. Shorter
// terminals get the one-line header.
const minTallHeight = 30

// labelW is the width of the label column of the skill screen's info block.
const labelW = 13

// View draws the frame of the screen: the header, the rule, the body of the
// screen or overlay, and the bar.
func (m Model) View() string {
	return m.frame(m.body())
}

// body is the body of the current screen or overlay, before it is fitted to the
// frame. Each dialog is a centered block.
func (m Model) body() []string {
	switch m.mode {
	case modeConfirm:
		return m.dialogBlock(m.confirmBody)
	case modeDiff:
		return m.diffBody()
	case modeHelp:
		return m.dialogBlock(m.helpBody)
	case modeAdopt:
		return m.dialogBlock(m.adoptBody)
	case modeNew:
		return m.inputBlock(m.newBody)
	case modeImport, modeExport:
		return m.inputBlock(m.pathBody)
	case modeTheme:
		return m.dialogBlock(m.themeBody)
	case modeLog:
		return m.dialogBlock(m.logBody)
	}
	if m.screen == screenSkill {
		return m.skillBody()
	}
	return m.mainBody()
}

// frame stacks the header, a rule, the body fitted to the rows between the rule
// and the bar, and the two bar rows. The notifications are drawn over the body.
// Before the first size message the body is not fitted.
func (m Model) frame(body []string) string {
	lines := append(m.headerLines(), m.rule())
	if h := m.bodyHeight(); h > 0 {
		body = body[:min(len(body), h)]
		for len(body) < h {
			body = append(body, "")
		}
		body = m.overlay(body)
	}
	lines = append(lines, body...)
	lines = append(lines, m.bar()...)
	return m.fit(lines)
}

// compact reports whether the header is the one-line form: the terminal is
// shorter than minTallHeight rows, or narrower than the art.
func (m Model) compact() bool {
	return (m.height > 0 && m.height < minTallHeight) || (m.width > 0 && m.width < artW)
}

// headerHeight is the number of lines the header takes.
func (m Model) headerHeight() int {
	if m.compact() {
		return 1
	}
	return len(sapatoArt) + 1
}

// bodyHeight is the number of lines between the rule under the header and the
// bar, which takes two rows. It is 0 before the terminal reports its size.
func (m Model) bodyHeight() int {
	if m.height == 0 {
		return 0
	}
	return max(1, m.height-m.headerHeight()-3)
}

// headerLines returns the header, centered: the art and tagline, or the one-line
// form with the tagline dropped when it does not fit.
func (m Model) headerLines() []string {
	if m.compact() {
		head := m.styles.bold.Render("sapato")
		if m.width == 0 || lipgloss.Width("sapato · "+tagline) <= m.width {
			head += m.styles.dim.Render(" · ") + m.styles.subtext.Render(tagline)
		}
		return []string{center(head, m.width)}
	}
	indent := strings.Repeat(" ", max(0, (m.width-artW)/2))
	lines := make([]string, 0, len(sapatoArt)+1)
	for _, l := range sapatoArt {
		lines = append(lines, indent+m.styles.accent.Render(l))
	}
	return append(lines, center(m.styles.subtext.Render(tagline), m.width))
}

// rule is the full-width line under the header and above the bar.
func (m Model) rule() string {
	return m.styles.rule.Render(strings.Repeat(m.icons.Rule, m.width))
}

// modeName is the badge text of the current screen or overlay.
func (m Model) modeName() string {
	switch m.mode {
	case modeFilter:
		return "FILTER"
	case modeConfirm:
		return "CONFIRM"
	case modeDiff:
		return "DIFF"
	case modeHelp:
		return "HELP"
	case modeAdopt:
		return "ADOPT"
	case modeNew:
		return "NEW"
	case modeImport:
		return "IMPORT"
	case modeExport:
		return "EXPORT"
	case modeTheme:
		return "THEME"
	case modeLog:
		return "LOG"
	}
	if m.screen == screenSkill {
		return "SKILL"
	}
	return "MAIN"
}

// mainBody is the grid: a column-header row, a rule, then the skill rows of the
// scrolling window. The grid block is centered.
func (m Model) mainBody() []string {
	nameW, colW := m.layout()
	total := 2 + nameW
	for _, w := range colW {
		total += w
	}
	pad := m.leftPad(total)
	lines := []string{pad + m.headerRow(nameW, colW), m.rule()}
	switch {
	case len(m.agents) == 0:
		lines = append(lines, pad+m.styles.dim.Render("No enabled agents. Enable one in the config."))
	case len(m.skills) == 0:
		lines = append(lines, pad+m.styles.dim.Render("No skills in the vault or the agent folders."))
	}
	end := min(len(m.skills), m.top+m.visibleRows())
	for i := m.top; i < end; i++ {
		lines = append(lines, pad+m.rowLine(i, nameW, colW))
	}
	return lines
}

// layout returns the width of the skill column and of each agent column. The
// skill column is as wide as the longest name, capped so the grid fits the width.
func (m Model) layout() (nameW int, colW []int) {
	colW = make([]int, len(m.agents))
	total := 0
	for i, a := range m.agents {
		colW[i] = max(lipgloss.Width(a.ID), 3) + 2
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

// leftPad is the indent that centers a block blockW columns wide.
func (m Model) leftPad(blockW int) string {
	if m.width == 0 {
		return ""
	}
	return strings.Repeat(" ", max(0, (m.width-blockW)/2))
}

func (m Model) headerRow(nameW int, colW []int) string {
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", 2))
	b.WriteString(m.styles.accent.Render(pad("SKILL", nameW)))
	for i, a := range m.agents {
		b.WriteString(m.styles.accent.Render(center(strings.ToUpper(a.ID), colW[i])))
	}
	return b.String()
}

func (m Model) rowLine(i int, nameW int, colW []int) string {
	skill := m.skills[i]
	selected := i == m.row
	marker, nameStyle := "  ", lipgloss.NewStyle()
	if selected {
		marker, nameStyle = pad(m.icons.Marker, 2), m.styles.bold
	}
	var b strings.Builder
	b.WriteString(m.on(selected, m.styles.accent).Render(marker))
	b.WriteString(m.on(selected, nameStyle).Render(pad(truncate(skill, nameW), nameW)))
	for j, a := range m.agents {
		b.WriteString(m.cellText(skill, a, selected, colW[j]))
	}
	return b.String()
}

// cellText renders one cell, centered in w columns. A cell with no status is blank.
func (m Model) cellText(skill string, agent config.Agent, selected bool, w int) string {
	glyph, style := " ", lipgloss.NewStyle()
	if state, ok := m.cells[cell{skill, agent.ID}]; ok {
		glyph = m.icons.Status[state]
		style = m.styles.glyph[state]
	}
	gap := max(0, w-lipgloss.Width(glyph))
	left := gap / 2
	return m.spaces(left, selected) + m.on(selected, style).Render(glyph) + m.spaces(gap-left, selected)
}

// on paints s in the selected-row style when selected. The row style sets only a
// background, or reverse video without color, so s keeps its own foreground.
func (m Model) on(selected bool, s lipgloss.Style) lipgloss.Style {
	if selected {
		return s.Inherit(m.styles.row)
	}
	return s
}

// spaces is n spaces in the row style, so a selected row has no gaps.
func (m Model) spaces(n int, selected bool) string {
	return m.on(selected, lipgloss.NewStyle()).Render(strings.Repeat(" ", n))
}

// skillBody is the skill screen: the info block, a rule as wide as the block, then
// the agent rows. The block is centered, and every line fits the terminal.
func (m Model) skillBody() []string {
	w := m.layoutWidth()
	info, blockW := m.skillBlock(w)
	agents := m.agentLines(w)
	lines := append(info, m.styles.rule.Render(strings.Repeat(m.icons.Rule, blockW)))
	return m.centered(append(lines, agents...), blockW)
}

// skillBlock is the info block of the skill screen and the width it is centered
// at. The width is the widest line other than the description, at least 60 columns
// and at most w. The description is wrapped to that width, so it comes last.
func (m Model) skillBlock(w int) (info []string, blockW int) {
	blockW = min(w, max(60, maxWidth(m.skillInfo(w, nil)), maxWidth(m.agentRows(w))))
	return m.skillInfo(w, m.descriptionLines(blockW)), blockW
}

// agentRows is one line per enabled agent of the open skill, cut to w columns.
func (m Model) agentRows(w int) []string {
	if len(m.agents) == 0 {
		return []string{m.styles.dim.Render(truncate("No enabled agents. Enable one in the config.", w))}
	}
	idW, statusW := 0, 0
	for _, a := range m.agents {
		idW = max(idW, lipgloss.Width(a.ID))
	}
	for _, st := range states {
		statusW = max(statusW, lipgloss.Width(string(st.state)))
	}
	statusW = max(statusW, lipgloss.Width("absent"))
	lines := make([]string, len(m.agents))
	for i, a := range m.agents {
		lines[i] = m.agentRow(i, a, idW, statusW, w)
	}
	return lines
}

// agentLines is the agent list of the skill screen: the rows of its window, from
// agentTop.
func (m Model) agentLines(w int) []string {
	rows := m.agentRows(w)
	if len(m.agents) == 0 {
		return rows
	}
	end := min(len(m.agents), m.agentTop+m.agentRowsShown())
	return rows[min(m.agentTop, end):end]
}

// skillInfo is the info block of the open skill, cut to w columns: name, version,
// hash, validity, its problems, then desc, and the vault path. desc is the
// description lines, nil to leave them out.
func (m Model) skillInfo(w int, desc []string) []string {
	s, inVault := m.vaultSkills[m.open]
	valueW := w - labelW
	lines := []string{m.styles.bold.Render(truncate(m.open, w))}
	field := func(label, value string) {
		lines = append(lines, m.styles.dim.Render(pad(label, labelW))+value)
	}
	if !inVault {
		field("version", m.styles.dim.Render("-"))
		field("hash", m.styles.dim.Render("-"))
		field("status", m.styles.dim.Render("not in the vault"))
		field("vault", m.styles.dim.Render("not in the vault"))
		return lines
	}
	field("version", truncate(orDash(s.Version), valueW))
	field("hash", truncate(orDash(shortHash(s.Hash)), valueW))
	if s.Valid {
		field("status", m.styles.glyph[ops.Synced].Render("valid"))
	} else {
		field("status", m.styles.fail.Render("invalid"))
		for _, p := range s.Problems {
			lines = append(lines, pad("", labelW)+m.styles.fail.Render(truncate(p, valueW)))
		}
	}
	lines = append(lines, desc...)
	field("vault", truncate(s.Dir, valueW))
	return lines
}

// descriptionLines wraps the open skill's description to three lines in blockW
// columns, the first after its label. When the text runs longer, the third line
// ends with an ellipsis. It is nil when the skill is not in the vault.
func (m Model) descriptionLines(blockW int) []string {
	s, inVault := m.vaultSkills[m.open]
	if !inVault {
		return nil
	}
	if s.Description == "" {
		return []string{m.styles.dim.Render(pad("description", labelW)) + m.styles.dim.Render("-")}
	}
	width := max(1, blockW-labelW)
	if m.width == 0 {
		width = 60
	}
	wrapped := strings.Split(lipgloss.NewStyle().Width(width).Render(s.Description), "\n")
	for i, l := range wrapped {
		wrapped[i] = strings.TrimRight(l, " ")
	}
	if len(wrapped) > 3 {
		wrapped = append(wrapped[:2], truncate(strings.Join(wrapped[2:], " "), width))
	}
	lines := make([]string, 0, len(wrapped))
	for i, l := range wrapped {
		label := pad("", labelW)
		if i == 0 {
			label = m.styles.dim.Render(pad("description", labelW))
		}
		lines = append(lines, label+l)
	}
	return lines
}

// agentRow is one agent of the skill screen: cursor marker, mark, agent id, status
// icon and word, and the installed path when the folder is there. The path is cut
// to w columns, and left out when no room is left for it.
func (m Model) agentRow(i int, a config.Agent, idW, statusW, w int) string {
	selected := i == m.cur
	marker, mark := "  ", "  "
	if selected {
		marker = pad(m.icons.Marker, 2)
	}
	if m.marks[a.ID] {
		mark = pad(m.icons.Mark, 2)
	}
	glyph, word, style := " ", "absent", m.styles.dim
	path := ""
	if state, ok := m.cells[cell{m.open, a.ID}]; ok {
		glyph, word, style = m.icons.Status[state], string(state), m.styles.glyph[state]
		switch state {
		case ops.Synced, ops.Outdated, ops.Modified, ops.Orphaned, ops.Unmanaged:
			path = filepath.Join(a.Path, m.open)
		}
	}
	idStyle := lipgloss.NewStyle()
	if selected {
		idStyle = m.styles.bold
	}
	var b strings.Builder
	b.WriteString(m.on(selected, m.styles.accent).Render(marker))
	b.WriteString(m.on(selected, m.styles.accent).Render(mark))
	b.WriteString(m.on(selected, idStyle).Render(pad(a.ID, idW)))
	b.WriteString(m.spaces(2, selected))
	b.WriteString(m.on(selected, style).Render(glyph + " " + pad(word, statusW)))
	head := b.String()
	room := w - lipgloss.Width(head) - 2
	if path == "" || room < 2 {
		return head
	}
	return head + m.spaces(2, selected) + m.on(selected, m.styles.dim).Render(truncate(path, room))
}

// confirmBody shows a plain confirm with its plan, or a force prompt in warning style.
// A discard of the NEW editor asks for its own plain confirm. Values longer than w
// are cut with an ellipsis.
func (m Model) confirmBody(w int) []string {
	p := m.pending
	if p.act.op == opDiscard {
		return []string{
			m.styles.bold.Render(truncate("Discard the new skill?", w)),
			"",
			m.styles.dim.Render(truncate(effect(p.act), w)),
		}
	}
	if p.act.force {
		return []string{
			m.styles.fail.Render(truncate("Force "+subject(p.act)+"?", w)),
			"",
			"  " + truncate(p.reason, w-2),
			"",
			m.styles.fail.Render(truncate(effect(p.act), w)),
		}
	}
	lines := []string{m.styles.bold.Render(truncate(subject(p.act)+"?", w)), ""}
	for _, a := range p.plan {
		lines = append(lines, "  "+truncate(describe(a), w-2))
	}
	return append(lines, "", m.styles.dim.Render(truncate(effect(p.act), w)))
}

func (m Model) diffBody() []string {
	return append([]string{m.styles.bold.Render(m.diffTitle)}, strings.Split(m.diff.View(), "\n")...)
}

// helpBody is the keys of the current screen and the icon legend. A description
// longer than w is cut with an ellipsis.
func (m Model) helpBody(w int) []string {
	// helpLine is a line of a label column and a description, cut to w.
	helpLine := func(prefix, desc string) string {
		return prefix + truncate(desc, w-lipgloss.Width(prefix))
	}
	lines := []string{m.styles.bold.Render(truncate("Help", w)), "", m.styles.bold.Render("Cells")}
	for _, st := range states {
		glyph := m.styles.glyph[st.state].Render(m.icons.Status[st.state])
		lines = append(lines, helpLine(fmt.Sprintf("  %s  %s ", glyph, pad(string(st.state), 13)), st.desc))
	}
	lines = append(lines, "", m.styles.bold.Render("Keys"))
	keys := m.hints()
	if m.screen == screenMain {
		keys = append(keys, m.keys.esc)
	}
	for _, b := range keys {
		h := b.Help()
		lines = append(lines, helpLine(fmt.Sprintf("  %s ", pad(h.Key, 16)), h.Desc))
	}
	lines = append(lines, helpLine(fmt.Sprintf("  %s ", pad("ctrl+c", 16)), "quit from any view"))
	lines = append(lines, "", m.styles.dim.Render(truncate("esc or ? closes this view", w)))
	return lines
}

// hints lists the bindings of the current screen, for the help view: the screen's
// own keys, then the global keys.
func (m Model) hints() []key.Binding {
	own, global := m.screenHints()
	return append(own, global...)
}

// screenHints lists the bindings of the current screen as its own keys and its
// global keys. The bar cuts the own keys first.
func (m Model) screenHints() (own, global []key.Binding) {
	if m.screen == screenSkill {
		return m.keys.skillHints(), m.keys.skillGlobals()
	}
	return m.keys.mainHints(), m.keys.mainGlobals()
}

// barHints lists the bindings the bar shows for the current mode, as the screen's
// own keys and its global keys. The other modes have no global keys.
func (m Model) barHints() (own, global []key.Binding) {
	switch m.mode {
	case modeFilter:
		return m.keys.filterHints(), nil
	case modeConfirm:
		return []key.Binding{m.keys.yes, m.keys.no}, nil
	case modeDiff:
		return []key.Binding{m.keys.closeView, scrollHint}, nil
	case modeHelp:
		return []key.Binding{m.keys.closeView}, nil
	case modeAdopt:
		return []key.Binding{moveHint, adoptHint, backHint}, nil
	case modeNew:
		return []key.Binding{m.keys.save, cancelHint}, nil
	case modeImport, modeExport:
		return []key.Binding{runHint, cancelHint}, nil
	case modeTheme:
		return []key.Binding{moveHint, keepHint, restoreHint}, nil
	case modeLog:
		return []key.Binding{scrollHint, m.keys.closeView}, nil
	}
	return m.screenHints()
}

// bar is the bottom bar of spec §9.1, two rows. The first has the mode badge and the
// counts, with the filter input after them while filtering. The second has the key
// hints, with the global keys last. Both rows fill the width in the Mantle background.
func (m Model) bar() []string {
	name := m.modeName()
	first := m.styles.badge(name).Render(" " + name + " ")
	first += m.styles.bar.Render(" ") + m.styles.barSub.Render(m.counts())
	if m.mode == modeFilter {
		first += m.styles.barSub.Render(" · ") + m.filter.View()
	}
	own, global := m.barHints()
	second := m.styles.bar.Render(" ") + m.styles.barDim.Render(hintText(own, global, m.layoutWidth()-1))
	if m.width == 0 {
		return []string{first, second}
	}
	return []string{m.barRow(first), m.barRow(second)}
}

// hintText joins the hints to w columns. The global keys come last and are kept
// whole while they fit. The own keys before them are cut with an ellipsis.
func hintText(own, global []key.Binding, w int) string {
	head, tail := joinHints(own), joinHints(global)
	switch {
	case tail == "":
		return truncate(head, w)
	case head == "":
		return truncate(tail, w)
	}
	room := w - lipgloss.Width(tail) - lipgloss.Width(" · ")
	if room < 1 {
		return truncate(tail, w)
	}
	return truncate(head, room) + " · " + tail
}

// barRow fills a bar row to the width in the Mantle background, and cuts what does
// not fit.
func (m Model) barRow(s string) string {
	return clip(s+m.styles.bar.Render(strings.Repeat(" ", max(0, m.width-lipgloss.Width(s)))), m.width)
}

// layoutWidth is the width the bar and the screen bodies are laid out in. Before
// the first size message nothing is cut, so it is wide enough for any text.
func (m Model) layoutWidth() int {
	if m.width == 0 {
		return 1 << 16
	}
	return m.width
}

// counts is the main screen's count segment: skills, invalid vault skills, and
// skills that need attention.
func (m Model) counts() string {
	invalid := 0
	for _, s := range m.vaultSkills {
		if !s.Valid {
			invalid++
		}
	}
	attention := 0
	for _, name := range m.names {
		if m.needsAttention(name) {
			attention++
		}
	}
	return fmt.Sprintf("%d skills · %d invalid · %d need attention", len(m.names), invalid, attention)
}

// needsAttention reports whether a skill is outdated, modified, orphaned or
// missing in some agent.
func (m Model) needsAttention(name string) bool {
	for _, a := range m.agents {
		switch m.cells[cell{name, a.ID}] {
		case ops.Outdated, ops.Modified, ops.Orphaned, ops.Missing:
			return true
		}
	}
	return false
}

// joinHints lists the bindings as "key description" pairs.
func joinHints(bindings []key.Binding) string {
	parts := make([]string, 0, len(bindings))
	for _, b := range bindings {
		h := b.Help()
		parts = append(parts, h.Key+" "+h.Desc)
	}
	return strings.Join(parts, " · ")
}

// fit cuts each line to the width of the terminal, so no line wraps.
func (m Model) fit(lines []string) string {
	if m.width > 0 {
		clipped := make([]string, len(lines))
		for i, l := range lines {
			clipped[i] = clip(l, m.width)
		}
		lines = clipped
	}
	return strings.Join(lines, "\n")
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

// describe renders one planned action for the confirm screen.
func describe(a ops.Action) string {
	if a.Op == ops.OpSkip {
		return fmt.Sprintf("skip %s in %s: %s", a.Skill, a.Agent, a.Reason)
	}
	return fmt.Sprintf("%s %s", a.Op, a.Path)
}

// subject names an action in its confirm prompt.
func subject(a action) string {
	switch a.op {
	case ops.OpRemove:
		return "remove " + a.skill
	case ops.OpCreate:
		return "create " + a.skill
	case ops.OpImport:
		return "import " + a.src
	}
	return fmt.Sprintf("%s %s in %s", a.op, a.skill, agentIDs(a.agents))
}

// effect says what a confirmed action does to local state. A force prompt
// states what is lost.
func effect(a action) string {
	switch a.op {
	case opDiscard:
		return "The text you wrote is lost."
	case ops.OpRemove:
		if a.force {
			return "Deletes the vault copy; installed copies become orphaned."
		}
		return "Deletes the skill from the vault."
	case ops.OpCreate, ops.OpImport:
		return "Overwrites the same-named skill in the vault."
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

// agentIDs lists the IDs of agents, comma-separated.
func agentIDs(agents []config.Agent) string {
	ids := make([]string, 0, len(agents))
	for _, a := range agents {
		ids = append(ids, a.ID)
	}
	return strings.Join(ids, ", ")
}

// shortHash keeps the algorithm and the first twelve hex digits of a hash.
func shortHash(h string) string {
	algo, hex, _ := strings.Cut(h, ":")
	if len(hex) > 12 {
		hex = hex[:12]
	}
	return algo + ":" + hex
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// clip cuts s to w columns, keeping any color codes. A width below one gives "".
func clip(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(s)
}

// truncate cuts s to w columns, ending with an ellipsis when it had to cut. A width
// of one or less gets no ellipsis.
func truncate(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	if w <= 0 {
		return ""
	}
	limit := w
	if w > 1 {
		limit = w - 1
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > limit {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	if w > 1 {
		b.WriteString("…")
	}
	return b.String()
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

// maxWidth is the width of the widest line.
func maxWidth(lines []string) int {
	w := 0
	for _, l := range lines {
		w = max(w, lipgloss.Width(l))
	}
	return w
}
