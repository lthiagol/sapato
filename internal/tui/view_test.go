package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

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

// barLines returns the bar at the foot of view: its rule, then its hint rows.
func barLines(m Model, view string) []string {
	lines := strings.Split(view, "\n")
	return lines[len(lines)-len(m.bar()):]
}

// plainBar is barLines with the color codes removed.
func plainBar(m Model, view string) []string {
	bar := barLines(m, view)
	for i, l := range bar {
		bar[i] = sgr.ReplaceAllString(l, "")
	}
	return bar
}

// powerlineGlyphs are the private-use separators of the nerd set (spec §9.5 drops them).
const powerlineGlyphs = ""

// barNames are the mode names the bar used to show as badges. The bar shows none.
var barNames = []string{"MAIN", "SKILL", "FILTER", "CONFIRM", "DIFF", "HELP", "ADOPT", "NEW", "IMPORT", "EXPORT", "THEME", "LOG"}

// TestBarAt80 checks the bar of the grid at 80 columns: the counts ride the rule, the
// action hints fit one row, and the globals wrap to a second.
func TestBarAt80(t *testing.T) {
	m, _ := layoutModel(t, 80, 24)
	bar := plainBar(m, m.View())
	want := []string{
		"───────────────────────────────────── 4 skills · 0 invalid · 3 need attention ──",
		" enter open · / filter · r refresh · a adopt · n new · I import",
		" ↑↓/jk move · t theme · L log · ? help · q quit",
	}
	if strings.Join(bar, "\n") != strings.Join(want, "\n") {
		t.Errorf("bar at 80 columns:\n%s\nwant:\n%s", strings.Join(bar, "\n"), strings.Join(want, "\n"))
	}
}

// TestBarIsSummaryRuleAndHintRowsOnEveryScreen checks the bar on every screen and
// overlay at each size: the rule with the counts, then one or two hint rows, each no
// wider than the terminal, with no mode badge and no powerline glyph.
func TestBarIsSummaryRuleAndHintRowsOnEveryScreen(t *testing.T) {
	for _, size := range layoutSizes {
		for _, sc := range allScreens {
			t.Run(fmt.Sprintf("%s at %dx%d", sc.name, size[0], size[1]), func(t *testing.T) {
				m, b := layoutModel(t, size[0], size[1])
				b.changes = []ops.FileChange{{Rel: "SKILL.md", Kind: ops.ChangeChanged, Diff: "--- a\n+++ b\n@@ -1 +1 @@\n-old\n+new\n"}}
				m = press(t, m, sc.steps...)
				view := m.View()
				bar := plainBar(m, view)
				if n := len(bar); n != 2 && n != 3 {
					t.Fatalf("bar is %d lines, want 2 or 3:\n%s", n, view)
				}
				if want := sgr.ReplaceAllString(m.summaryRule(), ""); bar[0] != want || !strings.Contains(bar[0], "skills ·") {
					t.Errorf("bar rule = %q, want the counts in it: %q", bar[0], want)
				}
				if w := lipgloss.Width(bar[0]); w != size[0] {
					t.Errorf("bar rule is %d columns, want %d", w, size[0])
				}
				for _, row := range bar[1:] {
					if w := lipgloss.Width(row); w > size[0] {
						t.Errorf("hint row is %d columns, want at most %d: %q", w, size[0], row)
					}
				}
				for _, name := range barNames {
					if strings.Contains(strings.Join(bar, "\n"), name) {
						t.Errorf("bar shows the mode name %q: %q", name, bar)
					}
				}
				if strings.ContainsAny(view, powerlineGlyphs) {
					t.Errorf("screen has a powerline glyph:\n%s", view)
				}
			})
		}
	}
}

// colored makes lipgloss write color codes for the rest of t, as a terminal does, and
// gives m its styles in color. The test models are built without color, which hides
// the styles.
func colored(t *testing.T, m Model) Model {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	m.styles = newStyles(m.styles.th, false)
	return m
}

