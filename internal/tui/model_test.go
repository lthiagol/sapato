package tui

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"sapato/internal/config"
	"sapato/internal/ops"
)

// fakeBackend answers the ops calls from canned data and records each call.
type fakeBackend struct {
	rows      []ops.Row
	statusErr error
	changes   []ops.FileChange
	diffErr   error
	inVault   map[string]bool  // skills already in the vault: an unforced adopt refuses them
	opErr     map[string]error // by op name: the error every call of that op returns
	noop      map[string]bool  // by op name: the call plans nothing
	calls     []string
}

func (f *fakeBackend) Status(_ []config.Agent) ([]ops.Row, error) {
	f.calls = append(f.calls, "status")
	return f.rows, f.statusErr
}

// needsForce is a refusal shaped like ops': its text is msg, and it matches
// ops.ErrNeedsForce.
type needsForce string

func (e needsForce) Error() string { return string(e) }

func (e needsForce) Unwrap() error { return ops.ErrNeedsForce }

// state is the status the fake reports for skill in agent, or "" for none.
func (f *fakeBackend) state(skill, agent string) ops.State {
	for _, r := range f.rows {
		if r.Skill == skill && r.Agent == agent {
			return r.Status
		}
	}
	return ""
}

// run answers install, uninstall and update. Without force, as in spec §7, ops
// refuses an install over an unmanaged or modified copy and an uninstall of a
// modified one, and an update skips a modified copy.
func (f *fakeBackend) run(op string, agents []config.Agent, skills []string, opts ops.Options) ([]ops.Action, error) {
	f.calls = append(f.calls, fmt.Sprintf("%s %s dry=%t force=%t", op, skills[0], opts.DryRun, opts.Force))
	if err := f.opErr[op]; err != nil {
		return nil, err
	}
	if f.noop[op] {
		return nil, nil
	}
	var acts []ops.Action
	for _, a := range agents {
		state := f.state(skills[0], a.ID)
		if !opts.Force {
			switch {
			case op == ops.OpUpdate && state == ops.Modified:
				acts = append(acts, ops.Action{Op: ops.OpSkip, Skill: skills[0], Agent: a.ID, Reason: ops.ReasonModified})
				continue
			case op == ops.OpInstall && (state == ops.Unmanaged || state == ops.Modified),
				op == ops.OpUninstall && state == ops.Modified:
				return nil, needsForce(fmt.Sprintf("%s %s in %s: %s (use --force)", op, skills[0], a.ID, state))
			}
		}
		acts = append(acts, ops.Action{Op: op, Skill: skills[0], Agent: a.ID, Path: filepath.Join(a.Path, skills[0])})
	}
	return acts, nil
}

func (f *fakeBackend) Install(agents []config.Agent, skills []string, opts ops.Options) ([]ops.Action, error) {
	return f.run(ops.OpInstall, agents, skills, opts)
}

func (f *fakeBackend) Uninstall(agents []config.Agent, skills []string, opts ops.Options) ([]ops.Action, error) {
	return f.run(ops.OpUninstall, agents, skills, opts)
}

func (f *fakeBackend) Update(agents []config.Agent, skills []string, opts ops.Options) ([]ops.Action, error) {
	return f.run(ops.OpUpdate, agents, skills, opts)
}

func (f *fakeBackend) Adopt(agent config.Agent, skill string, opts ops.Options) (ops.Action, error) {
	f.calls = append(f.calls, fmt.Sprintf("adopt %s dry=%t force=%t", skill, opts.DryRun, opts.Force))
	if err := f.opErr[ops.OpAdopt]; err != nil {
		return ops.Action{}, err
	}
	if f.inVault[skill] && !opts.Force {
		return ops.Action{}, needsForce(fmt.Sprintf("adopt %s in %s: already in the vault (use --force)", skill, agent.ID))
	}
	return ops.Action{Op: ops.OpAdopt, Skill: skill, Agent: agent.ID, Path: filepath.Join(agent.Path, skill)}, nil
}

func (f *fakeBackend) Diff(_ config.Agent, skill string) ([]ops.FileChange, error) {
	f.calls = append(f.calls, "diff "+skill)
	return f.changes, f.diffErr
}

var (
	claude      = config.Agent{ID: "claude", Path: "/agents/claude", Enabled: true}
	cursorAgent = config.Agent{ID: "cursor", Path: "/agents/cursor", Enabled: true}
)

func row(skill, agent string, st ops.State) ops.Row {
	return ops.Row{Skill: skill, Agent: agent, Status: st}
}

