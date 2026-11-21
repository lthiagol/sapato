package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"sapato/internal/config"
	"sapato/internal/ops"
	"sapato/internal/tui/icons"
	"sapato/internal/vault"
)

// layoutSizes are the terminal sizes the layout must hold at.
var layoutSizes = [][2]int{{60, 20}, {80, 24}, {120, 40}, {200, 60}}

// layoutModel returns the UI over two agents and a few skills, sized to w×h,
// with the fake backend it reads from.
func layoutModel(t *testing.T, w, h int) (Model, *fakeBackend) {
	t.Helper()
	b := &fakeBackend{rows: []ops.Row{
		row("code-review", "claude", ops.Synced), row("code-review", "cursor", ops.NotInstalled),
		row("obsidian-notes", "claude", ops.Outdated), row("obsidian-notes", "cursor", ops.Synced),
		row("a-rather-long-skill-name-that-keeps-going-on", "claude", ops.Modified),
		row("a-rather-long-skill-name-that-keeps-going-on", "cursor", ops.Missing),
		row("hand-made", "claude", ops.Unmanaged), row("hand-made", "cursor", ops.Orphaned),
	}}
	m := sized(t, loaded(t, b, []config.Agent{claude, cursorAgent}), w, h)
	return m, b
}

// leadingSpaces counts the spaces before the first other column of line.
func leadingSpaces(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

// lineWith returns the first line of view that contains s.
func lineWith(t *testing.T, view, s string) string {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, s) {
			return line
		}
	}
	t.Fatalf("no line contains %q:\n%s", s, view)
	return ""
}

func TestHeaderIsFullOrCompact(t *testing.T) {
	tests := []struct {
		name        string
		w, h        int
		wantCompact bool
	}{
		{name: "tall and wide shows the art", w: 120, h: 40, wantCompact: false},
		{name: "80 by 24 is compact", w: 80, h: 24, wantCompact: true},
		{name: "short terminal is compact", w: 120, h: 29, wantCompact: true},
		{name: "narrower than the art is compact", w: 40, h: 40, wantCompact: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := layoutModel(t, tt.w, tt.h)
			if got := m.compact(); got != tt.wantCompact {
				t.Fatalf("compact() = %t, want %t", got, tt.wantCompact)
			}
			lines := strings.Split(m.View(), "\n")
			if tt.wantCompact {
				if !strings.Contains(lines[0], "sapato") || lines[1] != m.styles.rule.Render(strings.Repeat(m.icons.Rule, tt.w)) {
					t.Errorf("compact header: line 0 %q, line 1 is not the rule", lines[0])
				}
				return
			}
			for i, art := range sapatoArt {
				if got := strings.TrimLeft(lines[i], " "); got != art {
					t.Errorf("art line %d = %q, want %q", i, got, art)
				}
				if got, want := leadingSpaces(lines[i]), (tt.w-artW)/2; got != want {
					t.Errorf("art line %d indent = %d, want %d", i, got, want)
				}
			}
			tag := lines[len(sapatoArt)]
			if got := strings.TrimSpace(tag); got != tagline {
				t.Errorf("tagline line = %q", tag)
			}
			left := leadingSpaces(tag)
			right := tt.w - left - lipgloss.Width(strings.TrimSpace(tag))
			if left-right > 1 || right-left > 1 {
				t.Errorf("tagline margins left %d, right %d; want equal within 1", left, right)
			}
		})
	}
}

func TestCompactHeaderDropsTaglineWhenItDoesNotFit(t *testing.T) {
	m, _ := layoutModel(t, 40, 40)
	if line := m.View(); strings.Contains(strings.Split(line, "\n")[0], "Skills") {
		t.Errorf("header kept the tagline at 40 columns:\n%s", line)
	}
	if line := strings.Split(m.View(), "\n")[0]; !strings.Contains(line, "sapato") {
		t.Errorf("header lacks the name: %q", line)
	}
}

func TestGridIsCentered(t *testing.T) {
	for _, size := range layoutSizes {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m, _ := layoutModel(t, size[0], size[1])
			view := m.View()
			// The cursor row's marker is not a space, so its leading spaces are the
			// indent of the whole grid block.
			line := lineWith(t, view, "▶ a-rather")
			left := leadingSpaces(line)
			block := lipgloss.Width(line) - left
			if right := size[0] - left - block; left-right > 1 || right-left > 1 {
				t.Errorf("grid left margin %d, right %d in %q; want equal within 1", left, right, line)
			}
		})
	}
}

func TestRuleIsFullWidth(t *testing.T) {
	for _, size := range layoutSizes {
		m, _ := layoutModel(t, size[0], size[1])
		lines := strings.Split(m.View(), "\n")
		if got := lipgloss.Width(lines[m.headerHeight()]); got != size[0] {
			t.Errorf("%dx%d: rule is %d columns, want %d", size[0], size[1], got, size[0])
		}
	}
}

