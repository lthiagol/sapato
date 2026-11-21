package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"sapato/internal/config"
	"sapato/internal/ops"
)

// fakeClock is the clock of the notifications, moved by hand.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

// clockModel returns the layout model at w×h, with its notifications stamped by a
// clock that starts at 10:00:00 and only moves when the test moves it.
func clockModel(t *testing.T, w, h int) (Model, *fakeClock) {
	t.Helper()
	m, _ := layoutModel(t, w, h)
	c := &fakeClock{now: time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)}
	m.now = c.Now
	return m, c
}

// toastCol is the column where a toast starts on a w-column terminal: the box, then
// one column clear of the right edge.
func toastCol(w int) int {
	return w - 1 - min(48, max(24, w/2))
}

// sgr matches the color codes that the styles write.
var sgr = regexp.MustCompile("\x1b\\[[0-9;]*m")

// plainLines splits view into lines and drops their color codes, so their columns
// can be counted. The layouts under test are drawn without color.
func plainLines(view string) []string {
	lines := strings.Split(view, "\n")
	for i, l := range lines {
		lines[i] = sgr.ReplaceAllString(l, "")
	}
	return lines
}

// runesAt returns s as runes, padded with spaces to n.
func runesAt(s string, n int) []rune {
	r := []rune(s)
	for len(r) < n {
		r = append(r, ' ')
	}
	return r
}

// lineIndex returns the index of the first line that contains s, or -1.
func lineIndex(lines []string, s string) int {
	for i, l := range lines {
		if strings.Contains(l, s) {
			return i
		}
	}
	return -1
}

func TestToastSitsBottomRightAboveTheBar(t *testing.T) {
	m, _ := clockModel(t, 80, 24)
	before := plainLines(m.View())
	m.notify(noteSuccess, "installed", "install alpha in claude")
	after := plainLines(m.View())

	x := toastCol(80)
	top, msg, bottom := after[len(after)-5], after[len(after)-4], after[len(after)-3]
	if r := runesAt(top, 80); r[x] != '╭' || r[78] != '╮' || r[79] != ' ' {
		t.Errorf("top border is not at columns %d-78 with a blank column 79: %q", x, top)
	}
	if !strings.Contains(top, "✔ installed") {
		t.Errorf("top border lacks the icon and title: %q", top)
	}
	if !strings.Contains(msg, "install alpha in claude") {
		t.Errorf("message row lacks the message: %q", msg)
	}
	if want := strings.Repeat(" ", x) + "╰" + strings.Repeat("─", 38) + "╯ "; bottom != want {
		t.Errorf("bottom border = %q, want %q", bottom, want)
	}
	for i, line := range after {
		if got, want := string(runesAt(line, x)[:x]), string(runesAt(before[i], x)[:x]); got != want {
			t.Errorf("line %d left of the toast = %q, want %q", i, got, want)
		}
		if i >= len(after)-5 && i <= len(after)-3 {
			if w := lipgloss.Width(line); w != 80 {
				t.Errorf("line %d is %d columns with the toast, want 80: %q", i, w, line)
			}
		}
	}
}

func TestToastsShowTheNewestThreeOldestFirst(t *testing.T) {
	m, _ := clockModel(t, 120, 40)
	for _, msg := range []string{"first note", "second note", "third note", "fourth note"} {
		m.notify(noteSuccess, "installed", msg)
	}
	lines := strings.Split(m.View(), "\n")
	if i := lineIndex(lines, "first note"); i >= 0 {
		t.Errorf("the oldest toast still shows at line %d", i)
	}
	prev := -1
	for _, msg := range []string{"second note", "third note", "fourth note"} {
		i := lineIndex(lines, msg)
		if i <= prev {
			t.Errorf("%q at line %d, want below the older toast at %d", msg, i, prev)
		}
		prev = i
	}
}

func TestLongMessageWrapsToFourLines(t *testing.T) {
	m, _ := clockModel(t, 80, 24)
	m.notify(noteError, "error", strings.Repeat("word ", 60))
	lines := plainLines(m.View())
	x := toastCol(80)
	top := -1
	for i, l := range lines {
		if runesAt(l, 80)[x] == '╭' {
			top = i
			break
		}
	}
	if top < 0 {
		t.Fatalf("no toast top border:\n%s", m.View())
	}
	var body []string
	for i := top + 1; i < len(lines) && runesAt(lines[i], 80)[x] != '╰'; i++ {
		body = append(body, lines[i])
	}
	if len(body) != 4 {
		t.Fatalf("message takes %d lines, want 4:\n%s", len(body), m.View())
	}
	if last := string(runesAt(body[3], 80)[x:]); !strings.Contains(last, "…") {
		t.Errorf("last message line lacks an ellipsis: %q", last)
	}
}

