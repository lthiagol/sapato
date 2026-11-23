package tui

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"sapato/internal/config"
	"sapato/internal/ops"
	"sapato/internal/testfixtures"
	"sapato/internal/tui/icons"
	"sapato/internal/vault"
)

// stripLines strips the color codes from lines.
func stripLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = ansi.Strip(l)
	}
	return out
}

func TestIncompatibleCellShowsTheGlyphOfEverySet(t *testing.T) {
	th, _ := testLook(t)
	for _, name := range icons.Names() {
		t.Run(name, func(t *testing.T) {
			set, ok := icons.ByName(name)
			if !ok {
				t.Fatalf("no icon set %q", name)
			}
			b := &fakeBackend{rows: []ops.Row{incompatRow("alpha", "claude", ops.Incompatible)}}
			m := testModel(b, []config.Agent{claude}, th, set, true)
			m, _ = settle(t, m, m.Init())
			if line := lineWith(t, m.View(), "alpha"); !strings.Contains(line, set.Status[ops.Incompatible]) {
				t.Errorf("grid row %q lacks the glyph %q", line, set.Status[ops.Incompatible])
			}
		})
	}
}

func TestSidebarMarksAnInstalledCopyThatIsIncompatible(t *testing.T) {
	th, set := testLook(t)
	b := &fakeBackend{rows: []ops.Row{
		incompatRow("alpha", "claude", ops.Synced),
		incompatRow("alpha", "cursor", ops.Incompatible),
	}}
	m := testModel(b, []config.Agent{claude, cursorAgent}, th, set, true)
	m, _ = settle(t, m, m.Init())
	m = press(t, onRow(t, m, "alpha"), "enter")

	w := m.skillLayout().side.w - 4
	lines := stripLines(m.sideLines(w))
	installed, absent := lines[0], lines[1]
	if f := strings.Fields(installed); !slices.Contains(f, "synced") || f[len(f)-1] != set.Status[ops.Incompatible] {
		t.Errorf("installed copy row %q lacks the incompatible mark after its status", installed)
	}
	if f := strings.Fields(absent); f[len(f)-1] != "incompatible" {
		t.Errorf("absent row %q does not end with its status word", absent)
	}
	for i, l := range m.sideLines(w) {
		if lipgloss.Width(l) != w {
			t.Errorf("sidebar row %d is %d wide, want %d", i, lipgloss.Width(l), w)
		}
	}
}

func TestDetailsShowTheCompatListAndTheIncompatibleCopy(t *testing.T) {
	tests := []struct {
		name   string
		compat []string
		want   string
	}{
		{name: "every agent", compat: nil, want: "compat   all agents"},
		{name: "a list", compat: []string{"claude"}, want: "compat   claude"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			th, set := testLook(t)
			b := &fakeBackend{
				rows: []ops.Row{row("alpha", "claude", ops.NotInstalled), incompatRow("alpha", "cursor", ops.Synced)},
				vaultSkills: []vault.Skill{{
					Name: "alpha", Dir: "/vault/skills/alpha", Valid: true, Hash: "sha256:0123456789abcdef", Compat: tt.compat,
				}},
			}
			m := testModel(b, []config.Agent{claude, cursorAgent}, th, set, false)
			m, _ = settle(t, m, m.Init())
			m = onSkill(t, m, "alpha", "cursor")
			lines := stripLines(m.detailLines(60))
			if !containsLine(lines, tt.want) {
				t.Errorf("details lack %q:\n%s", tt.want, strings.Join(lines, "\n"))
			}
			if !containsLine(lines, "cursor  "+set.Status[ops.Synced]+" synced · "+set.Status[ops.Incompatible]+" not compatible") {
				t.Errorf("details lack the incompatible suffix on the agent line:\n%s", strings.Join(lines, "\n"))
			}
			if containsLine(lines, "not compatible with") {
				t.Errorf("details still have the separate incompatible line:\n%s", strings.Join(lines, "\n"))
			}
			if m.styles.warn.GetForeground() != th.Yellow {
				t.Errorf("warn style is %v, want the Yellow role %v", m.styles.warn.GetForeground(), th.Yellow)
			}
		})
	}
}