// allScreens are the key sequences from the grid that reach each screen and overlay.
var allScreens = []struct {
	name  string
	steps []string
}{
	{name: "main grid"},
	{name: "filter", steps: []string{"/", "o"}},
	{name: "help", steps: []string{"?"}},
	{name: "skill screen", steps: []string{"enter"}},
	{name: "skill screen with a mark", steps: []string{"enter", "down", " "}},
	{name: "confirm", steps: []string{"enter", "u"}},
	{name: "diff", steps: []string{"enter", "d"}},
	{name: "skill help", steps: []string{"enter", "?"}},
	{name: "adopt", steps: []string{"a"}},
	{name: "new", steps: []string{"n"}},
	{name: "import", steps: []string{"I"}},
	{name: "export", steps: []string{"enter", "x"}},
	{name: "theme", steps: []string{"t"}},
	{name: "log", steps: []string{"L"}},
	{name: "log over help", steps: []string{"?", "L"}},
	{name: "log on the skill screen", steps: []string{"enter", "L"}},
}

func TestEveryScreenFitsTheTerminal(t *testing.T) {
	for _, size := range layoutSizes {
		for _, sc := range allScreens {
			t.Run(fmt.Sprintf("%s at %dx%d", sc.name, size[0], size[1]), func(t *testing.T) {
				m, b := layoutModel(t, size[0], size[1])
				b.changes = []ops.FileChange{{Rel: "SKILL.md", Kind: ops.ChangeChanged, Diff: "--- a\n+++ b\n@@ -1 +1 @@\n-" + strings.Repeat("old ", 60) + "\n+" + strings.Repeat("new ", 60) + "\n"}}
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

// barRows returns the two rows of the bar at the foot of view.
func barRows(view string) (first, second string) {
	lines := strings.Split(view, "\n")
	return lines[len(lines)-2], lines[len(lines)-1]
}

// powerlineGlyphs are the private-use separators of the nerd set (spec §9.5 drops them).
const powerlineGlyphs = "\ue0b0\ue0b1\ue0b2\ue0b3"

func TestBarFitsTwoRowsAt80(t *testing.T) {
	m, _ := layoutModel(t, 80, 24)
	first, second := barRows(m.View())
	for _, row := range []string{first, second} {
		if w := lipgloss.Width(row); w != 80 {
			t.Errorf("bar row is %d columns, want 80: %q", w, row)
		}
	}
	if !strings.Contains(first, "MAIN") || !strings.Contains(first, "3 need attention") {
		t.Errorf("first bar row lacks the badge or the counts: %q", first)
	}
	if !strings.Contains(second, "↑↓/jk move") {
		t.Errorf("second bar row lacks the hints: %q", second)
	}
}

// TestBarIsTwoRowsOnEveryScreen checks the bar on every screen and overlay at each
// size: two rows, each as wide as the terminal, the badge first, and no powerline glyph.
func TestBarIsTwoRowsOnEveryScreen(t *testing.T) {
	for _, size := range layoutSizes {
		for _, sc := range allScreens {
			t.Run(fmt.Sprintf("%s at %dx%d", sc.name, size[0], size[1]), func(t *testing.T) {
				m, b := layoutModel(t, size[0], size[1])
				b.changes = []ops.FileChange{{Rel: "SKILL.md", Kind: ops.ChangeChanged, Diff: "--- a\n+++ b\n@@ -1 +1 @@\n-old\n+new\n"}}
				m = press(t, m, sc.steps...)
				view := m.View()
				first, second := barRows(view)
				for _, row := range []string{first, second} {
					if w := lipgloss.Width(row); w != size[0] {
						t.Errorf("bar row is %d columns, want %d: %q", w, size[0], row)
					}
				}
				if !strings.Contains(first, m.modeName()) {
					t.Errorf("first bar row lacks the badge %q: %q", m.modeName(), first)
				}
				if !strings.Contains(sgr.ReplaceAllString(first, ""), "skills ·") {
					t.Errorf("first bar row lacks the counts: %q", first)
				}
				if !strings.HasPrefix(sgr.ReplaceAllString(second, ""), " ") {
					t.Errorf("second bar row has no left pad: %q", second)
				}
				if strings.ContainsAny(view, powerlineGlyphs) {
					t.Errorf("screen has a powerline glyph:\n%s", view)
				}
			})
		}
	}
}

// TestGlobalKeysStayAtTheEndAt80 checks that at 80 columns the bar cuts the screen's
// own hints and keeps the global keys whole at the end of row 2.
func TestGlobalKeysStayAtTheEndAt80(t *testing.T) {
	tests := []struct {
		name  string
		steps []string
		want  []string
	}{
		{name: "main grid", want: []string{"L log", "? help", "q quit"}},
		{name: "skill screen", steps: []string{"enter"}, want: []string{"esc back", "L log", "? help", "q quit"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := layoutModel(t, 80, 24)
			m = press(t, m, tt.steps...)
			_, second := barRows(m.View())
			plain := strings.TrimRight(sgr.ReplaceAllString(second, ""), " ")
			if want := strings.Join(tt.want, " · "); !strings.HasSuffix(plain, want) {
				t.Errorf("second bar row = %q, want it to end with %q", plain, want)
			}
			if !strings.Contains(plain, "…") {
				t.Errorf("second bar row shows all the own hints at 80 columns, want them cut: %q", plain)
			}
		})
	}
}

func TestNerdBarHasNoSeparators(t *testing.T) {
	th, _ := testLook(t)
	set, ok := icons.ByName("nerd")
	if !ok {
		t.Fatal("icon set nerd is missing")
	}
	b := &fakeBackend{rows: []ops.Row{row("code-review", "claude", ops.Synced)}}
	m := testModel(b, []config.Agent{claude}, th, set, true)
	m, _ = settle(t, m, m.Init())
	m = sized(t, m, 80, 24)
	view := m.View()
	if strings.ContainsAny(view, powerlineGlyphs) {
		t.Errorf("nerd bar has a powerline glyph:\n%s", view)
	}
	if first, second := barRows(view); lipgloss.Width(first) != 80 || lipgloss.Width(second) != 80 {
		t.Errorf("nerd bar rows are %d and %d columns, want 80", lipgloss.Width(first), lipgloss.Width(second))
	}
}

// longPath is a folder path too long for an 80-column terminal.
const longPath = "/home/agent/a-long-directory-name/another-long-directory-name/more-folders/skills"

// skillScreenModel opens alpha, which has a vault copy with a long path, a long
// description and an invalid problem, and sits in claude at a long path too.
func skillScreenModel(t *testing.T, w, h int) Model {
	t.Helper()
	b := &fakeBackend{
		rows: []ops.Row{row("alpha", "claude", ops.Synced), row("alpha", "cursor", ops.NotInstalled)},
		vaultSkills: []vault.Skill{{
			Name: "alpha", Dir: longPath + "/alpha", Version: "1.2.0", Hash: "sha256:0123456789abcdef",
			Description: strings.Repeat("Reviews the pending changes on a branch and says what to fix first. ", 4),
			Problems:    []string{strings.Repeat("the problem text runs on for a long time ", 3)},
		}},
	}
	agents := []config.Agent{{ID: "claude", Path: longPath, Enabled: true}, cursorAgent}
	return press(t, sized(t, loaded(t, b, agents), w, h), "enter")
}

func TestSkillScreenKeepsTheCursorAgentInView(t *testing.T) {
	var agents []config.Agent
	for i := range 20 {
		agents = append(agents, config.Agent{ID: fmt.Sprintf("agent-%02d", i), Path: fmt.Sprintf("/agents/%02d", i), Enabled: true})
	}
	b := &fakeBackend{rows: []ops.Row{row("alpha", "agent-00", ops.Synced)}}
	m := press(t, sized(t, loaded(t, b, agents), 80, 24), "enter")
	for range 19 {
		m = press(t, m, "down")
	}
	view := m.View()
	if got := lineWith(t, view, m.icons.Marker); !strings.Contains(got, "agent-19") {
		t.Errorf("cursor row = %q, want agent-19", got)
	}
	if lines := strings.Split(view, "\n"); len(lines) != 24 {
		t.Errorf("view has %d lines, want 24", len(lines))
	}
}

// assertFits fails on each body line wider than w columns. It checks the lines
// before the frame cuts them to the terminal.
func assertFits(t *testing.T, w int, body []string) {
	t.Helper()
	for _, line := range body {
		if got := lipgloss.Width(line); got > w {
			t.Errorf("body line is %d columns, want at most %d: %q", got, w, line)
		}
	}
}

func TestBodiesFitBeforeTheFrameCutsThem(t *testing.T) {
	for _, size := range layoutSizes {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			grid, _ := layoutModel(t, size[0], size[1])
			assertFits(t, size[0], grid.mainBody())
			assertFits(t, size[0], skillScreenModel(t, size[0], size[1]).skillBody())
		})
	}
}

func TestBarCountsSkillsOnTheMainScreen(t *testing.T) {
	m, _ := layoutModel(t, 200, 60)
	if first, _ := barRows(m.View()); !strings.Contains(first, "4 skills · 0 invalid · 3 need attention") {
		t.Errorf("bar lacks the counts: %q", first)
	}
}

func TestBarNamesTheSkillScreenAndItsBackKey(t *testing.T) {
	m, _ := layoutModel(t, 200, 60)
	m = press(t, m, "enter")
	first, second := barRows(m.View())
	if !strings.Contains(first, "SKILL") || !strings.Contains(second, "esc back") {
		t.Errorf("skill bar = %q / %q", first, second)
	}
}

func TestBarHintsCutAtTheEnd(t *testing.T) {
	tests := []struct {
		name string
		s    string
		w    int
		want string
	}{
		{name: "fits", s: "a · b", w: 10, want: "a · b"},
		{name: "cut at the end with an ellipsis", s: "a · b · c", w: 5, want: "a · …"},
		{name: "nothing fits", s: "a · b", w: 0, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncate(tt.s, tt.w); got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.w, got, tt.want)
			}
		})
	}
}