func TestNotificationLifetimes(t *testing.T) {
	tests := []struct {
		name  string
		kind  noteKind
		after time.Duration
		shown bool
	}{
		{name: "success shows before 4s", kind: noteSuccess, after: 3999 * time.Millisecond, shown: true},
		{name: "success is gone at 4s", kind: noteSuccess, after: 4 * time.Second},
		{name: "info is gone at 4s", kind: noteInfo, after: 4 * time.Second},
		{name: "error shows before 10s", kind: noteError, after: 9999 * time.Millisecond, shown: true},
		{name: "error is gone at 10s", kind: noteError, after: 10 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, c := clockModel(t, 120, 40)
			m.notify(tt.kind, "title", "lifetime note")
			c.now = c.now.Add(tt.after)
			if got := strings.Contains(m.View(), "lifetime note"); got != tt.shown {
				t.Errorf("shown = %t after %v, want %t:\n%s", got, tt.after, tt.shown, m.View())
			}
		})
	}
}

func TestNotificationsScheduleTheirRepaint(t *testing.T) {
	tests := []struct {
		name string
		rows []ops.Row
		keys []string
		want time.Duration
	}{
		{name: "a kept theme is a success", rows: []ops.Row{row("alpha", "claude", ops.Synced)}, keys: []string{"t", "enter"}, want: 4 * time.Second},
		{name: "an empty grid is an error", keys: []string{"enter"}, want: 10 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := loaded(t, &fakeBackend{rows: tt.rows}, []config.Agent{claude})
			var lifetimes []time.Duration
			m.tick = func(d time.Duration, _ func(time.Time) tea.Msg) tea.Cmd {
				lifetimes = append(lifetimes, d)
				return nil
			}
			press(t, m, tt.keys...)
			if len(lifetimes) != 1 || lifetimes[0] != tt.want {
				t.Errorf("repaints scheduled at %v, want [%v]", lifetimes, tt.want)
			}
		})
	}
}

func TestNotificationsKeepTheNewest200(t *testing.T) {
	m, _ := layoutModel(t, 80, 24)
	for i := range maxNotes + 5 {
		m.notify(noteInfo, "info", fmt.Sprintf("note %03d", i))
	}
	if len(m.notes) != maxNotes {
		t.Fatalf("kept %d notifications, want %d", len(m.notes), maxNotes)
	}
	if first := m.notes[0].message; first != "note 005" {
		t.Errorf("oldest kept = %q, want note 005", first)
	}
	if m.pushed != maxNotes+5 {
		t.Errorf("pushed = %d, want %d", m.pushed, maxNotes+5)
	}
}

func TestLogListsOldestFirstWithTheTimeAndIcon(t *testing.T) {
	m, c := clockModel(t, 120, 40)
	m.notify(noteSuccess, "installed", "alpha note")
	m.notify(noteError, "error", "beta note")
	m.notify(noteInfo, "info", "gamma note")
	c.now = c.now.Add(time.Minute)
	m = press(t, m, "L")
	if m.mode != modeLog {
		t.Fatalf("mode = %v, want LOG", m.mode)
	}
	lines := strings.Split(m.View(), "\n")
	alpha := lineIndex(lines, "10:00:00 ✔ installed: alpha note")
	beta := lineIndex(lines, "10:00:00 ✖ error: beta note")
	gamma := lineIndex(lines, "10:00:00 ℹ info: gamma note")
	if alpha < 0 || beta <= alpha || gamma <= beta {
		t.Errorf("LOG lines at %d, %d, %d; want each entry below the one before:\n%s", alpha, beta, gamma, m.View())
	}
}

