package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"sapato/internal/config"
	"sapato/internal/ops"
	"sapato/internal/target"
	"sapato/internal/tui/icons"
)

// treeChars are the glyphs a tree line starts with: the connectors of both icon sets
// and the spaces that indent under a branch.
const treeChars = "│├└─|`- "

// filesOf lists the FILES block of the details pane of m, one string per line. The
// tree prefix is kept as drawn; the text after it has its inner spaces collapsed to
// one, so the right-aligned sizes compare as plain text.
func filesOf(m Model) []string {
	var out []string
	in := false
	for _, line := range m.detailLines(m.skillLayout().detail.w - 4) {
		text := strings.TrimRight(ansi.Strip(line), " ")
		if strings.HasPrefix(text, "FILES") {
			in = true
		}
		if !in {
			continue
		}
		if text == "" {
			break
		}
		prefix := text[:len(text)-len(strings.TrimLeft(text, treeChars))]
		out = append(out, prefix+strings.Join(strings.Fields(text[len(prefix):]), " "))
	}
	return out
}

func TestSkillFilesTreeOfFixtureSkills(t *testing.T) {
	tests := []struct {
		skill string
		want  []string
	}{
		{"mocked-skill-00001", []string{"FILES (1)", "└── SKILL.md 405 B"}},
		{"mocked-skill-00003", []string{
			"FILES (4)", "├── SKILL.md 644 B", "└── references/",
			"    ├── guides/", "    │   └── advanced.md 248 B", "    ├── api.md 222 B", "    └── overview.md 224 B",
		}},
		{"mocked-skill-00004", []string{
			"FILES (4)", "├── SKILL.md 630 B", "└── scripts/",
			"    ├── README.md 114 B", "    ├── check.py ⚙ 154 B", "    └── run.sh ⚙ 155 B",
		}},
		{"mocked-skill-00005", []string{
			"FILES (3)", "├── SKILL.md 590 B", "└── assets/", "    ├── logo.png 70 B", "    └── template.txt 130 B",
		}},
		{"mocked-skill-00006", []string{
			"FILES (9)", "├── SKILL.md 773 B",
			"├── assets/", "│   ├── logo.png 70 B", "│   └── template.txt 130 B",
			"├── references/", "│   ├── guides/", "│   │   └── advanced.md 136 B", "│   ├── api.md 110 B", "│   └── overview.md 120 B",
			"└── scripts/", "    ├── README.md 114 B", "    ├── check.py ⚙ 154 B", "    └── run.sh ⚙ 155 B",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.skill, func(t *testing.T) {
			m, _, _ := fixtureModel(t)
			m = onSkill(t, sized(t, m, 120, 40), tt.skill, "claude")
			got := filesOf(m)
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("FILES block:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

// TestTreeUsesTheAsciiConnectors checks the tree of fixture 00003 in the ascii set.
func TestTreeUsesTheAsciiConnectors(t *testing.T) {
	m, _, _ := fixtureModel(t)
	m = onSkill(t, sized(t, m, 120, 40), "mocked-skill-00003", "claude")
	m.icons, _ = icons.ByName("ascii")
	want := []string{
		"FILES (4)", "|-- SKILL.md 644 B", "`-- references/",
		"    |-- guides/", "    |   `-- advanced.md 248 B", "    |-- api.md 222 B", "    `-- overview.md 224 B",
	}
	if got := filesOf(m); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("FILES block:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestSkillFilesTreeHidesDotPathsAndSymlinks(t *testing.T) {
	m, _, _ := fixtureModel(t)
	m = onSkill(t, sized(t, m, 120, 40), "mocked-skill-00006", "claude")
	for _, hidden := range []string{".DS_Store", ".hidden", ".cache", "link.md"} {
		if strings.Contains(m.View(), hidden) {
			t.Errorf("skill screen shows %s", hidden)
		}
	}
}

func TestOrphanedSkillListsItsInstalledFiles(t *testing.T) {
	m, _, _ := fixtureModel(t)
	m = onSkill(t, sized(t, m, 120, 40), "mocked-skill-00099", "claude")
	want := []string{"FILES (1, installed in claude)", "└── SKILL.md 176 B"}
	if got := filesOf(m); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("FILES block:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestSkillWithoutCopyReportsNoFiles(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Missing)}}
	m := press(t, sized(t, loaded(t, b, []config.Agent{claude}), 120, 40), "enter")
	if got := filesOf(m); strings.Join(got, "\n") != "FILES\nno files" {
		t.Errorf("FILES block = %q, want the title and no files", got)
	}
}

func TestHumanSize(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{70, "70 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1<<20 - 1, "1024.0 KiB"},
		{1 << 20, "1.0 MiB"},
		{5<<20 + 1<<19, "5.5 MiB"},
	}
	for _, tt := range tests {
		if got := humanSize(tt.n); got != tt.want {
			t.Errorf("humanSize(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestAgentSectionShowsTheInstalledDateOfAManagedAgent(t *testing.T) {
	m, _, _ := fixtureModel(t)
	m = onSkill(t, sized(t, m, 120, 40), "mocked-skill-00001", "claude")
	installed := time.Date(2026, 11, 21, 13, 0, 0, 0, time.UTC).Local().Format("2006-01-02 15:04")
	if want := "· installed " + installed; !strings.Contains(m.View(), want) {
		t.Errorf("agent section lacks %q:\n%s", want, m.View())
	}
}

func TestInstalledDateIsShownInLocalTime(t *testing.T) {
	prev := time.Local
	time.Local = time.FixedZone("UTC-3", -3*60*60)
	t.Cleanup(func() { time.Local = prev })
	b := &fakeBackend{
		rows:    []ops.Row{row("alpha", "claude", ops.Synced)},
		entries: map[string]target.LockEntry{"/agents/claude/alpha": {Hash: "sha256:x", InstalledAt: time.Date(2026, 11, 21, 13, 0, 0, 0, time.UTC)}},
	}
	m := onSkill(t, sized(t, loaded(t, b, []config.Agent{claude}), 120, 40), "alpha", "claude")
	if want := "· installed 2026-11-21 10:00"; !strings.Contains(m.View(), want) {
		t.Errorf("13:00 UTC at UTC-3 should show %q:\n%s", want, m.View())
	}
}

func TestFailedDetailLoadStopsLoading(t *testing.T) {
	files := map[string][]ops.File{"/vault/skills/alpha": {{Path: "SKILL.md", Size: 70}}}
	tests := []struct {
		name      string
		filesErr  error
		lockErr   error
		wantFiles string
	}{
		{name: "files fail, so there are no files", filesErr: errors.New("files unreadable"),
			wantFiles: "FILES\nno files"},
		{name: "lock fails, and the files still show", lockErr: errors.New("lock unreadable"),
			wantFiles: "FILES (1)\n└── SKILL.md 70 B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &fakeBackend{
				rows:     []ops.Row{row("alpha", "claude", ops.Synced)},
				files:    files,
				filesErr: tt.filesErr,
				lockErr:  tt.lockErr,
			}
			m := onSkill(t, sized(t, loaded(t, b, []config.Agent{claude}), 120, 40), "alpha", "claude")
			view := m.View()
			if strings.Contains(view, "loading…") {
				t.Errorf("details still say loading…:\n%s", view)
			}
			if got := strings.Join(filesOf(m), "\n"); got != tt.wantFiles {
				t.Errorf("FILES block:\n%s\nwant:\n%s", got, tt.wantFiles)
			}
			want := tt.filesErr
			if want == nil {
				want = tt.lockErr
			}
			if !m.lastErr() || m.lastMsg() != want.Error() {
				t.Errorf("notification = %q (error %t), want the error %q", m.lastMsg(), m.lastErr(), want)
			}
		})
	}
}

func TestNotInVaultLineSharesTheLabelColumn(t *testing.T) {
	m, _, _ := fixtureModel(t)
	view := onSkill(t, sized(t, m, 120, 40), "mocked-skill-00099", "claude").View()
	want := "vault" + strings.Repeat(" ", labelW-len("vault")) + "not in vault"
	if !strings.Contains(view, want) {
		t.Errorf("view lacks %q:\n%s", want, view)
	}
	if strings.Contains(view, "vault not in vault") {
		t.Errorf("the not-in-vault line has one space after its label:\n%s", view)
	}
}

func TestAgentSectionShowsNoDateWhenNotInstalled(t *testing.T) {
	m, _, _ := fixtureModel(t)
	m = onSkill(t, sized(t, m, 120, 40), "mocked-skill-00004", "claude")
	if view := m.View(); strings.Contains(view, "installed 20") {
		t.Errorf("agent section of a skill not installed shows a date:\n%s", view)
	}
}

// agentSection returns the two lines of the section of agent in the details pane of m,
// and the row of the FILES title. The row is -1 when the pane lacks either.
func agentSection(m Model, agent string) ([]string, int) {
	lines := stripLines(m.detailLines(m.skillLayout().detail.w - 4))
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	i := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, agent+"  ") })
	files := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "FILES") })
	if i < 0 || i+2 > len(lines) || files < 0 {
		return nil, -1
	}
	return lines[i : i+2], files
}

func TestAgentSectionIsTwoLinesSoFilesStaysOnOneRow(t *testing.T) {
	_, set := testLook(t)
	codex := config.Agent{ID: "codex", Path: "/agents/codex"}
	zed := config.Agent{ID: "zed", Path: "/agents/zed"}
	b := &fakeBackend{
		rows: []ops.Row{
			row("alpha", "claude", ops.Synced),
			row("alpha", "cursor", ops.NotInstalled),
			incompatRow("alpha", "codex", ops.Synced),
			incompatRow("alpha", "zed", ops.Incompatible),
		},
		files: map[string][]ops.File{"/vault/skills/alpha": {{Path: "SKILL.md", Size: 70}}},
	}
	agents := []config.Agent{claude, cursorAgent, codex, zed}
	m := onSkill(t, sized(t, loaded(t, b, agents), 120, 40), "alpha", "claude")
	tests := []struct {
		agent string
		line1 string
		line2 string
	}{
		{"claude", set.Status[ops.Synced] + " synced", "  /agents/claude/alpha"},
		{"cursor", set.Status[ops.NotInstalled] + " not inst.", "  /agents/cursor/alpha (not installed)"},
		{"codex", set.Status[ops.Synced] + " synced · " + set.Status[ops.Incompatible] + " not compatible", "  /agents/codex/alpha"},
		{"zed", set.Status[ops.Incompatible] + " incompatible", "  /agents/zed/alpha (not installed)"},
	}
	files := -1
	for i, tt := range tests {
		if i > 0 {
			m = press(t, m, "down")
		}
		section, filesRow := agentSection(m, tt.agent)
		if section == nil {
			t.Fatalf("details lack the section of %s:\n%s", tt.agent, m.View())
		}
		if want := tt.agent + "  " + tt.line1; section[0] != want {
			t.Errorf("%s line 1 = %q, want %q", tt.agent, section[0], want)
		}
		if section[1] != tt.line2 {
			t.Errorf("%s line 2 = %q, want %q", tt.agent, section[1], tt.line2)
		}
		if files < 0 {
			files = filesRow
		}
		if filesRow != files {
			t.Errorf("FILES is on row %d for %s, want row %d as for the first agent", filesRow, tt.agent, files)
		}
	}
}

func TestNotInstalledPathLineIsDimmed(t *testing.T) {
	b := &fakeBackend{
		rows:  []ops.Row{row("alpha", "claude", ops.Synced), row("alpha", "cursor", ops.NotInstalled)},
		files: map[string][]ops.File{"/vault/skills/alpha": {{Path: "SKILL.md", Size: 70}}},
	}
	m := colored(t, onSkill(t, sized(t, loaded(t, b, []config.Agent{claude, cursorAgent}), 120, 40), "alpha", "cursor"))
	want := m.styles.dim.Render("  /agents/cursor/alpha (not installed)")
	lines := m.detailLines(m.skillLayout().detail.w - 4)
	if !slices.ContainsFunc(lines, func(l string) bool { return strings.Contains(l, want) }) {
		t.Errorf("details lack the would-be path dimmed as %q:\n%s", want, strings.Join(stripLines(lines), "\n"))
	}
}

func TestLockEntryReadsTheDetailOfTheCursorAgent(t *testing.T) {
	when := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	b := &fakeBackend{
		rows:    []ops.Row{row("alpha", "claude", ops.Synced), row("alpha", "cursor", ops.Synced)},
		entries: map[string]target.LockEntry{"/agents/cursor/alpha": {Hash: "sha256:x", InstalledAt: when}},
	}
	m := press(t, sized(t, loaded(t, b, []config.Agent{claude, cursorAgent}), 120, 40), "enter")
	if strings.Contains(m.View(), "· installed") {
		t.Errorf("claude has no lock entry, yet its section shows an install date")
	}
	m = press(t, m, "down")
	if want := "· installed " + when.Local().Format("2006-01-02 15:04"); !strings.Contains(m.View(), want) {
		t.Errorf("after the cursor moved to cursor, the section lacks %q:\n%s", want, m.View())
	}
}

// sidebarWidth is the width of the sidebar pane in view, the margin left out.
func sidebarWidth(t *testing.T, view string) int {
	t.Helper()
	line := lineWith(t, view, "AGENTS")
	before, _, found := strings.Cut(line, "╮")
	if !found {
		t.Fatalf("sidebar top border has no corner: %q", line)
	}
	return lipgloss.Width(before+"╮") - 1
}

func TestSidebarIsAtLeast26WideAndItsFocusUsesAccent(t *testing.T) {
	m := skillScreenModel(t, 120, 40)
	if w := sidebarWidth(t, m.View()); w < 26 {
		t.Errorf("sidebar is %d columns, want at least 26", w)
	}
	th, _ := testLook(t)
	colored := newStyles(th, false)
	if got := colored.focus.GetForeground(); got != th.Accent {
		t.Errorf("focused border color = %v, want the Accent role %v", got, th.Accent)
	}
	if got := colored.border.GetForeground(); got != th.Overlay {
		t.Errorf("details border color = %v, want the Overlay role %v", got, th.Overlay)
	}
	if plain := newStyles(th, true); !plain.focus.GetBold() {
		t.Errorf("NO_COLOR focused border is not bold, so the focus would not show")
	}
}

func TestSkillPanesStackBelow70Columns(t *testing.T) {
	m := skillScreenModel(t, 60, 20)
	lines := strings.Split(m.View(), "\n")
	agents, details := -1, -1
	for i, l := range lines {
		if agents < 0 && strings.Contains(l, "AGENTS") {
			agents = i
		}
		if details < 0 && strings.Contains(l, "╭─ alpha") {
			details = i
		}
	}
	if agents < 0 || details < 0 || agents >= details {
		t.Fatalf("sidebar at line %d, details at %d; want the sidebar above the details:\n%s", agents, details, m.View())
	}
	if w := lipgloss.Width(lines[agents]); w != 60 {
		t.Errorf("stacked sidebar is %d columns, want the full width 60", w)
	}
	if w := lipgloss.Width(lines[details]); w != 60 {
		t.Errorf("stacked details are %d columns, want the full width 60", w)
	}
}

func TestSkillPanesSideBySideAtAndAbove70Columns(t *testing.T) {
	m := skillScreenModel(t, 70, 20)
	line := lineWith(t, m.View(), "AGENTS")
	if !strings.Contains(line, "alpha") {
		t.Errorf("at 70 columns the details do not sit beside the sidebar: %q", line)
	}
}

func TestDetailsScrollWithPageKeys(t *testing.T) {
	m, _, _ := fixtureModel(t)
	m = onSkill(t, sized(t, m, 80, 24), "mocked-skill-00006", "claude")
	if view := m.View(); !strings.Contains(view, "version") || strings.Contains(view, "run.sh") {
		t.Fatalf("before scrolling, want the top of the details and not the last file:\n%s", view)
	}
	m = press(t, m, "pgdown")
	view := m.View()
	if strings.Contains(view, "version") || !strings.Contains(view, "run.sh") {
		t.Errorf("after pgdown, want the last file and not the top of the details:\n%s", view)
	}
	top := m.detailTop
	if m = press(t, m, "pgdown"); m.detailTop != top {
		t.Errorf("pgdown past the end moved the details to %d, want it kept at %d", m.detailTop, top)
	}
	if m = press(t, m, "pgup", "pgup"); m.detailTop != 0 || !strings.Contains(m.View(), "version") {
		t.Errorf("after pgup, top = %d, want the top of the details back", m.detailTop)
	}
}

func TestWrapTextKeepsThreeLines(t *testing.T) {
	tests := []struct {
		name string
		s    string
		w    int
		want []string
	}{
		{name: "empty has no lines", s: "", w: 20, want: nil},
		{name: "short text is one line", s: "Short.", w: 20, want: []string{"Short."}},
		{name: "three lines are kept whole", s: "one two three four five six", w: 12,
			want: []string{"one two", "three four", "five six"}},
		{name: "a fourth line ends the third with an ellipsis", s: "one two three four five six seven eight", w: 12,
			want: []string{"one two", "three four", "five six se…"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapText(tt.s, tt.w)
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("wrapText(%q, %d) = %q, want %q", tt.s, tt.w, got, tt.want)
			}
			for _, l := range got {
				if lipgloss.Width(l) > tt.w {
					t.Errorf("line %q is %d columns, want at most %d", l, lipgloss.Width(l), tt.w)
				}
			}
		})
	}
}

func TestEllipsisFitsItsWidth(t *testing.T) {
	tests := []struct {
		s    string
		w    int
		want string
	}{
		{"abc", 5, "abc…"},
		{"abc", 4, "abc…"},
		{"abcdefgh", 5, "abcd…"},
		{"abc", 1, "…"},
		{"abc", 0, ""},
	}
	for _, tt := range tests {
		if got := ellipsis(tt.s, tt.w); got != tt.want {
			t.Errorf("ellipsis(%q, %d) = %q, want %q", tt.s, tt.w, got, tt.want)
		}
	}
}

func TestBarRowOneCutsWithAnEllipsis(t *testing.T) {
	m, _ := layoutModel(t, 40, 20)
	plain := plainBar(m, m.View())[1]
	if w := lipgloss.Width(plain); w > 40 {
		t.Errorf("hint row is %d columns, want at most 40: %q", w, plain)
	}
	if !strings.HasSuffix(strings.TrimRight(plain, " "), "…") {
		t.Errorf("hint row is not cut with an ellipsis: %q", plain)
	}
	// The filter's input and hints fit in 40 columns, so they show whole.
	m = press(t, m, "/", "a")
	bar := plainBar(m, m.View())
	if strings.Contains(bar[1], "…") || !strings.Contains(bar[1], "enter apply · esc clear") || lipgloss.Width(bar[1]) > 40 {
		t.Errorf("filtering hint row = %q, want the input and its hints whole at 40 columns", bar[1])
	}
}

// TestLogHintOnTheModesThatOpenLog checks that the bar lists L log in the modes that
// open LOG, and not in the modes where L is text.
func TestLogHintOnTheModesThatOpenLog(t *testing.T) {
	tests := []struct {
		name    string
		steps   []string
		wantLog bool
	}{
		{name: "diff", steps: []string{"enter", "d"}, wantLog: true},
		{name: "help", steps: []string{"?"}, wantLog: true},
		{name: "adopt", steps: []string{"a"}, wantLog: true},
		{name: "theme", steps: []string{"t"}, wantLog: true},
		{name: "add", steps: []string{"n"}, wantLog: true},
		{name: "new", steps: []string{"n", "c"}, wantLog: false},
		{name: "import", steps: []string{"I"}, wantLog: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, b := layoutModel(t, 120, 40)
			b.changes = []ops.FileChange{{Rel: "SKILL.md", Kind: ops.ChangeChanged, Diff: "--- a\n+++ b\n@@ -1 +1 @@\n-old\n+new\n"}}
			m = press(t, m, tt.steps...)
			bar := strings.Join(plainBar(m, m.View()), "\n")
			if got := strings.Contains(bar, "L log"); got != tt.wantLog {
				t.Errorf("bar lists L log = %t, want %t: %q", got, tt.wantLog, bar)
			}
		})
	}
}

// detailText is the text of the details pane on a line of the skill screen, cut
// of its right border and trailing blanks.
func detailText(line string) string {
	parts := strings.Split(line, "│")
	return strings.TrimRight(parts[len(parts)-2], " ")
}

func TestSkillDetailsCutLongValuesWithAnEllipsis(t *testing.T) {
	view := skillScreenModel(t, 80, 24).View()
	for _, label := range []string{"vault", "problem text"} {
		if line := detailText(lineWith(t, view, label)); !strings.HasSuffix(line, "…") {
			t.Errorf("details line with %q does not end with an ellipsis: %q", label, line)
		}
	}
}

func TestSkillStatusKeepsItsWordAt80Columns(t *testing.T) {
	view := skillScreenModel(t, 80, 24).View()
	if line := strings.TrimSpace(detailText(lineWith(t, view, "invalid"))); line != "✖ invalid" {
		t.Errorf("status line = %q, want the whole word on its own line:\n%s", line, view)
	}
}

func TestSkillDescriptionEndsItsThirdLineWithAnEllipsis(t *testing.T) {
	lines := strings.Split(skillScreenModel(t, 80, 24).View(), "\n")
	first := -1
	for i, l := range lines {
		if strings.Contains(l, "Reviews") {
			first = i
			break
		}
	}
	if first < 0 {
		t.Fatalf("description is not shown")
	}
	if third := detailText(lines[first+2]); !strings.HasSuffix(third, "…") {
		t.Errorf("third description line %q does not end with an ellipsis", third)
	}
}

func TestSkillScreenFitsEverySizeWithLongValues(t *testing.T) {
	for _, size := range layoutSizes {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			view := skillScreenModel(t, size[0], size[1]).View()
			lines := strings.Split(view, "\n")
			if len(lines) != size[1] {
				t.Errorf("view has %d lines, want %d", len(lines), size[1])
			}
			for _, l := range lines {
				if w := lipgloss.Width(l); w > size[0] {
					t.Errorf("line is %d columns, want at most %d: %q", w, size[0], l)
				}
			}
		})
	}
}