// keyMsg turns a key name as bubbletea spells it into a key message.
func keyMsg(s string) tea.KeyMsg {
	special := map[string]tea.KeyType{
		"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
		"enter": tea.KeyEnter, "esc": tea.KeyEsc, "ctrl+c": tea.KeyCtrlC,
	}
	if t, ok := special[s]; ok {
		return tea.KeyMsg{Type: t}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// settle runs cmd and feeds each result back into the model, until no command is
// left. It reports whether a quit was requested. The commands under test are
// the ops calls, which return at once.
func settle(t *testing.T, m Model, cmd tea.Cmd) (Model, bool) {
	t.Helper()
	for i := 0; cmd != nil; i++ {
		if i > 20 {
			t.Fatal("commands did not settle")
		}
		out := cmd()
		if _, ok := out.(tea.QuitMsg); ok {
			return m, true
		}
		next, c := m.Update(out)
		m, cmd = next.(Model), c
	}
	return m, false
}

// send delivers msg to m and settles the command it starts.
func send(t *testing.T, m Model, msg tea.Msg) (Model, bool) {
	t.Helper()
	next, cmd := m.Update(msg)
	return settle(t, next.(Model), cmd)
}

// press sends each key in turn and settles after each one.
func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		m, _ = send(t, m, keyMsg(k))
	}
	return m
}

// loaded returns a model over b that has loaded its status. The calls made
// so far are forgotten, so a test sees only its own calls.
func loaded(t *testing.T, b *fakeBackend, agents []config.Agent) Model {
	t.Helper()
	m := New(b, agents, true)
	m, _ = settle(t, m, m.Init())
	b.calls = nil
	return m
}

// sized returns m after the terminal reports a size, as bubbletea does first.
func sized(t *testing.T, m Model, width, height int) Model {
	t.Helper()
	m, _ = send(t, m, tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

// at moves the cursor onto skill in agent.
func at(t *testing.T, m Model, skill, agent string) Model {
	t.Helper()
	m.row = slices.Index(m.skills, skill)
	m.col = slices.IndexFunc(m.agents, func(a config.Agent) bool { return a.ID == agent })
	if m.row < 0 || m.col < 0 {
		t.Fatalf("no cell for %s in %s", skill, agent)
	}
	return m
}

func TestMatrixCursorStaysInBounds(t *testing.T) {
	tests := []struct {
		name    string
		keys    []string
		wantRow int
		wantCol int
	}{
		{name: "start at top left", keys: nil, wantRow: 0, wantCol: 0},
		{name: "up at top stays", keys: []string{"up", "k"}, wantRow: 0, wantCol: 0},
		{name: "left at edge stays", keys: []string{"left", "h"}, wantRow: 0, wantCol: 0},
		{name: "down and right move", keys: []string{"j", "l"}, wantRow: 1, wantCol: 1},
		{name: "down at bottom stays", keys: []string{"down", "down", "down"}, wantRow: 1, wantCol: 0},
		{name: "right at last agent stays", keys: []string{"right", "right", "right"}, wantRow: 0, wantCol: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &fakeBackend{rows: []ops.Row{
				row("alpha", "claude", ops.Synced), row("alpha", "cursor", ops.NotInstalled),
				row("beta", "claude", ops.Synced), row("beta", "cursor", ops.Synced),
			}}
			m := press(t, loaded(t, b, []config.Agent{claude, cursorAgent}), tt.keys...)
			if m.row != tt.wantRow || m.col != tt.wantCol {
				t.Errorf("cursor = (%d,%d), want (%d,%d)", m.row, m.col, tt.wantRow, tt.wantCol)
			}
		})
	}
}

func TestActionsOnCursorCell(t *testing.T) {
	tests := []struct {
		name      string
		state     ops.State
		key       string
		setup     func(*fakeBackend)
		answer    string // key sent at the confirm prompt, if one is shown
		wantMode  mode
		wantCalls []string
		wantMsg   string
		wantErr   bool
	}{
		{
			name: "install of a not-installed skill applies at once", state: ops.NotInstalled, key: "i",
			wantMode:  modeMatrix,
			wantCalls: []string{"install alpha dry=true force=false", "install alpha dry=false force=false", "status"},
			wantMsg:   "install alpha in claude",
		},
		{
			name: "uninstall waits for a plain yes", state: ops.Synced, key: "u",
			wantMode:  modeConfirm,
			wantCalls: []string{"uninstall alpha dry=true force=false"},
		},
		{
			name: "uninstall after yes", state: ops.Synced, key: "u", answer: "y",
			wantMode:  modeMatrix,
			wantCalls: []string{"uninstall alpha dry=true force=false", "uninstall alpha dry=false force=false", "status"},
			wantMsg:   "uninstall alpha in claude",
		},
		{
			name: "update of an outdated skill applies at once", state: ops.Outdated, key: "U",
			wantMode:  modeMatrix,
			wantCalls: []string{"update alpha dry=true force=false", "update alpha dry=false force=false", "status"},
			wantMsg:   "update alpha in claude",
		},
		{
			name: "update with nothing to do does not apply", state: ops.Synced, key: "U",
			setup:     func(b *fakeBackend) { b.noop = map[string]bool{ops.OpUpdate: true} },
			wantMode:  modeMatrix,
			wantCalls: []string{"update alpha dry=true force=false"},
			wantMsg:   "nothing to do",
		},
		{
			name: "adopt of an unmanaged folder into an empty vault applies at once", state: ops.Unmanaged, key: "a",
			wantMode:  modeMatrix,
			wantCalls: []string{"adopt alpha dry=true force=false", "adopt alpha dry=false force=false", "status"},
			wantMsg:   "adopt alpha in claude",
		},
		{
			name: "a dry run error is shown and nothing applies", state: ops.Synced, key: "u",
			setup:     func(b *fakeBackend) { b.opErr = map[string]error{ops.OpUninstall: errors.New("boom")} },
			wantMode:  modeMatrix,
			wantCalls: []string{"uninstall alpha dry=true force=false"},
			wantMsg:   "boom",
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", tt.state)}}
			if tt.setup != nil {
				tt.setup(b)
			}
			m := loaded(t, b, []config.Agent{claude})
			m = press(t, m, tt.key)
			if m.mode == modeConfirm && tt.answer != "" {
				m = press(t, m, tt.answer)
			}
			if m.mode != tt.wantMode {
				t.Errorf("mode = %v, want %v", m.mode, tt.wantMode)
			}
			if !slices.Equal(b.calls, tt.wantCalls) {
				t.Errorf("calls:\n got %q\nwant %q", b.calls, tt.wantCalls)
			}
			if tt.wantMsg != "" && m.result != tt.wantMsg {
				t.Errorf("result = %q, want %q", m.result, tt.wantMsg)
			}
			if m.resultErr != tt.wantErr {
				t.Errorf("resultErr = %t, want %t", m.resultErr, tt.wantErr)
			}
		})
	}
}