// TestSummaryRidesTheBarRule checks the counts at 120 columns: right-aligned in the
// bar's rule, with two rule glyphs at the edge, and each part in its own style.
func TestSummaryRidesTheBarRule(t *testing.T) {
	m, _ := layoutModel(t, 120, 40)
	m = colored(t, m)
	m.vaultSkills = map[string]vault.Skill{"hand-made": {Name: "hand-made"}}
	rule := barLines(m, m.View())[0]
	plain := sgr.ReplaceAllString(rule, "")
	if want := " 4 skills · 1 invalid · 3 need attention ──"; !strings.HasSuffix(plain, want) {
		t.Errorf("rule = %q, want it to end with %q", plain, want)
	}
	if !strings.HasPrefix(plain, m.icons.Rule) || lipgloss.Width(plain) != 120 {
		t.Errorf("rule = %q, want rule glyphs from the left edge, 120 columns wide", plain)
	}
	for _, part := range []string{
		m.styles.text.Render("4 skills"),
		m.styles.fail.Render("1 invalid"),
		m.styles.warn.Render("3 need attention"),
	} {
		if !strings.Contains(rule, part) {
			t.Errorf("rule lacks the part %q in its style", part)
		}
	}
	m, _ = layoutModel(t, 120, 40)
	m = colored(t, m)
	if rule := barLines(m, m.View())[0]; !strings.Contains(rule, m.styles.dim.Render("0 invalid")) {
		t.Errorf("zero invalid is not dimmed in the rule: %q", rule)
	}
}

// TestSummaryDropsFromTheRight checks the counts at narrow widths: attention drops
// first, then invalid, then the skill count, and the rule is plain when even that
// does not fit with four rule glyphs.
func TestSummaryDropsFromTheRight(t *testing.T) {
	tests := []struct {
		w    int
		want string // the end of the rule, or "" for a plain rule
	}{
		{w: 120, want: " 4 skills · 0 invalid · 3 need attention ──"},
		{w: 45, want: " 4 skills · 0 invalid · 3 need attention ──"},
		{w: 44, want: " 4 skills · 0 invalid ──"},
		{w: 26, want: " 4 skills · 0 invalid ──"},
		{w: 25, want: " 4 skills ──"},
		{w: 14, want: " 4 skills ──"},
		{w: 13, want: ""},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d columns", tt.w), func(t *testing.T) {
			m, _ := layoutModel(t, tt.w, 24)
			rule := sgr.ReplaceAllString(barLines(m, m.View())[0], "")
			if lipgloss.Width(rule) != tt.w {
				t.Errorf("rule is %d columns, want %d: %q", lipgloss.Width(rule), tt.w, rule)
			}
			if tt.want == "" {
				if plain := strings.Repeat(m.icons.Rule, tt.w); rule != plain {
					t.Errorf("rule = %q, want a plain rule", rule)
				}
				return
			}
			if !strings.HasSuffix(rule, tt.want) {
				t.Errorf("rule = %q, want it to end with %q", rule, tt.want)
			}
		})
	}
}

// TestHintRowIsLeftAlignedWithDimGlobals checks the hint row at 200 columns: one column
// of pad, the action hints, the │ divider, then the globals in the dimmer style.
func TestHintRowIsLeftAlignedWithDimGlobals(t *testing.T) {
	m, _ := layoutModel(t, 200, 60)
	m = colored(t, m)
	view := m.View()
	bar := barLines(m, view)
	if len(bar) != 2 {
		t.Fatalf("bar is %d lines, want 2 at 200 columns", len(bar))
	}
	row := bar[1]
	plain := sgr.ReplaceAllString(row, "")
	want := " enter open · / filter · r refresh · a adopt · n new · I import │ ↑↓/jk move · t theme · L log · ? help · q quit"
	if plain != want {
		t.Errorf("hint row = %q, want %q", plain, want)
	}
	for _, part := range []string{
		m.styles.hintKey.Render("enter") + " " + m.styles.subtext.Render("open"),
		m.styles.dim.Render(" │ "),
		m.styles.subtext.Render("↑↓/jk") + " " + m.styles.dim.Render("move"),
	} {
		if !strings.Contains(row, part) {
			t.Errorf("hint row lacks the styled part %q", part)
		}
	}
}

