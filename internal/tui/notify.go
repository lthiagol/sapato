package tui

import (
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"sapato/internal/ops"
)

// The notifications are the messages of spec §9.6: every action result, error and
// info. The body shows the newest of them as boxes over its bottom-right corner,
// and LOG lists them all.

// noteKind is the kind of a notification. It picks the color and the icon.
type noteKind int

const (
	noteSuccess noteKind = iota
	noteError
	noteInfo
)

const (
	successLife = 4 * time.Second
	errorLife   = 10 * time.Second
	// maxNotes is how many notifications LOG keeps.
	maxNotes = 200
	// maxToasts is how many boxes the body shows at once.
	maxToasts = 3
	// toastLines is the most message lines a box shows.
	toastLines = 4
	toastMaxW  = 48
	toastMinW  = 24
	// sgrReset ends the styles of the body text left of a box. NO_COLOR writes none.
	sgrReset = "\x1b[0m"
)

// notification is one message: its kind, a short title such as "installed", the
// message, and when it was pushed.
type notification struct {
	kind    noteKind
	title   string
	message string
	at      time.Time
}

// lifetime is how long the body shows a notification. Errors stay longer.
func (n notification) lifetime() time.Duration {
	if n.kind == noteError {
		return errorLife
	}
	return successLife
}

// doneTitles names a finished action in the title of its notification.
var doneTitles = map[string]string{
	ops.OpInstall:   "installed",
	ops.OpUninstall: "uninstalled",
	ops.OpUpdate:    "updated",
	ops.OpAdopt:     "adopted",
	ops.OpRemove:    "removed",
	ops.OpCreate:    "created",
	ops.OpImport:    "imported",
	opExport:        "exported",
	opScaffold:      "created",
}

// doneTitle is the title of the notification of an applied action.
func doneTitle(op string) string {
	if t, ok := doneTitles[op]; ok {
		return t
	}
	return op
}

// notify pushes a notification stamped with the model's clock. Newlines in text
// become spaces, so a message stays on its lines.
func (m *Model) notify(kind noteKind, title, text string) {
	m.notes = append(m.notes, notification{
		kind:    kind,
		title:   title,
		message: strings.ReplaceAll(text, "\n", " "),
		at:      m.now(),
	})
	if len(m.notes) > maxNotes {
		m.notes = m.notes[len(m.notes)-maxNotes:]
	}
	m.pushed++
}

// repaintMsg says that a notification may have run out. Bubble Tea redraws after
// every message, so the message needs no handling.
type repaintMsg struct{}

// repaint is the tick function that sends repaintMsg.
func repaint(time.Time) tea.Msg {
	return repaintMsg{}
}

// expiries returns a tick for each notification pushed since before, so the body
// drops it when its time runs out, without a key press.
func (m Model) expiries(before int) tea.Cmd {
	fresh := min(m.pushed-before, len(m.notes))
	var cmds []tea.Cmd
	for _, n := range m.notes[len(m.notes)-fresh:] {
		cmds = append(cmds, m.tick(n.lifetime(), repaint))
	}
	return tea.Batch(cmds...)
}

// toasts are the notifications the body shows: the newest maxToasts that have not
// run out, oldest first.
func (m Model) toasts() []notification {
	now := m.now()
	var live []notification
	for _, n := range m.notes {
		if now.Sub(n.at) < n.lifetime() {
			live = append(live, n)
		}
	}
	return live[max(0, len(live)-maxToasts):]
}

// overlay draws the toasts over the bottom-right corner of body, the newest on the
// last line, one column clear of the right edge. Each line keeps its text left of
// the boxes and stays as wide as the terminal.
func (m Model) overlay(body []string) []string {
	ts := m.toasts()
	boxW := min(toastMaxW, max(toastMinW, m.width/2))
	if len(ts) == 0 || boxW+1 > m.width {
		return body
	}
	var stack []string
	for _, n := range ts {
		stack = append(stack, m.toast(n, boxW)...)
	}
	stack = stack[max(0, len(stack)-len(body)):]
	top := len(body) - len(stack)
	x := m.width - 1 - boxW
	reset := sgrReset
	if m.styles.noColor {
		reset = ""
	}
	out := slices.Clone(body)
	for i, box := range stack {
		out[top+i] = splice(body[top+i], box, reset, x)
	}
	return out
}

// splice draws box over line from column x on. The text of line left of x stays,
// preceded by reset, and one blank column follows the box.
func splice(line, box, reset string, x int) string {
	return pad(clip(line, x), x) + reset + box + " "
}