func TestConfirmCancelWithEscapeMakesNoCall(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced)}}
	m := press(t, loaded(t, b, []config.Agent{claude}), "u", "esc")
	if m.mode != modeMatrix || m.result != "cancelled" {
		t.Errorf("mode = %v, result = %q; want matrix and cancelled", m.mode, m.result)
	}
	if want := []string{"uninstall alpha dry=true force=false"}; !slices.Equal(b.calls, want) {
		t.Errorf("calls = %q, want %q", b.calls, want)
	}
}

func TestActionWithNoSkillReportsIt(t *testing.T) {
	b := &fakeBackend{}
	m := press(t, loaded(t, b, []config.Agent{claude}), "i", "d")
	if m.result != nothingSelected || !m.resultErr {
		t.Errorf("result = %q (err %t), want %q as an error", m.result, m.resultErr, nothingSelected)
	}
	if len(b.calls) != 0 {
		t.Errorf("calls = %q, want none", b.calls)
	}
}

func TestLoadErrorIsShownAndRowsKept(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced)}}
	m := loaded(t, b, []config.Agent{claude})
	b.statusErr = errors.New("disk gone")
	m = press(t, m, "r")
	if m.result != "disk gone" || !m.resultErr {
		t.Errorf("result = %q (err %t), want the load error", m.result, m.resultErr)
	}
	if got := m.skills; !slices.Equal(got, []string{"alpha"}) {
		t.Errorf("skills = %q, want the rows kept", got)
	}
}

func TestRefreshKeepsCursorOnSameSkill(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced), row("beta", "claude", ops.Synced)}}
	m := at(t, loaded(t, b, []config.Agent{claude}), "beta", "claude")
	b.rows = []ops.Row{row("alpha", "claude", ops.Synced), row("aardvark", "claude", ops.Synced), row("beta", "claude", ops.Synced)}
	m = press(t, m, "r")
	if got := m.skills[m.row]; got != "beta" {
		t.Errorf("cursor on %q, want beta", got)
	}
}