// TestHintRowsWrapThenDrop checks the hint rows where the globals wrap to a second row,
// and where t theme, then L log, drop out of it.
func TestHintRowsWrapThenDrop(t *testing.T) {
	tests := []struct {
		name  string
		steps []string
		w     int
		want  []string // the hint rows, without their color codes
	}{
		{name: "grid at 120 keeps one row", w: 120, want: []string{" enter open · / filter · r refresh · a adopt · n new · I import │ ↑↓/jk move · t theme · L log · ? help · q quit"}},
		{name: "grid at 80 wraps the globals", w: 80, want: []string{" enter open · / filter · r refresh · a adopt · n new · I import", " ↑↓/jk move · t theme · L log · ? help · q quit"}},
		{name: "grid at 40 drops t theme", w: 40, want: []string{" enter open · / filter · r refresh · a …", " ↑↓/jk move · L log · ? help · q quit"}},
		{name: "grid at 30 drops t theme and L log", w: 30, want: []string{" enter open · / filter · r re…", " ↑↓/jk move · ? help · q quit"}},
		{name: "skill screen at 120 wraps the globals", steps: []string{"enter"}, w: 120, want: []string{" space mark · i install · u uninstall · U update · d diff · a adopt · A install all · c compat · x export · D remove", " ↑↓/jk move · pgup/pgdn page · esc back · t theme · L log · ? help · q quit"}},
		{name: "skill screen at 80 keeps every global", steps: []string{"enter"}, w: 80, want: []string{" space mark · i install · u uninstall · U update · d diff · a adopt · A install…", " ↑↓/jk move · pgup/pgdn page · esc back · t theme · L log · ? help · q quit"}},
		{name: "skill screen at 60 drops t theme and L log", steps: []string{"enter"}, w: 60, want: []string{" space mark · i install · u uninstall · U update · d diff ·…", " ↑↓/jk move · pgup/pgdn page · esc back · ? help · q quit"}},
		{name: "action alone too wide at 30", steps: []string{"enter"}, w: 30, want: []string{" space mark · i install · u u…", " ↑↓/jk move · pgup/pgdn page …"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := layoutModel(t, tt.w, 24)
			m = press(t, m, tt.steps...)
			rows := plainBar(m, m.View())[1:]
			if strings.Join(rows, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("hint rows:\n%s\nwant:\n%s", strings.Join(rows, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

// TestFilterAndConfirmReplaceOnlyTheActionPart checks that FILTER and a confirm show
// their own action hints in place of the screen's, with no global hints.
func TestFilterAndConfirmReplaceOnlyTheActionPart(t *testing.T) {
	m, _ := layoutModel(t, 80, 24)
	m = press(t, m, "/", "o")
	if bar := plainBar(m, m.View()); len(bar) != 2 || bar[1] != " / o · enter apply · esc clear" {
		t.Errorf("filter hint row = %q, want the input then its hints, no globals", bar)
	}
	m, _ = layoutModel(t, 80, 24)
	m = press(t, m, "enter", "u")
	if bar := plainBar(m, m.View()); len(bar) != 2 || bar[1] != " y yes · n no" {
		t.Errorf("confirm hint row = %q, want y yes · n no", bar)
	}
}

// TestActionHintsAreLeftAligned checks that the action hints start after one column of pad, at
// each width, on the grid and the skill screen.
func TestActionHintsAreLeftAligned(t *testing.T) {
	for _, w := range []int{80, 120, 200} {
		for _, sc := range []struct {
			name  string
			steps []string
			first string
		}{{name: "main grid", first: " enter open"}, {name: "skill screen", steps: []string{"enter"}, first: " space mark"}} {
			t.Run(fmt.Sprintf("%s at %d", sc.name, w), func(t *testing.T) {
				m, _ := layoutModel(t, w, 24)
				m = press(t, m, sc.steps...)
				row := plainBar(m, m.View())[1]
				if !strings.HasPrefix(row, sc.first) || strings.HasPrefix(row, "  ") {
					t.Errorf("hint row = %q, want it to start with one pad column and %q", row, sc.first)
				}
			})
		}
	}
}

// TestBodyHeightFollowsTheBarLines checks that the body takes the rows the bar leaves:
// the bar is two lines on a wide grid and three when its globals wrap.
func TestBodyHeightFollowsTheBarLines(t *testing.T) {
	tests := []struct {
		w, h, bar int
	}{
		{w: 200, h: 60, bar: 2},
		{w: 60, h: 20, bar: 3},
		{w: 120, h: 40, bar: 2},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%dx%d", tt.w, tt.h), func(t *testing.T) {
			m, _ := layoutModel(t, tt.w, tt.h)
			if n := len(m.bar()); n != tt.bar {
				t.Fatalf("bar is %d lines, want %d", n, tt.bar)
			}
			if want := tt.h - m.headerHeight() - 1 - tt.bar; m.bodyHeight() != want {
				t.Errorf("body height = %d, want %d", m.bodyHeight(), want)
			}
			if lines := strings.Split(m.View(), "\n"); len(lines) != tt.h {
				t.Errorf("view has %d lines, want %d", len(lines), tt.h)
			}
		})
	}
}

func TestZeroCountsAreDimmed(t *testing.T) {
	th, _ := testLook(t)
	m := testModel(&fakeBackend{}, nil, th, icons.Set{}, false)
	if got := m.countStyle(0, m.styles.fail).GetForeground(); got != th.Overlay {
		t.Errorf("zero invalid is %v, want the Overlay role %v", got, th.Overlay)
	}
	if got := m.countStyle(2, m.styles.fail).GetForeground(); got != th.Red {
		t.Errorf("two invalid is %v, want the Red role %v", got, th.Red)
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
	if bar := barLines(m, view); lipgloss.Width(bar[0]) != 80 {
		t.Errorf("nerd bar rule is %d columns, want 80", lipgloss.Width(bar[0]))
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
	agents := []config.Agent{{ID: "claude", Path: longPath}, cursorAgent}
	return press(t, sized(t, loaded(t, b, agents), w, h), "enter")
}

func TestSkillScreenKeepsTheCursorAgentInView(t *testing.T) {
	var agents []config.Agent
	for i := range 20 {
		agents = append(agents, config.Agent{ID: fmt.Sprintf("agent-%02d", i), Path: fmt.Sprintf("/agents/%02d", i)})
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
	if rule := plainBar(m, m.View())[0]; !strings.Contains(rule, "4 skills · 0 invalid · 3 need attention") {
		t.Errorf("bar rule lacks the counts: %q", rule)
	}
}

func TestBarShowsTheSkillScreensActions(t *testing.T) {
	m, _ := layoutModel(t, 200, 60)
	m = press(t, m, "enter")
	bar := plainBar(m, m.View())
	if !strings.Contains(bar[1], "i install · u uninstall") {
		t.Errorf("skill hint row = %q, want the action hints", bar[1])
	}
	if !strings.Contains(bar[1], "esc back") {
		t.Errorf("skill hint row = %q, want esc back", bar[1])
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
	for _, line := range lines[m.headerHeight()+1 : len(lines)-3] {
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
	bar := plainBar(m, m.View())
	if strings.Contains(strings.Join(bar, "\n"), "install alpha") {
		t.Errorf("bar shows the message: %q", bar)
	}
	if !strings.Contains(bar[0], "4 skills · 0 invalid · 3 need attention") {
		t.Errorf("bar lacks the counts: %q", bar[0])
	}
}
