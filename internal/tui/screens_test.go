package tui

import (
	"slices"
	"strings"
	"testing"

	"sapato/internal/config"
	"sapato/internal/ops"
)

func TestEnterOpensSkillAndEscReturns(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced), row("beta", "claude", ops.Synced)}}
	m := press(t, onRow(t, loaded(t, b, []config.Agent{claude}), "beta"), "enter")
	if m.screen != screenSkill || m.open != "beta" || m.mode != modeNormal {
		t.Fatalf("after enter: screen %v, open %q, mode %v; want the skill screen for beta", m.screen, m.open, m.mode)
	}
	if m = press(t, m, "esc"); m.screen != screenMain {
		t.Errorf("after esc: screen = %v, want main", m.screen)
	}
	if m.row != 1 {
		t.Errorf("grid cursor = %d, want it kept on beta (1)", m.row)
	}
}

func TestEnterOnEmptyGridReportsIt(t *testing.T) {
	m := press(t, loaded(t, &fakeBackend{}, []config.Agent{claude}), "enter")
	if m.screen != screenMain || m.lastMsg() != nothingSelected || !m.lastErr() {
		t.Errorf("screen %v, result %q (err %t); want main and %q as an error", m.screen, m.lastMsg(), m.lastErr(), nothingSelected)
	}
}

func TestSkillCursorMovesOverAgents(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced), row("alpha", "cursor", ops.NotInstalled)}}
	m := press(t, loaded(t, b, []config.Agent{claude, cursorAgent}), "enter")
	tests := []struct {
		keys []string
		want int
	}{
		{keys: []string{"down"}, want: 1},
		{keys: []string{"down", "j"}, want: 1},
		{keys: []string{"up"}, want: 0},
		{keys: []string{"j", "k", "k"}, want: 0},
	}
	for _, tt := range tests {
		if got := press(t, m, tt.keys...).cur; got != tt.want {
			t.Errorf("keys %q: cursor = %d, want %d", tt.keys, got, tt.want)
		}
	}
}

func TestMarksChooseTheActionTargets(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.NotInstalled), row("alpha", "cursor", ops.NotInstalled)}}
	m := press(t, loaded(t, b, []config.Agent{claude, cursorAgent}), "enter")

	// With no mark, the action goes to the cursor's agent.
	m = press(t, m, "i")
	if !slices.Equal(b.lastAgents, []string{"claude"}) {
		t.Errorf("unmarked install went to %q, want the cursor's claude", b.lastAgents)
	}

	// Marking the second agent and moving the cursor back still targets only it.
	b.lastAgents = nil
	m = press(t, m, "down", " ", "up", "i")
	if !slices.Equal(b.lastAgents, []string{"cursor"}) {
		t.Errorf("marked install went to %q, want cursor", b.lastAgents)
	}
	if !m.marks["cursor"] || m.marks["claude"] {
		t.Errorf("marks = %v, want only cursor", m.marks)
	}

	// Marks apply in agent order, and a second space unmarks.
	b.lastAgents = nil
	m = press(t, m, "up", " ", "U")
	if !slices.Equal(b.lastAgents, []string{"claude", "cursor"}) {
		t.Errorf("two marks went to %q, want claude then cursor", b.lastAgents)
	}
	m = press(t, m, "down", " ")
	if m.marks["cursor"] {
		t.Errorf("second space left cursor marked: %v", m.marks)
	}
}

func TestMarksAreClearedOnLeavingTheSkill(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced)}}
	m := press(t, loaded(t, b, []config.Agent{claude}), "enter", " ", "esc", "enter")
	if len(m.marks) != 0 {
		t.Errorf("marks after reopening = %v, want none", m.marks)
	}
}

func TestInstallAllTargetsEveryConfiguredAgent(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.NotInstalled), row("alpha", "cursor", ops.NotInstalled)}}
	m := press(t, loaded(t, b, []config.Agent{claude, cursorAgent}), "enter", "down", " ", "A")
	if !slices.Equal(b.lastAgents, []string{"claude", "cursor"}) {
		t.Errorf("A went to %q, want every configured agent", b.lastAgents)
	}
	if m.lastMsg() != "install alpha in claude; install alpha in cursor" {
		t.Errorf("result = %q", m.lastMsg())
	}
}

func TestSkillActionsNeedTheVaultExceptUninstall(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Orphaned)}}
	m := loaded(t, b, []config.Agent{claude})
	for _, key := range []string{"i", "U", "d"} {
		got := press(t, onSkill(t, m, "alpha", "claude"), key)
		if !got.lastErr() || got.lastMsg() != "alpha is not in the vault" {
			t.Errorf("%s: result = %q (err %t), want the vault refusal", key, got.lastMsg(), got.lastErr())
		}
	}
	if len(b.calls) != 0 {
		t.Errorf("calls = %q, want none", b.calls)
	}
	if got := press(t, onSkill(t, m, "alpha", "claude"), "u"); got.mode != modeConfirm {
		t.Errorf("uninstall of an orphan: mode = %v, want the confirm", got.mode)
	}
}