func TestDiffPagerOpensAndCloses(t *testing.T) {
	b := &fakeBackend{
		rows:    []ops.Row{row("alpha", "claude", ops.Modified)},
		changes: []ops.FileChange{{Rel: "SKILL.md", Kind: ops.ChangeChanged, Diff: "--- a\n+++ b\n@@ -1 +1 @@\n-old\n+new\n"}},
	}
	m := press(t, sized(t, loaded(t, b, []config.Agent{claude}), 80, 30), "d")
	if m.mode != modeDiff {
		t.Fatalf("mode = %v, want diff", m.mode)
	}
	if want := []string{"diff alpha"}; !slices.Equal(b.calls, want) {
		t.Errorf("calls = %q, want %q", b.calls, want)
	}
	view := m.View()
	for _, want := range []string{"diff alpha in claude", "changed SKILL.md", "+new", "-old"} {
		if !strings.Contains(view, want) {
			t.Errorf("diff view lacks %q:\n%s", want, view)
		}
	}
	if m = press(t, m, "esc"); m.mode != modeMatrix {
		t.Errorf("mode after esc = %v, want matrix", m.mode)
	}
}

func TestDiffWithNoChangesSaysSo(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced)}}
	m := press(t, sized(t, loaded(t, b, []config.Agent{claude}), 80, 30), "d")
	if !strings.Contains(m.View(), "no differences") {
		t.Errorf("view lacks the no-differences line:\n%s", m.View())
	}
}

func TestDiffErrorIsShown(t *testing.T) {
	b := &fakeBackend{
		rows:    []ops.Row{row("alpha", "claude", ops.Missing)},
		diffErr: errors.New("diff alpha in claude: not installed"),
	}
	m := press(t, loaded(t, b, []config.Agent{claude}), "d")
	if m.mode != modeMatrix || !m.resultErr || !strings.Contains(m.result, "not installed") {
		t.Errorf("mode = %v, result = %q (err %t); want matrix with the error", m.mode, m.result, m.resultErr)
	}
}

func TestFilterNarrowsRowsAndEscClears(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{
		row("alpha", "claude", ops.Synced), row("beta", "claude", ops.Synced), row("gamma", "claude", ops.Synced),
	}}
	m := loaded(t, b, []config.Agent{claude})

	tests := []struct {
		name     string
		keys     []string
		want     []string
		wantMode mode
		wantQ    string
	}{
		{name: "typing narrows live", keys: []string{"/", "A"}, want: []string{"alpha", "beta", "gamma"}, wantMode: modeFilter, wantQ: "A"},
		{name: "letters match case-insensitively", keys: []string{"/", "a", "m"}, want: []string{"gamma"}, wantMode: modeFilter, wantQ: "am"},
		{name: "enter keeps the filter", keys: []string{"/", "e", "t", "a", "enter"}, want: []string{"beta"}, wantMode: modeMatrix, wantQ: "eta"},
		{name: "esc clears the filter", keys: []string{"/", "b", "esc"}, want: []string{"alpha", "beta", "gamma"}, wantMode: modeMatrix, wantQ: ""},
		{name: "esc in matrix clears a kept filter", keys: []string{"/", "b", "enter", "esc"}, want: []string{"alpha", "beta", "gamma"}, wantMode: modeMatrix, wantQ: ""},
		{name: "no match leaves no rows", keys: []string{"/", "z"}, want: nil, wantMode: modeFilter, wantQ: "z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := press(t, m, tt.keys...)
			if !slices.Equal(got.skills, tt.want) {
				t.Errorf("skills = %q, want %q", got.skills, tt.want)
			}
			if got.mode != tt.wantMode || got.query != tt.wantQ {
				t.Errorf("mode = %v, query = %q; want %v, %q", got.mode, got.query, tt.wantMode, tt.wantQ)
			}
		})
	}
}

func TestQuitKeys(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced)}}
	tests := []struct {
		name string
		keys []string
		want bool
	}{
		{name: "q quits from the matrix", keys: []string{"q"}, want: true},
		{name: "ctrl+c quits from the matrix", keys: []string{"ctrl+c"}, want: true},
		{name: "ctrl+c quits while filtering", keys: []string{"/", "ctrl+c"}, want: true},
		{name: "q while filtering is text", keys: []string{"/", "q"}, want: false},
		{name: "q in help closes help without quitting", keys: []string{"?", "q"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := loaded(t, b, []config.Agent{claude})
			var quit bool
			for _, k := range tt.keys {
				m, quit = send(t, m, keyMsg(k))
			}
			if quit != tt.want {
				t.Errorf("quit = %t, want %t", quit, tt.want)
			}
		})
	}
}