// bodyMargins returns the columns left of the screen body's block and the columns
// right of it. The block is the widest non-blank line of the body.
func bodyMargins(t *testing.T, m Model) (left, right int) {
	t.Helper()
	lines := strings.Split(m.View(), "\n")
	left, widest := m.width, 0
	for _, line := range lines[m.headerHeight()+1 : len(lines)-2] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		left = min(left, leadingSpaces(line))
		widest = max(widest, lipgloss.Width(line))
	}
	return left, m.width - widest
}

func TestSkillInfoShowsTheVaultSkill(t *testing.T) {
	b := &fakeBackend{
		rows: []ops.Row{row("alpha", "claude", ops.Synced), row("alpha", "cursor", ops.NotInstalled)},
		vaultSkills: []vault.Skill{{
			Name: "alpha", Dir: "/vault/skills/alpha", Version: "1.2.0", Hash: "sha256:0123456789abcdef",
			Description: "Reviews the pending changes on a branch, then says what to fix first.",
			Valid:       false, Problems: []string{"description is longer than 1024 characters"},
		}},
	}
	m := press(t, sized(t, loaded(t, b, []config.Agent{claude, cursorAgent}), 120, 40), "enter")
	view := m.View()
	for _, want := range []string{"1.2.0", "sha256:0123456789ab", "invalid", "description is longer than 1024 characters", "Reviews the pending changes", "/vault/skills/alpha"} {
		if !strings.Contains(view, want) {
			t.Errorf("skill screen lacks %q:\n%s", want, view)
		}
	}
	if descLines := strings.Count(view, "Reviews") + strings.Count(view, "branch") + strings.Count(view, "first."); descLines > 3 {
		t.Errorf("description spans more than 3 lines:\n%s", view)
	}
	for _, want := range []string{"claude", "synced", "cursor", "not inst.", "/agents/claude/alpha"} {
		if !strings.Contains(view, want) {
			t.Errorf("sidebar and agent section lack %q:\n%s", want, view)
		}
	}
}

