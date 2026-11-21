package tui

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"sapato/internal/ops"
)

// The skill screen of spec §9.3: a sidebar with one row per enabled agent, and
// beside it the details of the open skill for the agent under the cursor. Below
// narrowW columns the sidebar stacks above the details.

const (
	// narrowW is the terminal width below which the panes stack.
	narrowW = 70
	// sidebarMinW is the least width of the sidebar, borders included.
	sidebarMinW = 26
	// dateFormat shows the installed time of a lock entry.
	dateFormat = "2006-01-02 15:04"
	// labelW is the column the field labels of the details pane share: the longest
	// label, version, and a gap of two.
	labelW = len("version") + 2
)

// paneSize is the outer size of a pane, borders included.
type paneSize struct{ w, h int }

// skillPanes is the size of the sidebar and of the details pane.
type skillPanes struct {
	stacked bool
	side    paneSize
	detail  paneSize
}

// seg is a piece of a line: its text and the style it is drawn in.
type seg struct {
	text  string
	style lipgloss.Style
}

// treeNode is a file or a folder of the skill's file tree. A file has its
// entry; a folder has none.
type treeNode struct {
	name string
	file *ops.File
	kids []*treeNode
}

// skillWidth is the width the skill screen lays out in. Before the terminal
// reports its size it is 80 columns.
func (m Model) skillWidth() int {
	if m.width == 0 {
		return 80
	}
	return m.width
}

// skillLayout sizes the panes. A wide terminal shows the sidebar beside the
// details, both as tall as the body. A narrow one stacks them, the sidebar taking
// at most half the body. Before the terminal reports its height, each pane is as
// tall as its content.
func (m Model) skillLayout() skillPanes {
	w, h := m.skillWidth(), m.bodyHeight()
	idW, wordW := m.sideWidths()
	natSide := max(2, len(m.agents)+2)
	if w < narrowW {
		full := w - 2
		if h == 0 {
			natDetail := len(m.detailLines(full-4)) + 2
			return skillPanes{stacked: true, side: paneSize{full, natSide}, detail: paneSize{full, natDetail}}
		}
		side := min(natSide, max(3, h/2))
		return skillPanes{stacked: true, side: paneSize{full, side}, detail: paneSize{full, max(2, h-side)}}
	}
	// A row is 8 columns of marks, gaps and the status glyph, then the id and the
	// word. The borders and a column of padding each side add 4.
	sideW := min(max(sidebarMinW, 8+idW+wordW+4), w/2)
	detailW := w - sideW - 3
	if h == 0 {
		natural := max(natSide, len(m.detailLines(detailW-4))+2)
		return skillPanes{side: paneSize{sideW, natural}, detail: paneSize{detailW, natural}}
	}
	return skillPanes{side: paneSize{sideW, h}, detail: paneSize{detailW, h}}
}

// skillBody is the skill screen: the sidebar and the details pane side by side, or
// stacked below narrowW. Each pane is cut to its rows, and the details scroll.
func (m Model) skillBody() []string {
	lay := m.skillLayout()
	side := m.pane("AGENTS", m.styles.focus, lay.side, window(m.sideLines(lay.side.w-4), m.agentTop, lay.side.h-2))
	det := m.pane(m.open, m.styles.border, lay.detail, window(m.detailLines(lay.detail.w-4), m.detailTop, lay.detail.h-2))
	var out []string
	if lay.stacked {
		for _, l := range append(side, det...) {
			out = append(out, " "+l+" ")
		}
		return out
	}
	// A column of margin on each side, and one between the panes.
	for i := range side {
		out = append(out, " "+side[i]+" "+det[i]+" ")
	}
	return out
}

// sideWidths is the width of the agent ID column and of the status word column of
// the sidebar.
func (m Model) sideWidths() (idW, wordW int) {
	for _, a := range m.agents {
		idW = max(idW, lipgloss.Width(a.ID))
	}
	wordW = lipgloss.Width("absent")
	for _, st := range states {
		wordW = max(wordW, lipgloss.Width(shortWord(st.state)))
	}
	return idW, wordW
}