func TestHelpTogglesAndLegendCoversAllStates(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced)}}
	m := press(t, loaded(t, b, []config.Agent{claude}), "?")
	if m.mode != modeHelp {
		t.Fatalf("mode = %v, want help", m.mode)
	}
	view := m.View()
	for _, st := range states {
		if !strings.Contains(view, string(st.state)) || !strings.Contains(view, st.desc) {
			t.Errorf("help lacks the legend entry for %s", st.state)
		}
	}
	if m = press(t, m, "?"); m.mode != modeMatrix {
		t.Errorf("mode after second ? = %v, want matrix", m.mode)
	}
}

func TestEveryStatusHasAGlyphInTheMatrix(t *testing.T) {
	var rows []ops.Row
	for i, st := range states {
		rows = append(rows, row(fmt.Sprintf("skill-%d", i), "claude", st.state))
	}
	m := New(&fakeBackend{rows: rows}, []config.Agent{claude}, true)
	m, _ = settle(t, m, m.Init())
	view := m.View()
	if len(states) != 8 {
		t.Fatalf("states has %d entries, want the 8 of spec §6", len(states))
	}
	for _, st := range states {
		if !strings.Contains(view, st.glyph) {
			t.Errorf("matrix lacks the glyph %q of %s", st.glyph, st.state)
		}
	}
}

func TestScrollKeepsCursorRowInWindow(t *testing.T) {
	var rows []ops.Row
	for i := range 10 {
		rows = append(rows, row(fmt.Sprintf("skill-%02d", i), "claude", ops.Synced))
	}
	b := &fakeBackend{rows: rows}
	m := sized(t, loaded(t, b, []config.Agent{claude}), 80, matrixChrome+2)

	m = press(t, m, "j", "j", "j", "j", "j")
	if m.row != 5 || m.top != 4 {
		t.Errorf("after 5 downs: row = %d, top = %d; want row 5, top 4", m.row, m.top)
	}
	if view := m.View(); !strings.Contains(view, "skill-05") || strings.Contains(view, "skill-00") {
		t.Errorf("window does not show the cursor row alone:\n%s", view)
	}
	m = press(t, m, "k", "k", "k", "k", "k")
	if m.row != 0 || m.top != 0 {
		t.Errorf("after 5 ups: row = %d, top = %d; want 0, 0", m.row, m.top)
	}
}

func TestViewFitsTheWidth(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{
		row("a-rather-long-skill-name", "claude", ops.Modified), row("a-rather-long-skill-name", "cursor", ops.Synced),
	}}
	m := sized(t, loaded(t, b, []config.Agent{claude, cursorAgent}), 24, 20)
	for _, line := range strings.Split(m.View(), "\n") {
		if w := lipgloss.Width(line); w > 24 {
			t.Errorf("line is %d columns wide, want at most 24: %q", w, line)
		}
	}
}

func TestNoColorDropsForegroundColors(t *testing.T) {
	plain := newStyles(true)
	colored := newStyles(false)
	for _, st := range states {
		if _, ok := plain.glyph[st.state].GetForeground().(lipgloss.NoColor); !ok {
			t.Errorf("NO_COLOR glyph style for %s sets a foreground", st.state)
		}
		if _, ok := colored.glyph[st.state].GetForeground().(lipgloss.NoColor); ok {
			t.Errorf("color glyph style for %s has no foreground", st.state)
		}
	}
}