// toast draws one notification as a rounded box w columns wide. The top border
// carries the icon and title, and the message wraps inside it.
func (m Model) toast(n notification, w int) []string {
	st := m.noteStyle(n.kind)
	title := truncate(" "+m.noteIcon(n.kind)+" "+n.title+" ", w-4)
	lines := []string{st.Render("╭─" + title + strings.Repeat("─", w-3-lipgloss.Width(title)) + "╮")}
	for _, l := range wrapNote(n.message, w-4) {
		lines = append(lines, st.Render("│")+" "+pad(l, w-4)+" "+st.Render("│"))
	}
	return append(lines, st.Render("╰"+strings.Repeat("─", w-2)+"╯"))
}

// wrapNote wraps text to w columns and to toastLines lines. A text that runs
// longer ends its last line with an ellipsis.
func wrapNote(text string, w int) []string {
	raw := strings.Split(lipgloss.NewStyle().Width(w).Render(text), "\n")
	lines := make([]string, len(raw))
	for i, l := range raw {
		lines[i] = strings.TrimRight(l, " ")
	}
	if len(lines) <= toastLines {
		return lines
	}
	last := truncate(strings.Join(lines[toastLines-1:], " "), w)
	return append(lines[:toastLines-1], last)
}

// noteIcon is the icon of a notification kind: the synced icon, the invalid icon, or
// the info icon.
func (m Model) noteIcon(kind noteKind) string {
	switch kind {
	case noteSuccess:
		return m.icons.Status[ops.Synced]
	case noteError:
		return m.icons.Status[ops.Invalid]
	}
	return m.icons.Info
}

// noteStyle paints a notification kind: green, red or accent.
func (m Model) noteStyle(kind noteKind) lipgloss.Style {
	switch kind {
	case noteSuccess:
		return m.styles.ok
	case noteError:
		return m.styles.fail
	}
	return m.styles.info
}

// logOpensFrom lists the modes L opens LOG from. The confirm reads any key as an
// answer, and the text inputs take L as text, so neither lists it.
func logOpensFrom(md mode) bool {
	switch md {
	case modeNormal, modeDiff, modeHelp, modeAdopt, modeImportList, modeImportBrowse, modeTheme:
		return true
	}
	return false
}

// openLog shows LOG over the current mode, scrolled to the newest entry.
func (m Model) openLog() Model {
	m.logBack = m.mode
	m.mode = modeLog
	m.logTop = m.logMaxTop()
	return m
}

// logRows is how many entries fit under the title and its gap. Before the first
// size message it is every entry.
func (m Model) logRows() int {
	if m.height == 0 {
		return max(1, len(m.notes))
	}
	return max(1, m.bodyHeight()-2)
}

// logMaxTop is the first entry shown when the newest entry is the last row.
func (m Model) logMaxTop() int {
	return max(0, len(m.notes)-m.logRows())
}

// logKey scrolls LOG, and closes it back to the mode it opened over.
func (m Model) logKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	k := m.keys
	last := m.logMaxTop()
	rows := m.logRows()
	switch {
	case key.Matches(msg, k.closeView):
		m.mode = m.logBack
	case key.Matches(msg, k.up):
		m.logTop = clampIndex(m.logTop-1, last+1)
	case key.Matches(msg, k.down):
		m.logTop = clampIndex(m.logTop+1, last+1)
	case key.Matches(msg, k.pageUp):
		m.logTop = clampIndex(m.logTop-rows, last+1)
	case key.Matches(msg, k.pageDown):
		m.logTop = clampIndex(m.logTop+rows, last+1)
	case key.Matches(msg, k.top):
		m.logTop = 0
	case key.Matches(msg, k.bottom):
		m.logTop = last
	}
	return m, nil
}

// logBody is LOG: the notifications from logTop, oldest first.
func (m Model) logBody(w int) []string {
	lines := []string{m.styles.bold.Render(truncate("Log", w)), ""}
	if len(m.notes) == 0 {
		return append(lines, m.styles.dim.Render(truncate("no notifications yet", w)))
	}
	end := min(len(m.notes), m.logTop+m.logRows())
	for _, n := range m.notes[m.logTop:end] {
		lines = append(lines, m.logLine(n, w))
	}
	return lines
}

// logLine is one entry, HH:MM:SS icon title: message, cut to w columns.
func (m Model) logLine(n notification, w int) string {
	head := n.at.Format("15:04:05") + " " + m.noteStyle(n.kind).Render(m.noteIcon(n.kind)) + " " + n.title + ": "
	return clip(head+truncate(n.message, w-lipgloss.Width(head)), w)
}