// shortWord is the status word of a sidebar row.
func shortWord(s ops.State) string {
	if s == ops.NotInstalled {
		return "not inst."
	}
	return string(s)
}

// sideLines lists one row per enabled agent, each cut and padded to w columns.
func (m Model) sideLines(w int) []string {
	idW, wordW := m.sideWidths()
	lines := make([]string, len(m.agents))
	for i := range m.agents {
		lines[i] = m.sideRow(i, idW, wordW, w)
	}
	return lines
}

// sideRow is one agent of the sidebar: its mark, the cursor marker, the agent's ID,
// and the status icon and word. The cursor's row is drawn in the row style.
func (m Model) sideRow(i, idW, wordW, w int) string {
	a := m.agents[i]
	selected := i == m.cur
	mark, marker := "  ", "  "
	if m.marks[a.ID] {
		mark = pad(m.icons.Mark, 2)
	}
	if selected {
		marker = pad(m.icons.Marker, 2)
	}
	glyph, word, style := " ", "absent", m.styles.dim
	if state, ok := m.cells[cell{m.open, a.ID}]; ok {
		glyph, word, style = m.icons.Status[state], shortWord(state), m.styles.glyph[state]
	}
	idStyle := lipgloss.NewStyle()
	if selected {
		idStyle = m.styles.bold
	}
	return m.draw(w, selected,
		seg{mark + marker, m.styles.accent},
		seg{pad(a.ID, idW), idStyle},
		seg{"  ", lipgloss.NewStyle()},
		seg{glyph + " " + pad(word, wordW), style},
	)
}

// detailLines is the details pane of the open skill, one string per line, each cut
// and padded to w columns: the info line, the description, the vault path and its
// problems, the cursor agent's section, then the files.
func (m Model) detailLines(w int) []string {
	var lines []string
	add := func(segs ...seg) {
		lines = append(lines, m.draw(w, false, segs...))
	}
	plain := lipgloss.NewStyle()
	dim := m.styles.dim

	s, inVault := m.vaultSkills[m.open]
	if inVault {
		hash := "-"
		if s.Hash != "" {
			hash = shortHash(s.Hash)
		}
		status := seg{m.icons.Status[ops.Invalid] + " invalid", m.styles.fail}
		if s.Valid {
			status = seg{m.icons.Status[ops.Synced] + " valid", m.styles.glyph[ops.Synced]}
		}
		info := []seg{
			{pad("version", labelW), dim}, {orDash(s.Version), plain}, {"   ", plain},
			{pad("hash", labelW), dim}, {hash, plain},
		}
		// The status goes on its own line when the info line would cut it.
		if segsWidth(info)+3+lipgloss.Width(status.text) <= w {
			add(append(info, seg{"   ", plain}, status)...)
		} else {
			add(info...)
			add(status)
		}
		if s.Description == "" {
			add(seg{"no description", dim})
		}
		for _, l := range wrapText(s.Description, w) {
			add(seg{l, plain})
		}
		add(seg{pad("vault", labelW), dim}, seg{orDash(s.Dir), plain})
		if !s.Valid {
			for _, p := range s.Problems {
				add(seg{"  " + p, m.styles.fail})
			}
		}
	} else {
		add(seg{pad("version", labelW), dim}, seg{"-   ", plain}, seg{pad("hash", labelW), dim}, seg{"-", plain})
		add(seg{pad("vault", labelW), dim}, seg{"not in vault", dim})
	}

	lines = append(lines, "")
	if len(m.agents) == 0 {
		add(seg{"No enabled agents. Enable one in the config.", dim})
	} else {
		a := m.agents[m.cur]
		glyph, word, style := " ", "absent", dim
		state, has := m.cells[cell{m.open, a.ID}]
		if has {
			glyph, word, style = m.icons.Status[state], string(state), m.styles.glyph[state]
		}
		segs := []seg{{a.ID + "  ", m.styles.bold}, {glyph + " " + word, style}}
		if d, ok := m.shownDetail(); ok && d.managed {
			segs = append(segs, seg{" · installed " + d.entry.InstalledAt.Local().Format(dateFormat), dim})
		}
		add(segs...)
		if hasFolder(state) {
			add(seg{"  ", plain}, seg{filepath.Join(a.Path, m.open), dim})
		}
	}

	lines = append(lines, "")
	d, loaded := m.shownDetail()
	title := "FILES"
	if loaded && len(d.files) > 0 {
		title = fmt.Sprintf("FILES (%d", len(d.files))
		if d.installed != "" {
			title += ", installed in " + d.installed
		}
		title += ")"
	}
	add(seg{title, m.styles.bold})
	switch {
	case !loaded:
		add(seg{"loading…", dim})
	case len(d.files) == 0:
		add(seg{"no files", dim})
	default:
		lines = append(lines, m.treeLines(w, d.files)...)
	}
	return lines
}