func TestLogScrollsWithinTheEntriesAndCloses(t *testing.T) {
	m, c := clockModel(t, 80, 24)
	for i := range 30 {
		m.notify(noteSuccess, "installed", fmt.Sprintf("note %02d", i))
	}
	c.now = c.now.Add(time.Minute)
	m = press(t, m, "L")
	if view := m.View(); !strings.Contains(view, "note 29") || strings.Contains(view, "note 00") {
		t.Errorf("LOG opens at the newest entry; want note 29 shown and note 00 scrolled off:\n%s", view)
	}
	m = press(t, m, "g")
	if view := m.View(); !strings.Contains(view, "note 00") || !strings.Contains(view, "note 17") || strings.Contains(view, "note 18") {
		t.Errorf("after g: want note 00 to note 17 shown:\n%s", view)
	}
	m = press(t, m, "G")
	if view := m.View(); !strings.Contains(view, "note 29") {
		t.Errorf("after G: want note 29 shown:\n%s", view)
	}
	m = press(t, m, "esc")
	if m.mode != modeNormal {
		t.Errorf("after esc: mode = %v, want the grid", m.mode)
	}
}

func TestLogOpensOverTheModeItCameFrom(t *testing.T) {
	m, _ := layoutModel(t, 120, 40)
	m = press(t, m, "?", "L")
	if m.mode != modeLog {
		t.Fatalf("L over help: mode = %v, want LOG", m.mode)
	}
	if m = press(t, m, "esc"); m.mode != modeHelp {
		t.Errorf("esc from LOG over help: mode = %v, want help", m.mode)
	}
}

func TestLogIsIgnoredWhereItIsText(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want mode
	}{
		{name: "NEW editor", keys: []string{"n", "L"}, want: modeNew},
		{name: "IMPORT path", keys: []string{"I", "L"}, want: modeImport},
		{name: "filter", keys: []string{"/", "L"}, want: modeFilter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := layoutModel(t, 120, 40)
			if m = press(t, m, tt.keys...); m.mode != tt.want {
				t.Errorf("mode = %v, want %v: L must type into the input", m.mode, tt.want)
			}
		})
	}
}

func TestLogKeyIsInEveryScreensHints(t *testing.T) {
	m, _ := layoutModel(t, 120, 40)
	if hints := joinHints(m.hints()); !strings.Contains(hints, "L log") {
		t.Errorf("grid hints lack L log: %q", hints)
	}
	m = press(t, m, "enter")
	if hints := joinHints(m.hints()); !strings.Contains(hints, "L log") {
		t.Errorf("skill hints lack L log: %q", hints)
	}
}

// TestToastsWriteAResetOnlyWithColor checks that a box is preceded by the reset that
// ends the body's styles, except under NO_COLOR, which has no styles to end.
func TestToastsWriteAResetOnlyWithColor(t *testing.T) {
	tests := []struct {
		name    string
		noColor bool
		want    bool
	}{
		{name: "color", noColor: false, want: true},
		{name: "NO_COLOR", noColor: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := clockModel(t, 80, 24)
			m.styles = newStyles(m.th, tt.noColor)
			m.notify(noteSuccess, "installed", "install alpha in claude")
			out := strings.Join(m.overlay(make([]string, 24)), "\n")
			if got := strings.Contains(out, sgrReset); got != tt.want {
				t.Errorf("reset written = %t, want %t:\n%q", got, tt.want, out)
			}
		})
	}
}

// TestNotificationsFitEveryScreen puts toasts over every screen at each size, and
// checks that the frame still holds the terminal's rows and columns.
func TestNotificationsFitEveryScreen(t *testing.T) {
	for _, size := range layoutSizes {
		for _, sc := range allScreens {
			t.Run(fmt.Sprintf("%s at %dx%d", sc.name, size[0], size[1]), func(t *testing.T) {
				m, b := layoutModel(t, size[0], size[1])
				b.changes = []ops.FileChange{{Rel: "SKILL.md", Kind: ops.ChangeChanged, Diff: "--- a\n+++ b\n@@ -1 +1 @@\n-old\n+new\n"}}
				m.notify(noteError, "error", strings.Repeat("a long failure message ", 20))
				m.notify(noteSuccess, "installed", "install alpha in claude")
				m = press(t, m, sc.steps...)
				lines := strings.Split(m.View(), "\n")
				if len(lines) != size[1] {
					t.Errorf("view has %d lines, want %d", len(lines), size[1])
				}
				for _, line := range lines {
					if w := lipgloss.Width(line); w > size[0] {
						t.Errorf("line is %d columns, want at most %d: %q", w, size[0], line)
					}
				}
			})
		}
	}
}