// containsLine reports whether some line of lines contains s.
func containsLine(lines []string, s string) bool {
	for _, l := range lines {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

func TestCompatKeyWritesAndClearsTheTableOnAFixtureCopy(t *testing.T) {
	vaultDir := testfixtures.CopyTree(t, filepath.Join(testfixtures.Root(), "skill-db"))
	v, err := vault.Open(vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	agents := []config.Agent{
		{ID: "claude", Path: t.TempDir()},
		{ID: "cursor", Path: t.TempDir()},
		{ID: "codex", Path: t.TempDir()},
	}
	th, set := testLook(t)
	m := testModel(opsBackend{v: v}, agents, th, set, true)
	m, _ = settle(t, m, m.Init())
	const skill = "mocked-skill-00001"
	m = onSkill(t, m, skill, "claude")

	// Marking claude and cursor and toggling writes the list of the agent left.
	m = press(t, m, "space", "down", "space", "c")
	if m.lastErr() || m.lastMsg() != skill+": codex" {
		t.Fatalf("result %q (err %t), want %s: codex", m.lastMsg(), m.lastErr(), skill)
	}
	reopened, err := vault.Open(vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.CompatAgents(skill); len(got) != 1 || got[0] != "codex" {
		t.Fatalf("stored list = %v, want [codex]", got)
	}

	// Toggling the marked agents on again covers every agent, so the table goes.
	m = press(t, m, "up", "c")
	if m.lastErr() || m.lastMsg() != skill+": all agents" {
		t.Fatalf("result %q (err %t), want %s: all agents", m.lastMsg(), m.lastErr(), skill)
	}
	reopened, err = vault.Open(vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.CompatAgents(skill); got != nil {
		t.Errorf("stored list = %v, want none", got)
	}
	if data := tomlOf(t, vaultDir); strings.Contains(data, "[skills."+skill+"]") {
		t.Errorf("sapato.toml still has the table:\n%s", data)
	}
}

func TestCompatKeyWithoutMarksActsOnTheCursorAgent(t *testing.T) {
	vaultDir := testfixtures.CopyTree(t, filepath.Join(testfixtures.Root(), "skill-db"))
	v, err := vault.Open(vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	agents := []config.Agent{
		{ID: "claude", Path: t.TempDir()},
		{ID: "cursor", Path: t.TempDir()},
		{ID: "codex", Path: t.TempDir()},
	}
	th, set := testLook(t)
	m := testModel(opsBackend{v: v}, agents, th, set, true)
	m, _ = settle(t, m, m.Init())
	const skill = "mocked-skill-00001"
	m = press(t, onSkill(t, m, skill, "cursor"), "c")

	// Nothing is marked, so only the cursor's agent is toggled off the full list.
	if m.lastErr() || m.lastMsg() != skill+": claude, codex" {
		t.Fatalf("result %q (err %t), want %s: claude, codex", m.lastMsg(), m.lastErr(), skill)
	}
	reopened, err := vault.Open(vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.CompatAgents(skill); !slices.Equal(got, []string{"claude", "codex"}) {
		t.Fatalf("stored list = %v, want [claude codex]", got)
	}
}

func TestCompatKeyRefusesASkillOutsideTheVault(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("orphan", "claude", ops.Orphaned)}}
	m := loaded(t, b, []config.Agent{claude})
	m = press(t, onRow(t, m, "orphan"), "enter", "c")
	if !m.lastErr() || m.lastMsg() != "orphan is not in the vault" {
		t.Errorf("result %q (err %t), want the not-in-vault error", m.lastMsg(), m.lastErr())
	}
	for _, c := range b.calls {
		if strings.HasPrefix(c, "compat") {
			t.Errorf("a refused c made the call %q", c)
		}
	}
}

func TestCompatErrorIsShown(t *testing.T) {
	b := &fakeBackend{
		rows:      []ops.Row{row("alpha", "claude", ops.Synced)},
		compatErr: errors.New("compat alpha: compatible list is empty"),
	}
	m := press(t, onSkill(t, loaded(t, b, []config.Agent{claude}), "alpha", "claude"), "c")
	if !m.lastErr() || m.lastMsg() != "compat alpha: compatible list is empty" {
		t.Errorf("result %q (err %t), want the ops error", m.lastMsg(), m.lastErr())
	}
}

func TestInstallAllSkipsIncompatibleAgentsAndReportsThem(t *testing.T) {
	tests := []struct {
		name      string
		rows      []ops.Row
		wantTitle string
		wantMsg   string
	}{
		{
			name:      "compatible agents install, the others are reported",
			rows:      []ops.Row{row("alpha", "claude", ops.NotInstalled), incompatRow("alpha", "cursor", ops.Incompatible)},
			wantTitle: "installed",
			wantMsg:   "install alpha in claude; skipped alpha in cursor: incompatible",
		},
		{
			name:      "only incompatible agents: nothing changes",
			rows:      []ops.Row{incompatRow("alpha", "claude", ops.Incompatible), incompatRow("alpha", "cursor", ops.Incompatible)},
			wantTitle: "no change",
			wantMsg:   "skipped alpha in claude: incompatible; skipped alpha in cursor: incompatible",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &fakeBackend{rows: tt.rows}
			m := press(t, onSkill(t, loaded(t, b, []config.Agent{claude, cursorAgent}), "alpha", "claude"), "A")
			if !b.lastOpts.SkipIncompatible {
				t.Error("install all did not ask ops to skip incompatible pairs")
			}
			if got := m.lastNote(); got.title != tt.wantTitle || got.message != tt.wantMsg {
				t.Errorf("notification %q: %q, want %q: %q", got.title, got.message, tt.wantTitle, tt.wantMsg)
			}
		})
	}
}

// tomlOf returns the sapato.toml of the vault at vaultDir.
func tomlOf(t *testing.T, vaultDir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(vaultDir, "sapato.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