// shownDetail is the detail loaded for the open skill and the agent under the
// cursor. ok is false while that detail is still loading.
func (m Model) shownDetail() (d detail, ok bool) {
	return m.detail, m.open != "" && m.detail.skill == m.open && m.detail.agent == m.cursorID()
}

// hasFolder reports whether an agent holds a folder for a skill. That is every status
// but not-installed, invalid and missing.
func hasFolder(s ops.State) bool {
	switch s {
	case ops.Synced, ops.Outdated, ops.Modified, ops.Orphaned, ops.Unmanaged:
		return true
	}
	return false
}

// wrapText wraps s to w columns, in at most three lines. When the text runs past
// the third line, that line ends with an ellipsis.
func wrapText(s string, w int) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(lipgloss.NewStyle().Width(max(1, w)).Render(s), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	if len(lines) > 3 {
		lines = append(lines[:2], ellipsis(strings.Join(lines[2:], " "), w))
	}
	return lines
}

// ellipsis cuts s to fit in w columns and ends it with an ellipsis, even when s
// already fits.
func ellipsis(s string, w int) string {
	if w <= 0 {
		return ""
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes))+lipgloss.Width("…") > w {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

// treeLines lists the files as a tree. A folder is a line with name/, and its
// children follow two columns in. Folders come before files, each sorted by name.
// A file shows its exec glyph and its size, right-aligned to w.
func (m Model) treeLines(w int, files []ops.File) []string {
	var lines []string
	var walk func(n *treeNode, depth int)
	walk = func(n *treeNode, depth int) {
		indent := strings.Repeat("  ", depth)
		for _, k := range slices.SortedFunc(slices.Values(n.kids), treeOrder(depth == 0)) {
			if k.file == nil {
				lines = append(lines, m.draw(w, false, seg{indent + k.name + "/", lipgloss.NewStyle()}))
				walk(k, depth+1)
				continue
			}
			lines = append(lines, m.fileLine(w, indent, k))
		}
	}
	walk(fileTree(files), 0)
	return lines
}

// fileTree builds the tree of files, one node for each element of a path.
func fileTree(files []ops.File) *treeNode {
	root := &treeNode{}
	for _, f := range files {
		parts := strings.Split(f.Path, "/")
		n := root
		for _, dir := range parts[:len(parts)-1] {
			n = n.folder(dir)
		}
		n.kids = append(n.kids, &treeNode{name: parts[len(parts)-1], file: &f})
	}
	return root
}

// folder is the sub-folder of n called name. It is made when n has none.
func (n *treeNode) folder(name string) *treeNode {
	for _, k := range n.kids {
		if k.file == nil && k.name == name {
			return k
		}
	}
	k := &treeNode{name: name}
	n.kids = append(n.kids, k)
	return k
}

// treeOrder orders the children of a folder: at the root, SKILL.md comes first, then
// folders, then files, each group sorted by name.
func treeOrder(root bool) func(a, b *treeNode) int {
	return func(a, b *treeNode) int {
		if c := cmp.Compare(treeRank(a, root), treeRank(b, root)); c != 0 {
			return c
		}
		return strings.Compare(a.name, b.name)
	}
}

// treeRank is where a child sorts among its siblings: 0 for the root's SKILL.md, 1
// for a folder, 2 for any other file.
func treeRank(n *treeNode, root bool) int {
	switch {
	case n.file == nil:
		return 1
	case root && n.name == "SKILL.md":
		return 0
	}
	return 2
}

// segsWidth is the width of segs laid end to end.
func segsWidth(segs []seg) int {
	w := 0
	for _, s := range segs {
		w += lipgloss.Width(s.text)
	}
	return w
}

// fileLine is one file of the tree at indent: its name, its exec glyph when it is
// executable, and its size, right-aligned to w. A long name gives way to the size.
func (m Model) fileLine(w int, indent string, n *treeNode) string {
	size := humanSize(n.file.Size)
	tail := ""
	if n.file.Exec {
		tail = " " + m.icons.Exec
	}
	name := n.name
	if room := w - lipgloss.Width(indent+tail) - lipgloss.Width(size) - 1; lipgloss.Width(name) > room {
		name = truncate(name, max(1, room))
	}
	gap := max(1, w-lipgloss.Width(indent+name+tail)-lipgloss.Width(size))
	return m.draw(w, false,
		seg{indent + name, lipgloss.NewStyle()},
		seg{tail, m.styles.accent},
		seg{strings.Repeat(" ", gap), lipgloss.NewStyle()},
		seg{size, m.styles.dim},
	)
}

// humanSize is a size in bytes as the skill screen shows it: plain bytes below one
// KiB, then KiB and MiB with one decimal.
func humanSize(n int64) string {
	switch {
	case n < 1<<10:
		return fmt.Sprintf("%d B", n)
	case n < 1<<20:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
}

// draw lays out segs in order, cut to w columns. A segment that runs past w is cut
// with an ellipsis, and the segments after it are left out. The line is padded to
// w. A selected line is drawn in the row style, so its highlight spans the width.
func (m Model) draw(w int, selected bool, segs ...seg) string {
	var b strings.Builder
	used := 0
	for _, s := range segs {
		text := truncate(s.text, w-used)
		b.WriteString(m.on(selected, s.style).Render(text))
		used += lipgloss.Width(text)
		if text != s.text {
			break
		}
	}
	b.WriteString(m.spaces(max(0, w-used), selected))
	return b.String()
}

// pane draws s as a rounded box, with title in its top border and lines inside it,
// a column in from each side. Lines past the box's rows are left out, and missing
// rows are blank. The box is exactly s wide.
func (m Model) pane(title string, border lipgloss.Style, s paneSize, lines []string) []string {
	rb := lipgloss.RoundedBorder()
	inner := max(0, s.w-2)
	cw := max(0, inner-2)
	t := truncate(title, max(0, s.w-5))
	fill := max(0, s.w-5-lipgloss.Width(t))
	out := []string{
		border.Render(rb.TopLeft+rb.Top) + m.styles.bold.Render(" "+t+" ") +
			border.Render(strings.Repeat(rb.Top, fill)+rb.TopRight),
	}
	for i := range max(0, s.h-2) {
		text := ""
		if i < len(lines) {
			text = clip(lines[i], cw)
		}
		body := text + strings.Repeat(" ", max(0, cw-lipgloss.Width(text)))
		out = append(out, border.Render(rb.Left)+" "+body+" "+border.Render(rb.Right))
	}
	return append(out, border.Render(rb.BottomLeft+strings.Repeat(rb.Bottom, inner)+rb.BottomRight))
}

// window is rows lines of lines from top. The window moves up when top would run
// past the end, so it never shows blank rows past the last line.
func window(lines []string, top, rows int) []string {
	rows = max(0, rows)
	top = max(0, min(top, len(lines)-rows))
	return lines[top:min(len(lines), top+rows)]
}
