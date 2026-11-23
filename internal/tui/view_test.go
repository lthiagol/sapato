package tui

import (
	"fmt"
	"math"
	"path/filepath"
	"slices"
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

// wordSpan is a space-separated word of a rendered line and its first and last
// display columns.
type wordSpan struct {
	word       string
	start, end int
}

// wordSpans returns the words of line with their columns.
func wordSpans(line string) []wordSpan {
	var spans []wordSpan
	col := 0
	for _, f := range strings.Split(line, " ") {
		if f != "" {
			spans = append(spans, wordSpan{f, col, col + lipgloss.Width(f) - 1})
		}
		col += lipgloss.Width(f) + 1
	}
	return spans
}

func TestGridTextIsCentered(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.toml"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var rows []ops.Row
	for _, skill := range []string{"code-review", "obsidian-notes", "a-long-skill-name-for-the-grid"} {
		for _, a := range cfg.Agents {
			rows = append(rows, row(skill, a.ID, ops.Synced))
		}
	}
	tests := []struct {
		name string
		w    int
	}{
		{name: "141 columns", w: 141},
		{name: "100 columns", w: 100},
		{name: "80 columns", w: 80},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := sized(t, loaded(t, &fakeBackend{rows: rows}, cfg.Agents), tt.w, 40)
			view := m.View()
			header := wordSpans(lineWith(t, view, "SKILL"))
			if len(header) != 1+len(cfg.Agents) {
				t.Fatalf("header words %v, want SKILL and %d labels", header, len(cfg.Agents))
			}
			labels := header[1:]
			body := wordSpans(lineWith(t, view, "code-review"))
			at := slices.IndexFunc(body, func(s wordSpan) bool { return s.word == "code-review" })
			icons := body[at+1:]
			if len(icons) != len(cfg.Agents) {
				t.Fatalf("row icons %v, want %d", icons, len(cfg.Agents))
			}

			rowMid := float64(body[at].start+icons[len(icons)-1].end) / 2
			if center := float64(tt.w-1) / 2; math.Abs(rowMid-center) > 0.5 {
				t.Errorf("row text midpoint %.1f, screen center %.1f", rowMid, center)
			}
			headerMid := float64(header[0].start+labels[len(labels)-1].end) / 2
			if math.Abs(headerMid-rowMid) > 1.5 {
				t.Errorf("header text midpoint %.1f, row midpoint %.1f", headerMid, rowMid)
			}
			for i, a := range cfg.Agents {
				if labels[i].word != strings.ToUpper(a.ID) {
					t.Errorf("label %d is %q, want %q", i, labels[i].word, strings.ToUpper(a.ID))
				}
				labelMid := float64(labels[i].start+labels[i].end) / 2
				if math.Abs(float64(icons[i].start)-labelMid) > 0.5 {
					t.Errorf("%s icon at column %d, label centered at %.1f", a.ID, icons[i].start, labelMid)
				}
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > tt.w {
					t.Errorf("line is %d columns, terminal is %d: %q", lipgloss.Width(line), tt.w, line)
				}
			}
		})
	}
}

