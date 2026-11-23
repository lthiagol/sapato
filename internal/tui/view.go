package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"sapato/internal/config"
	"sapato/internal/ops"
)

// artW is the width of the widest art line.
var artW = maxWidth(sapatoArt)

// minTallHeight is the terminal height from which the full header shows. Shorter
// terminals get the one-line header.
const minTallHeight = 30

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
		return m.confirmModal()
	case modeDiff:
		return m.diffBody()
	case modeHelp:
		return m.screenBody("Help", m.helpBody)
	case modeAdopt:
		return m.screenBody("Adopt unmanaged skills into the vault", m.adoptBody)
	case modeNew:
		return m.screenBody("Create a new skill", m.newBody)
	case modeImport:
		return m.screenBody("Import skills", m.pathBody)
	case modeExport:
		return m.screenBody("Export "+m.open, m.pathBody)
	case modeImportList:
		return m.screenBody("Import skills · pick skills", m.importListBody)
	case modeImportFix:
		return m.screenBody("Import skills · fix "+m.fixName(), m.fixBody)
	case modeImportBrowse:
		return m.screenBody("Import skills · choose a file", m.browseBody)
	case modeTheme:
		return m.screenBody("Theme", m.themeBody)
	case modeLog:
		return m.screenBody("Log", m.logBody)
	}
	if m.screen == screenSkill {
		return m.skillBody()
	}
	return m.mainBody()
}

// frame stacks the header, a rule, the body fitted to the rows between the rule
// and the bar, and the bar. The notifications are drawn over the body. Before the
// first size message the body is not fitted.
func (m Model) frame(body []string) string {
	lines := append(m.headerLines(), m.rule())
	if h := m.bodyHeight(); h > 0 {
		body = m.overlay(fitRows(body, h))
	}
	lines = append(lines, body...)
	lines = append(lines, m.bar()...)
	return m.fit(lines)
}