func TestTruncateAndCenter(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "short name kept", got: truncate("abc", 5), want: "abc"},
		{name: "long name cut with ellipsis", got: truncate("abcdef", 4), want: "abc…"},
		{name: "width one keeps one rune", got: truncate("abc", 1), want: "a"},
		{name: "center splits the gap, extra space on the right", got: center("x", 4), want: " x  "},
		{name: "center with no gap", got: center("xy", 2), want: "xy"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

// forceCase is an action that ops refuses without --force, and the force prompt
// it must open.
type forceCase struct {
	name    string
	key     string
	state   ops.State
	inVault bool   // the vault already has the skill
	op      string // the op name, as the prompt and ops record it
	reason  string // the refusal the prompt shows
	effect  string // what the prompt says is lost
}

var forceCases = []forceCase{
	{
		name: "install over an unmanaged folder", key: "i", state: ops.Unmanaged,
		op: ops.OpInstall, reason: "install alpha in claude: unmanaged (use --force)",
		effect: "Overwrites the existing folder, including anything in it.",
	},
	{
		name: "install over a modified copy", key: "i", state: ops.Modified,
		op: ops.OpInstall, reason: "install alpha in claude: modified (use --force)",
		effect: "Overwrites the existing folder, including anything in it.",
	},
	{
		name: "uninstall of a modified copy", key: "u", state: ops.Modified,
		op: ops.OpUninstall, reason: "uninstall alpha in claude: modified (use --force)",
		effect: "Local edits are lost with the installed copy and its lock entry.",
	},
	{
		name: "update of a modified copy", key: "U", state: ops.Modified,
		op: ops.OpUpdate, reason: "skip alpha in claude: modified",
		effect: "Overwrites local edits in the installed copy.",
	},
	{
		name: "adopt over a skill already in the vault", key: "a", state: ops.Unmanaged, inVault: true,
		op: ops.OpAdopt, reason: "adopt alpha in claude: already in the vault (use --force)",
		effect: "Replaces the vault copy of alpha with this folder.",
	},
}

// forceStart presses the key of tt on its cell and returns the model with its backend.
func forceStart(t *testing.T, tt forceCase) (Model, *fakeBackend) {
	t.Helper()
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", tt.state)}}
	if tt.inVault {
		b.inVault = map[string]bool{"alpha": true}
	}
	return press(t, loaded(t, b, []config.Agent{claude}), tt.key), b
}

func TestForceRefusalAsksForForce(t *testing.T) {
	for _, tt := range forceCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("asks for force after an unforced dry run", func(t *testing.T) {
				m, b := forceStart(t, tt)
				if want := []string{tt.op + " alpha dry=true force=false"}; !slices.Equal(b.calls, want) {
					t.Errorf("calls = %q, want %q", b.calls, want)
				}
				if m.mode != modeConfirm || !m.pending.act.force {
					t.Fatalf("mode = %v, force = %t; want the force prompt", m.mode, m.pending.act.force)
				}
				view := m.View()
				for _, want := range []string{"Force " + tt.op + " alpha in claude?", tt.reason, tt.effect} {
					if !strings.Contains(view, want) {
						t.Errorf("force prompt lacks %q:\n%s", want, view)
					}
				}
			})
			t.Run("any key but y cancels without a further call", func(t *testing.T) {
				for _, k := range []string{"n", "esc", "q", "x"} {
					m, b := forceStart(t, tt)
					m = press(t, m, k)
					if want := []string{tt.op + " alpha dry=true force=false"}; !slices.Equal(b.calls, want) {
						t.Errorf("%s: calls = %q, want %q", k, b.calls, want)
					}
					if m.mode != modeMatrix || m.result != "cancelled" {
						t.Errorf("%s: mode = %v, result = %q; want matrix and cancelled", k, m.mode, m.result)
					}
				}
			})
			t.Run("y applies with Force", func(t *testing.T) {
				m, b := forceStart(t, tt)
				m = press(t, m, "y")
				want := []string{tt.op + " alpha dry=true force=false", tt.op + " alpha dry=false force=true", "status"}
				if !slices.Equal(b.calls, want) {
					t.Errorf("calls = %q, want %q", b.calls, want)
				}
				if m.mode != modeMatrix || m.resultErr || m.result != tt.op+" alpha in claude" {
					t.Errorf("mode = %v, result = %q (err %t); want matrix and %s alpha in claude", m.mode, m.result, m.resultErr, tt.op)
				}
			})
		})
	}
}

func TestPlainConfirmNeverForces(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced)}}
	m := press(t, loaded(t, b, []config.Agent{claude}), "u")
	if m.mode != modeConfirm || m.pending.act.force {
		t.Fatalf("mode = %v, force = %t; want the plain confirm", m.mode, m.pending.act.force)
	}
	view := m.View()
	for _, want := range []string{"uninstall alpha in claude?", "uninstall " + filepath.Join(claude.Path, "alpha"), "Removes the installed copy and its lock entry. The vault copy stays.", "y yes · n no"} {
		if !strings.Contains(view, want) {
			t.Errorf("plain confirm lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Force") {
		t.Errorf("plain confirm names a force:\n%s", view)
	}
	m = press(t, m, "y")
	want := []string{"uninstall alpha dry=true force=false", "uninstall alpha dry=false force=false", "status"}
	if !slices.Equal(b.calls, want) {
		t.Errorf("calls = %q, want %q", b.calls, want)
	}
}