func TestGridFitsNarrowTerminal(t *testing.T) {
	for _, w := range []int{60, 40} {
		t.Run(fmt.Sprintf("%d columns", w), func(t *testing.T) {
			m, _ := layoutModel(t, w, 24)
			for _, line := range strings.Split(m.View(), "\n") {
				if lipgloss.Width(line) > w {
					t.Errorf("line is %d columns, terminal is %d: %q", lipgloss.Width(line), w, line)
				}
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
	{name: "add", steps: []string{"n"}},
	{name: "new", steps: []string{"n", "c"}},
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
		centered("enter open · / filter · r refresh · n add", 80),
		centered("↑↓/jk move · t theme · L log · ? help · q quit", 80),
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

// TestHintRowIsCenteredWithDimGlobals checks the hint row at 200 columns: centered, the
// action hints, the │ divider, then the globals in the dimmer style.
func TestHintRowIsCenteredWithDimGlobals(t *testing.T) {
	m, _ := layoutModel(t, 200, 60)
	m = colored(t, m)
	view := m.View()
	bar := barLines(m, view)
	if len(bar) != 2 {
		t.Fatalf("bar is %d lines, want 2 at 200 columns", len(bar))
	}
	row := bar[1]
	plain := sgr.ReplaceAllString(row, "")
	want := centered("enter open · / filter · r refresh · n add │ ↑↓/jk move · t theme · L log · ? help · q quit", 200)
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
// and where t theme, then L log, drop out of it. Each row is centered.
func TestHintRowsWrapThenDrop(t *testing.T) {
	tests := []struct {
		name  string
		steps []string
		w     int
		want  []string // the hint rows, without their pad and color codes
	}{
		{name: "grid at 120 keeps one row", w: 120, want: []string{"enter open · / filter · r refresh · n add │ ↑↓/jk move · t theme · L log · ? help · q quit"}},
		{name: "grid at 80 wraps the globals", w: 80, want: []string{"enter open · / filter · r refresh · n add", "↑↓/jk move · t theme · L log · ? help · q quit"}},
		{name: "grid at 40 drops t theme", w: 40, want: []string{"enter open · / filter · r refresh · n a…", "↑↓/jk move · L log · ? help · q quit"}},
		{name: "grid at 30 drops t theme and L log", w: 30, want: []string{"enter open · / filter · r ref…", "↑↓/jk move · ? help · q quit"}},
		{name: "skill screen at 120 wraps the globals", steps: []string{"enter"}, w: 120, want: []string{"space mark · i install · u uninstall · U update · d diff · a adopt · A install all · c compat · x export · D remove", "↑↓/jk move · pgup/pgdn page · esc back · t theme · L log · ? help · q quit"}},
		{name: "skill screen at 80 keeps every global", steps: []string{"enter"}, w: 80, want: []string{"space mark · i install · u uninstall · U update · d diff · a adopt · A install …", "↑↓/jk move · pgup/pgdn page · esc back · t theme · L log · ? help · q quit"}},
		{name: "skill screen at 60 drops t theme and L log", steps: []string{"enter"}, w: 60, want: []string{"space mark · i install · u uninstall · U update · d diff · …", "↑↓/jk move · pgup/pgdn page · esc back · ? help · q quit"}},
		{name: "action alone too wide at 30", steps: []string{"enter"}, w: 30, want: []string{"space mark · i install · u un…", "↑↓/jk move · pgup/pgdn page ·…"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := layoutModel(t, tt.w, 24)
			m = press(t, m, tt.steps...)
			rows := plainBar(m, m.View())[1:]
			if len(rows) != len(tt.want) {
				t.Fatalf("hint rows = %q, want %q", rows, tt.want)
			}
			for i, row := range rows {
				if got := assertCentered(t, row, tt.w); got != tt.want[i] {
					t.Errorf("hint row %d = %q, want %q", i, got, tt.want[i])
				}
			}
		})
	}
}

// TestFilterAndConfirmReplaceOnlyTheActionPart checks that FILTER and a confirm show
// their own action hints in place of the screen's, with no global hints.
func TestFilterAndConfirmReplaceOnlyTheActionPart(t *testing.T) {
	m, _ := layoutModel(t, 80, 24)
	m = press(t, m, "/", "o")
	if bar := plainBar(m, m.View()); len(bar) != 2 || bar[1] != centered("/ o · enter apply · esc clear", 80) {
		t.Errorf("filter hint row = %q, want the input then its hints, no globals", bar)
	}
	m, _ = layoutModel(t, 80, 24)
	m = press(t, m, "enter", "u")
	if bar := plainBar(m, m.View()); len(bar) != 2 || bar[1] != centered("y yes · n no", 80) {
		t.Errorf("confirm hint row = %q, want y yes · n no", bar)
	}
}

// TestHintRowsAreCentered checks that the action hints of the grid and the skill screen
// start where a centered row of their width starts.
func TestHintRowsAreCentered(t *testing.T) {
	for _, w := range []int{80, 120, 200} {
		for _, sc := range []struct {
			name  string
			steps []string
			first string
		}{{name: "main grid", first: "enter open"}, {name: "skill screen", steps: []string{"enter"}, first: "space mark"}} {
			t.Run(fmt.Sprintf("%s at %d", sc.name, w), func(t *testing.T) {
				m, _ := layoutModel(t, w, 24)
				m = press(t, m, sc.steps...)
				row := plainBar(m, m.View())[1]
				if got := assertCentered(t, row, w); !strings.HasPrefix(got, sc.first) {
					t.Errorf("hint row = %q, want it to start with %q", got, sc.first)
				}
			})
		}
	}
}

// centered is s with the pad a centered row of s gets in w columns.
func centered(s string, w int) string {
	return strings.Repeat(" ", max(0, (w-lipgloss.Width(s))/2)) + s
}

// assertCentered fails unless the plain row is centered in w columns. It returns the row
// without its pad.
func assertCentered(t *testing.T, row string, w int) string {
	t.Helper()
	got := strings.TrimLeft(row, " ")
	if want := centered(got, w); row != want {
		t.Errorf("hint row %q is not centered in %d columns, want %q", row, w, want)
	}
	return got
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

// bandScreens are the non-main screens and overlays that draw the title band (spec
// §9.4), each with the title it draws. setup reaches the screen from the grid of
// layoutModel.
var bandScreens = []struct {
	name, title string
	setup       func(t *testing.T, m Model, b *fakeBackend) Model
}{
	{name: "help", title: "Help", setup: func(t *testing.T, m Model, b *fakeBackend) Model {
		return press(t, m, "?")
	}},
	{name: "add", title: "Add a skill to the vault", setup: func(t *testing.T, m Model, b *fakeBackend) Model {
		return press(t, m, "n")
	}},
	{name: "adopt", title: "Adopt unmanaged skills into the vault", setup: func(t *testing.T, m Model, b *fakeBackend) Model {
		return press(t, m, "a")
	}},
	{name: "new wizard", title: "Create a new skill", setup: func(t *testing.T, m Model, b *fakeBackend) Model {
		return press(t, m, "n", "c")
	}},
	{name: "new paste", title: "Create a new skill", setup: func(t *testing.T, m Model, b *fakeBackend) Model {
		return press(t, m, "n", "c", "tab")
	}},
	{name: "import source", title: "Import skills", setup: func(t *testing.T, m Model, b *fakeBackend) Model {
		return press(t, m, "I")
	}},
	{name: "import picker", title: "Import skills · choose a file", setup: func(t *testing.T, m Model, b *fakeBackend) Model {
		m.home = t.TempDir()
		for i := range 4 {
			writeSkillFolder(t, m.home, fmt.Sprintf("entry-%d", i))
		}
		return press(t, typeText(t, press(t, m, "I"), m.home), "ctrl+o")
	}},
	{name: "import list", title: "Import skills · pick skills", setup: func(t *testing.T, m Model, b *fakeBackend) Model {
		b.discovered = map[string][]ops.Candidate{"/src/pack": {
			{Rel: "a", Name: "alpha", Hint: "alpha", Valid: true},
			{Rel: "b", Name: "", Problems: []string{"description is missing"}},
		}}
		return press(t, typeText(t, press(t, m, "I"), "/src/pack"), "enter")
	}},
	{name: "import fix-up", title: "Import skills · fix draft", setup: func(t *testing.T, m Model, b *fakeBackend) Model {
		b.discovered = map[string][]ops.Candidate{"/src/bad": {
			{Rel: ".", Name: "draft", Hint: "draft", Problems: []string{"name is invalid"}, Fixable: true},
		}}
		return press(t, typeText(t, press(t, m, "I"), "/src/bad"), "enter")
	}},
	{name: "export", title: "Export a-rather-long-skill-name-that-keeps-going-on", setup: func(t *testing.T, m Model, b *fakeBackend) Model {
		const name = "a-rather-long-skill-name-that-keeps-going-on"
		m.vaultSkills[name] = vault.Skill{Name: name}
		return press(t, onRow(t, m, name), "enter", "x")
	}},
	{name: "theme", title: "Theme", setup: func(t *testing.T, m Model, b *fakeBackend) Model {
		return press(t, m, "t")
	}},
	{name: "log", title: "Log", setup: func(t *testing.T, m Model, b *fakeBackend) Model {
		return press(t, m, "L")
	}},
}

// TestTitleBandOnEveryNonMainScreen checks each non-main screen at 60x20, 100x30, 120x40
// and 141x40: the rule under the header, then the title row centered with the right
// title, then a full-width rule. The content is one block centered in the terminal, and
// nothing is wider than the terminal.
func TestTitleBandOnEveryNonMainScreen(t *testing.T) {
	for _, size := range [][2]int{{60, 20}, {100, 30}, {120, 40}, {141, 40}} {
		for _, sc := range bandScreens {
			t.Run(fmt.Sprintf("%s at %dx%d", sc.name, size[0], size[1]), func(t *testing.T) {
				m, b := layoutModel(t, size[0], size[1])
				m = sc.setup(t, m, b)
				if m.mode == modeNormal {
					t.Fatalf("the screen did not open: %s", m.lastMsg())
				}
				w, h := size[0], size[1]
				view := m.View()
				lines := strings.Split(view, "\n")
				if len(lines) != h {
					t.Fatalf("view has %d lines, want %d", len(lines), h)
				}
				head := m.headerHeight()
				plain := func(i int) string { return sgr.ReplaceAllString(lines[i], "") }
				if got := plain(head); got != strings.Repeat(m.icons.Rule, w) {
					t.Errorf("row under the header is %q, want the full-width rule", got)
				}
				title := plain(head + 1)
				if got := strings.TrimSpace(title); got != sc.title {
					t.Errorf("title row = %q, want %q", got, sc.title)
				}
				if got, want := leadingSpaces(title), (w-lipgloss.Width(sc.title))/2; got != want {
					t.Errorf("title starts at column %d, want %d (centered)", got, want)
				}
				if got := plain(head + 2); got != strings.Repeat(m.icons.Rule, w) {
					t.Errorf("row under the title is %q, want the full-width rule", got)
				}

				// Every titled screen and step is one block, centered, with its content
				// left-aligned.
				checkBlock(t, m, view)
				for i, l := range lines {
					if got := lipgloss.Width(l); got > w {
						t.Errorf("line %d is %d columns, want at most %d: %q", i, got, w, l)
					}
				}
			})
		}
	}
}

// TestShortScreensAreCenteredAtWideTerminals checks that ADD, the NEW wizard and ADOPT,
// whose content is narrower than their block, sit centered at 141 columns: the content
// starts past column 30.
func TestShortScreensAreCenteredAtWideTerminals(t *testing.T) {
	short := []string{"add", "new wizard", "adopt"}
	for _, sc := range bandScreens {
		if !slices.Contains(short, sc.name) {
			continue
		}
		t.Run(sc.name, func(t *testing.T) {
			m, b := layoutModel(t, 141, 40)
			m = sc.setup(t, m, b)
			if got := contentEdge(m, m.View()); got <= 30 {
				t.Errorf("content starts at column %d, want past 30 at 141 columns:\n%s", got, m.View())
			}
		})
	}
}

// TestStepsOfOneScreenShareTheirLeftEdge checks that the steps of IMPORT, and the wizard
// and the paste mode of NEW, draw at one left edge at each size, so the content does not
// move between them.
func TestStepsOfOneScreenShareTheirLeftEdge(t *testing.T) {
	groups := [][]string{
		{"import source", "import picker", "import list", "import fix-up"},
		{"new wizard", "new paste"},
	}
	for _, size := range [][2]int{{141, 40}, {100, 30}, {60, 20}} {
		for _, group := range groups {
			t.Run(fmt.Sprintf("%s at %dx%d", group[0], size[0], size[1]), func(t *testing.T) {
				first := -1
				for _, sc := range bandScreens {
					if !slices.Contains(group, sc.name) {
						continue
					}
					m, b := layoutModel(t, size[0], size[1])
					m = sc.setup(t, m, b)
					edge := contentEdge(m, m.View())
					if first < 0 {
						first = edge
					}
					if edge != first {
						t.Errorf("%s starts at column %d, want %d like its siblings:\n%s", sc.name, edge, first, m.View())
					}
				}
			})
		}
	}
}

// TestAddRowsShareOneLeftEdge checks that the rows of ADD name in one column whichever row
// holds the cursor.
func TestAddRowsShareOneLeftEdge(t *testing.T) {
	for _, size := range [][2]int{{141, 40}, {60, 20}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m, _ := layoutModel(t, size[0], size[1])
			m = press(t, m, "n")
			for cur := range 3 {
				m.addCur = cur
				view := m.View()
				col := -1
				for _, name := range []string{"Create", "Import", "Adopt"} {
					line := sgr.ReplaceAllString(lineWith(t, view, name), "")
					at := lipgloss.Width(line[:strings.Index(line, name)])
					if col < 0 {
						col = at
					}
					if at != col {
						t.Errorf("cursor on row %d: %s is at column %d, want %d:\n%s", cur, name, at, col, view)
					}
				}
			}
		})
	}
}

// TestPasteEditorSitsAboveItsBar checks that the check line of NEW's paste mode is the
// row just above the bar at every size: the editor is sized for the paste mode's own bar,
// not for the bar of the wizard or of the screen NEW opened over.
func TestPasteEditorSitsAboveItsBar(t *testing.T) {
	for _, size := range layoutSizes {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m, _ := layoutModel(t, size[0], size[1])
			m = press(t, m, "n", "c", "tab")
			if !m.newPaste {
				t.Fatalf("NEW did not switch to the paste mode: %q", m.lastMsg())
			}
			lines := strings.Split(m.View(), "\n")
			above := sgr.ReplaceAllString(lines[len(lines)-len(m.bar())-1], "")
			if !strings.Contains(above, "paste a complete SKILL.md") {
				t.Errorf("row above the bar is %q, want the editor's check line:\n%s", above, m.View())
			}
		})
	}
}

// helpLast is the last line of HELP. It shows only once HELP is scrolled to its end.
const helpLast = "esc or ? closes this view"

// TestHelpScrollsToItsLastLine checks that HELP, taller than a short terminal, scrolls
// to its last line without drawing past the terminal, and that a reopened HELP starts
// at its top.
func TestHelpScrollsToItsLastLine(t *testing.T) {
	m, _ := layoutModel(t, 60, 20)
	m = press(t, m, "?")
	if strings.Contains(m.View(), helpLast) {
		t.Fatalf("HELP shows its last line at 60x20 before any scroll:\n%s", m.View())
	}
	for range 10 {
		m = press(t, m, "pgdown")
	}
	view := m.View()
	if !strings.Contains(view, helpLast) {
		t.Fatalf("HELP does not reach its last line by scrolling:\n%s", view)
	}
	lines := strings.Split(view, "\n")
	if len(lines) != 20 {
		t.Errorf("view has %d rows, want 20", len(lines))
	}
	assertFits(t, 60, lines)

	m = press(t, m, "esc", "?")
	if !strings.Contains(m.View(), "Cells") {
		t.Errorf("reopened HELP does not start at its top:\n%s", m.View())
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

// contentEdge is the column where the content block of a titled screen starts: the
// least indent of its non-blank lines, between the title band and the bar.
func contentEdge(m Model, view string) int {
	lines := strings.Split(view, "\n")
	edge := m.width
	for _, line := range lines[m.headerHeight()+3 : len(lines)-len(m.bar())] {
		plain := sgr.ReplaceAllString(line, "")
		if strings.TrimSpace(plain) == "" {
			continue
		}
		edge = min(edge, leadingSpaces(plain))
	}
	return edge
}

// contentExtent is the widest content line of view, counted from the block's left edge
// to its last visible column. Trailing spaces do not count.
func contentExtent(m Model, view string) int {
	lines := strings.Split(view, "\n")
	edge := contentEdge(m, view)
	ext := 0
	for _, line := range lines[m.headerHeight()+3 : len(lines)-len(m.bar())] {
		plain := strings.TrimRight(sgr.ReplaceAllString(line, ""), " ")
		ext = max(ext, lipgloss.Width(plain)-edge)
	}
	return ext
}

// checkBlock checks the block of the titled screen m is on, drawn as view. The block is
// between its least and the most (two thirds of the terminal, at least dialogMinW). An
// input screen (NEW, IMPORT, EXPORT) has the least of dialogMinW, or the terminal when
// narrower; other screens have none. The block is centered and its content starts at
// its left edge. A screen without an input is exactly as wide as its content.
func checkBlock(t *testing.T, m Model, view string) {
	t.Helper()
	w := m.width
	least, most := 1, min(w, max(dialogMinW, w*2/3))
	input := false
	switch m.mode {
	case modeNew, modeImport, modeImportBrowse, modeImportList, modeImportFix, modeExport:
		input = true
		least = min(dialogMinW, w)
	}
	if m.block < least || m.block > most {
		t.Errorf("block is %d wide, want %d to %d", m.block, least, most)
	}
	if got, want := contentEdge(m, view), (w-m.block)/2; got != want {
		t.Errorf("content starts at column %d, want %d (a centered block %d wide):\n%s", got, want, m.block, view)
	}
	ext := contentExtent(m, view)
	if input && ext > m.block || !input && ext != m.block {
		t.Errorf("block is %d wide for content %d wide:\n%s", m.block, ext, view)
	}
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