// fitRows cuts lines to h rows, and pads a short list with blank rows.
func fitRows(lines []string, h int) []string {
	lines = lines[:min(len(lines), h)]
	for len(lines) < h {
		lines = append(lines, "")
	}
	return lines
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

// bodyHeight is the number of lines between the rule under the header and the bar,
// which takes two or three: its rule and one or two hint rows. It is 0 before the
// terminal reports its size.
func (m Model) bodyHeight() int {
	if m.height == 0 {
		return 0
	}
	return max(1, m.height-m.headerHeight()-1-len(m.bar()))
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
		lines = append(lines, pad+m.styles.dim.Render("No agents configured. Add one under [agents] in the config."))
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

// confirmBody shows a plain confirm with its plan, or a force prompt in warning style.
// A discard of the NEW editor asks for its own plain confirm. Values longer than w
// are cut with an ellipsis.
func (m Model) confirmBody(w int) []string {
	p := m.pending
	if p.act.op == opDiscard {
		return append([]string{m.styles.bold.Render(truncate("Discard the new skill?", w)), ""}, m.effectLines(p.act, w)...)
	}
	if p.act.op == opConfigInit {
		return append([]string{m.styles.bold.Render(truncate("No config found. Create "+p.act.path+"?", w)), ""}, m.effectLines(p.act, w)...)
	}
	if p.act.op == opVaultInit {
		return append([]string{m.styles.bold.Render(truncate("No vault at "+p.act.path+". Create it?", w)), ""}, m.effectLines(p.act, w)...)
	}
	if p.act.op == opEdit {
		return append([]string{m.styles.bold.Render(truncate("Open "+p.act.path+" in "+m.editorName(p.act.path)+"?", w)), ""}, m.effectLines(p.act, w)...)
	}
	lines := []string{m.styles.bold.Render(truncate(capitalize(subject(p.act))+"?", w)), ""}
	if p.act.force {
		for _, l := range wrapText(strings.Join(p.reasons, "\n"), w-2) {
			lines = append(lines, "  "+l)
		}
	} else {
		for _, a := range p.plan {
			lines = append(lines, "  "+truncate(describe(a), w-2))
		}
	}
	return append(append(lines, ""), m.effectLines(p.act, w)...)
}

// effectLines is what a confirm says it loses or changes, wrapped to w columns and
// at most three lines. A force prompt states it in Red, the others in Overlay.
func (m Model) effectLines(a action, w int) []string {
	style := m.styles.dim
	if a.force {
		style = m.styles.fail
	}
	var lines []string
	for _, l := range wrapText(effect(a), w) {
		lines = append(lines, style.Render(l))
	}
	return lines
}

func (m Model) diffBody() []string {
	return append([]string{m.styles.bold.Render(m.diffTitle)}, strings.Split(m.diff.View(), "\n")...)
}

// helpBody is HELP: the lines from helpTop, as many as the body holds.
func (m Model) helpBody(w int) []string {
	lines := m.helpLines(w)
	top := min(m.helpTop, max(0, len(lines)-m.helpRows(len(lines))))
	return lines[top:]
}

// helpRows is how many HELP lines fit under the title band, given the total of n. Before
// the first size message it is every line.
func (m Model) helpRows(n int) int {
	if m.height == 0 {
		return n
	}
	return max(1, m.bodyHeight()-2)
}

// helpLines is the keys of the current screen and the icon legend. A description
// longer than w is cut with an ellipsis.
func (m Model) helpLines(w int) []string {
	// helpLine is a line of a label column and a description, cut to w.
	helpLine := func(prefix, desc string) string {
		return prefix + truncate(desc, w-lipgloss.Width(prefix))
	}
	lines := []string{m.styles.bold.Render("Cells")}
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

// bar is the bottom bar of spec §9.1: the rule carrying the counts, then the hint
// row, or two when the globals wrap. Each hint row is centered. The bar has no
// background.
func (m Model) bar() []string {
	own, global := m.barHints()
	bar := []string{m.summaryRule()}
	for _, row := range m.hintRows(own, global) {
		bar = append(bar, m.leftPad(lipgloss.Width(row))+row)
	}
	return bar
}

// summaryRule is the bar's rule with the counts right-aligned and two rule glyphs
// after them. The counts drop from the right, attention first, then invalid. The rule
// is plain when even the skill count does not fit with four rule glyphs.
func (m Model) summaryRule() string {
	if m.width == 0 {
		return m.rule()
	}
	parts := m.summaryParts()
	sep := m.styles.dim.Render(" · ")
	for n := len(parts); n > 0; n-- {
		sum := " " + strings.Join(parts[:n], sep) + " "
		if lipgloss.Width(sum)+4 > m.width {
			continue
		}
		lead := m.width - lipgloss.Width(sum) - 2
		return m.styles.rule.Render(strings.Repeat(m.icons.Rule, lead)) + sum +
			m.styles.rule.Render(strings.Repeat(m.icons.Rule, 2))
	}
	return m.rule()
}

// summaryParts are the counts of the bar's rule: the skills in Text, the invalid
// ones in Red and the ones that need attention in Yellow.
func (m Model) summaryParts() []string {
	invalid, attention := m.tally()
	return []string{
		m.count(len(m.names), "skills", m.styles.text),
		m.count(invalid, "invalid", m.styles.fail),
		m.count(attention, "need attention", m.styles.warn),
	}
}

// hintRows is the hint row of the bar, before it is centered: the action hints, ` │ `,
// then the global hints. When that is too wide the globals wrap to a second row, and
// the action part is cut with an ellipsis if it is too wide alone.
func (m Model) hintRows(own, global []key.Binding) []string {
	action := m.actionText(own)
	if len(global) == 0 {
		return []string{m.cutRow(action)}
	}
	if action == "" {
		return []string{m.globalRow(global)}
	}
	row := action + m.styles.dim.Render(" │ ") + m.globalText(global)
	if m.fits(row) {
		return []string{row}
	}
	return []string{m.cutRow(action), m.globalRow(global)}
}

// globalRow is the global hints as a row of their own. It drops t theme, then L log,
// until the row fits, and cuts it with an ellipsis when even that is too wide.
func (m Model) globalRow(global []key.Binding) string {
	var row string
	for drop := 0; drop <= 2; drop++ {
		row = m.globalText(dropGlobals(global, drop))
		if m.fits(row) {
			return row
		}
	}
	return m.cutRow(row)
}

// dropGlobals removes the global hints that go first when the row is narrow: at level
// 1 the t theme hint, at level 2 also the L log hint.
func dropGlobals(global []key.Binding, level int) []key.Binding {
	var kept []key.Binding
	for _, b := range global {
		switch b.Help().Key {
		case "t":
			if level >= 1 {
				continue
			}
		case "L":
			if level >= 2 {
				continue
			}
		}
		kept = append(kept, b)
	}
	return kept
}

// actionText is the action part of the hint row. While filtering it is the filter
// input followed by the hints.
func (m Model) actionText(own []key.Binding) string {
	text := m.hintText(own)
	if m.mode == modeFilter {
		sep := " · "
		// At the end of the input the view already ends in the cursor cell, a space.
		if m.filter.Position() == len([]rune(m.filter.Value())) {
			sep = "· "
		}
		text = m.filter.View() + m.styles.dim.Render(sep) + text
	}
	return text
}

// fits reports whether s is no wider than the bar's width. Before the first size
// message everything fits.
func (m Model) fits(s string) bool {
	return lipgloss.Width(s) <= m.layoutWidth()
}

// cutRow cuts a hint row to the width, ending with an ellipsis when it had to cut.
// The cut keeps the color codes of s.
func (m Model) cutRow(s string) string {
	return ansi.Truncate(s, m.layoutWidth(), "…")
}

// hintText lists the bindings as hints: each key in Accent bold, its description in
// Subtext, and the hints joined with Overlay dots.
func (m Model) hintText(bindings []key.Binding) string {
	return joinHints(bindings, m.styles.hintKey, m.styles.subtext, m.styles.dim)
}

// globalText lists the global hints one step dimmer than the action hints: each key
// in Subtext, its description in Overlay, joined with Overlay dots.
func (m Model) globalText(bindings []key.Binding) string {
	return joinHints(bindings, m.styles.subtext, m.styles.dim, m.styles.dim)
}

// joinHints renders each binding as its key in keySt and its description in descSt,
// joined with sepSt dots.
func joinHints(bindings []key.Binding, keySt, descSt, sepSt lipgloss.Style) string {
	parts := make([]string, 0, len(bindings))
	for _, b := range bindings {
		h := b.Help()
		parts = append(parts, keySt.Render(h.Key)+" "+descSt.Render(h.Desc))
	}
	return strings.Join(parts, sepSt.Render(" · "))
}

// barHints lists the bindings the bar shows for the current mode: the action hints and
// the global hints. The modes where L opens LOG list it among their globals. Text
// inputs and confirms have no globals: L is typed in the first, and the second only
// takes y or cancels.
func (m Model) barHints() (own, global []key.Binding) {
	switch m.mode {
	case modeFilter:
		return m.keys.filterHints(), nil
	case modeConfirm:
		return []key.Binding{m.keys.yes, m.keys.no}, nil
	case modeDiff:
		return []key.Binding{scrollHint}, []key.Binding{m.keys.closeView, m.keys.log}
	case modeHelp:
		return []key.Binding{scrollHint}, []key.Binding{m.keys.closeView, m.keys.log}
	case modeAdopt:
		return []key.Binding{adoptHint, m.keys.mark, agentHint}, []key.Binding{moveHint, backHint, m.keys.log}
	case modeNew:
		if m.newPaste {
			return []key.Binding{m.keys.save, wizardHint}, []key.Binding{cancelHint}
		}
		return []key.Binding{m.keys.save, m.keys.mark, pasteHint}, []key.Binding{fieldsHint, cancelHint}
	case modeImport:
		return []key.Binding{runHint, m.keys.openFile}, []key.Binding{cancelHint}
	case modeExport:
		return []key.Binding{runHint}, []key.Binding{cancelHint}
	case modeImportList:
		return []key.Binding{m.keys.mark, m.keys.markAll, importHint}, []key.Binding{moveHint, backHint, m.keys.log}
	case modeImportFix:
		return []key.Binding{fixHint, fieldHint}, []key.Binding{backHint}
	case modeImportBrowse:
		return []key.Binding{chooseHint, openHint, upHint, m.keys.dotfiles}, []key.Binding{moveHint, backHint, m.keys.log}
	case modeTheme:
		return []key.Binding{keepHint, restoreHint}, []key.Binding{moveHint, m.keys.log}
	case modeLog:
		return []key.Binding{scrollHint}, []key.Binding{m.keys.closeView}
	}
	return m.screenHints()
}

// count is n and its label in st, or in the dim style when n is zero.
func (m Model) count(n int, label string, st lipgloss.Style) string {
	return m.countStyle(n, st).Render(fmt.Sprintf("%d %s", n, label))
}

// countStyle is st for a count of n, or the dim style when n is zero.
func (m Model) countStyle(n int, st lipgloss.Style) lipgloss.Style {
	if n == 0 {
		return m.styles.dim
	}
	return st
}

// tally counts the invalid vault skills, and the skills that need attention.
func (m Model) tally() (invalid, attention int) {
	for _, s := range m.vaultSkills {
		if !s.Valid {
			invalid++
		}
	}
	for _, name := range m.names {
		if m.needsAttention(name) {
			attention++
		}
	}
	return invalid, attention
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

// layoutWidth is the width the bar and the screen bodies are laid out in. Before
// the first size message nothing is cut, so it is wide enough for any text.
func (m Model) layoutWidth() int {
	if m.width == 0 {
		return 1 << 16
	}
	return m.width
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
	case ops.OpCreate, opScaffold:
		return "create " + a.skill
	case ops.OpImport:
		return "import " + a.src
	case opConfigInit:
		return "create config " + a.path
	case opVaultInit:
		return "create vault " + a.path
	case opEdit:
		return "open " + a.path
	case ops.OpAdopt:
		return "adopt " + adoptList(a.adopts)
	}
	return fmt.Sprintf("%s %s in %s", a.op, a.skill, agentIDs(a.agents))
}

// adoptList names the copies of an adopt as "skill in agent", comma-separated.
func adoptList(groups []adoptGroup) string {
	var parts []string
	for _, g := range groups {
		for _, s := range g.skills {
			parts = append(parts, s+" in "+g.from.ID)
		}
	}
	return strings.Join(parts, ", ")
}

// effect says what a confirmed action does to local state. A force prompt
// states what is lost.
func effect(a action) string {
	switch a.op {
	case opDiscard:
		return "The text you wrote is lost."
	case opConfigInit:
		return "Writes a starter config with the built-in agents. n keeps the defaults for this run."
	case opVaultInit:
		return "Creates sapato.toml and skills/ there. Any other key exits."
	case ops.OpRemove:
		if a.force {
			return "Deletes the vault copy; installed copies become orphaned."
		}
		return "Deletes the skill from the vault."
	case ops.OpCreate, ops.OpImport:
		return "Overwrites the same-named skill in the vault."
	case opScaffold:
		return "Overwrites the same-named skill and drops its compatible list."
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
		if names := adoptNames(a); len(names) == 1 {
			return fmt.Sprintf("Replaces the vault copy of %s with this folder.", names[0])
		}
		return "Replaces the vault copies of the marked skills with their folders."
	case opEdit:
		return "The skill is already saved. The editor keeps the terminal until it exits."
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

// compatText is a compatible list as the details pane shows it: its agents, or
// "all agents" when list is nil.
func compatText(list []string) string {
	if list == nil {
		return "all agents"
	}
	return strings.Join(list, ", ")
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

// capitalize upper-cases the first letter of s.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
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