func TestAdoptNeedsAnUnmanagedCopy(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced)}}
	m := press(t, loaded(t, b, []config.Agent{claude}), "enter", "a")
	if m.lastMsg() != "adopt needs an unmanaged copy" || !m.lastErr() {
		t.Errorf("result = %q (err %t)", m.lastMsg(), m.lastErr())
	}
	if len(b.calls) != 0 {
		t.Errorf("calls = %q, want none", b.calls)
	}
}

func TestDiffAndHelpReturnToTheSkillScreen(t *testing.T) {
	b := &fakeBackend{
		rows:    []ops.Row{row("alpha", "claude", ops.Modified)},
		changes: []ops.FileChange{{Rel: "SKILL.md", Kind: ops.ChangeChanged, Diff: "--- a\n+++ b\n@@ -1 +1 @@\n-old\n+new\n"}},
	}
	m := press(t, loaded(t, b, []config.Agent{claude}), "enter", "d")
	if m = press(t, m, "esc"); m.mode != modeNormal || m.screen != screenSkill {
		t.Errorf("after the diff's esc: mode %v, screen %v; want the skill screen", m.mode, m.screen)
	}
	m = press(t, m, "?")
	if m.mode != modeHelp || m.screen != screenSkill {
		t.Fatalf("after ?: mode %v, screen %v; want help over the skill screen", m.mode, m.screen)
	}
	if m = press(t, m, "?"); m.mode != modeNormal || m.screen != screenSkill {
		t.Errorf("after the second ?: mode %v, screen %v; want the skill screen", m.mode, m.screen)
	}
}

func TestSkillGoneReturnsToTheGrid(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Orphaned), row("beta", "claude", ops.Synced)}}
	m := press(t, onSkill(t, loaded(t, b, []config.Agent{claude}), "alpha", "claude"), "u")
	// The uninstall removes the orphaned copy, so the next status no longer lists it.
	b.rows = []ops.Row{row("beta", "claude", ops.Synced)}
	m = press(t, m, "y")
	if m.screen != screenMain {
		t.Errorf("screen = %v after its skill was removed, want main", m.screen)
	}
}

func TestSkillKeysAreNotMainKeys(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced)}}
	m := press(t, loaded(t, b, []config.Agent{claude}), "i", "u", "U", "d", "space", " ")
	if len(b.calls) != 0 || m.mode != modeNormal || m.lastMsg() != "" {
		t.Errorf("main screen acted on skill keys: calls %q, mode %v, result %q", b.calls, m.mode, m.lastMsg())
	}
}

func TestQuitFromTheSkillScreen(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced)}}
	m := press(t, loaded(t, b, []config.Agent{claude}), "enter")
	if _, quit := send(t, m, keyMsg("q")); !quit {
		t.Error("q on the skill screen did not quit")
	}
}

func TestPageKeysMoveByAPage(t *testing.T) {
	var rows []ops.Row
	for _, name := range []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10", "a11", "a12"} {
		rows = append(rows, row(name, "claude", ops.Synced))
	}
	b := &fakeBackend{rows: rows}
	m := sized(t, loaded(t, b, []config.Agent{claude}), 80, 10)
	vis := m.visibleRows()
	m = press(t, m, "pgdown")
	if m.row != vis {
		t.Errorf("after pgdown: row = %d, want %d", m.row, vis)
	}
	m = press(t, m, "pgup")
	if m.row != 0 {
		t.Errorf("after pgup: row = %d, want 0", m.row)
	}
}

func TestFilterIsNotOfferedOnTheSkillScreen(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced), row("beta", "claude", ops.Synced)}}
	m := press(t, loaded(t, b, []config.Agent{claude}), "enter", "/")
	if m.mode == modeFilter {
		t.Errorf("/ opened the filter on the skill screen")
	}
}

func TestBarShowsTheFilterInput(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced)}}
	m := sized(t, loaded(t, b, []config.Agent{claude}), 80, 24)
	m = press(t, m, "/", "a")
	first, second := barRows(m.View())
	if plain := sgr.ReplaceAllString(first, ""); !strings.Contains(plain, "/ a") || !strings.Contains(plain, "enter apply · esc clear") {
		t.Errorf("filter bar row 1 = %q, want the input, then its hints", plain)
	}
	if plain := sgr.ReplaceAllString(second, ""); !strings.Contains(plain, "1 skills · 0 invalid · 0 need attention") {
		t.Errorf("filter bar row 2 = %q, want the counts", plain)
	}
}

// TestThemeOpensFromTheSkillScreenAndEscReturnsToIt checks that t opens THEME on the
// skill screen, and that esc closes it back to the skill screen.
func TestThemeOpensFromTheSkillScreenAndEscReturnsToIt(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced)}}
	m := press(t, loaded(t, b, []config.Agent{claude}), "enter", "t")
	if m.mode != modeTheme || m.screen != screenSkill {
		t.Fatalf("mode = %v, screen = %v; want THEME over the skill screen", m.mode, m.screen)
	}
	if m = press(t, m, "esc"); m.mode != modeNormal || m.screen != screenSkill {
		t.Errorf("after esc: mode = %v, screen = %v; want the skill screen", m.mode, m.screen)
	}
}