func TestArtIsSixLinesWithoutTrailingSpaces(t *testing.T) {
	if len(sapatoArt) != 6 {
		t.Fatalf("art has %d lines, want 6", len(sapatoArt))
	}
	for i, l := range sapatoArt {
		if strings.TrimRight(l, " ") != l {
			t.Errorf("art line %d has trailing spaces: %q", i, l)
		}
	}
}

func TestShortHash(t *testing.T) {
	tests := []struct{ in, want string }{
		{"sha256:0123456789abcdef", "sha256:0123456789ab"},
		{"sha256:abc", "sha256:abc"},
		{"", ":"},
	}
	for _, tt := range tests {
		if got := shortHash(tt.in); got != tt.want {
			t.Errorf("shortHash(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNoColorSelectsWithReverseVideo(t *testing.T) {
	th, _ := testLook(t)
	plain := newStyles(th, true)
	if !plain.row.GetReverse() {
		t.Errorf("NO_COLOR selected row lacks reverse video")
	}
	if _, ok := plain.row.GetBackground().(lipgloss.NoColor); !ok {
		t.Errorf("NO_COLOR selected row sets a background")
	}
	colored := newStyles(th, false)
	if got := colored.row.GetBackground(); got != th.Surface {
		t.Errorf("selected row background = %v, want the Surface role %v", got, th.Surface)
	}
}

// TestBarShowsNoMessage checks that an action result leaves the bar: the bar keeps
// its counts, and the message is a notification instead.
func TestBarShowsNoMessage(t *testing.T) {
	m, _ := layoutModel(t, 80, 24)
	m.notify(noteSuccess, "installed", "install alpha in claude")
	first, second := barRows(m.View())
	if strings.Contains(first+second, "install alpha") {
		t.Errorf("bar shows the message: %q / %q", first, second)
	}
	if !strings.Contains(first, m.counts()) {
		t.Errorf("bar lacks the counts %q: %q", m.counts(), first)
	}
}
