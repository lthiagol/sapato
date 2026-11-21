package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"sapato/internal/config"
	"sapato/internal/ops"
	"sapato/internal/tui/theme"
	"sapato/internal/vault"
)

// freshSkill is a valid SKILL.md for a skill that the fixtures do not have.
const freshSkill = "---\nname: fresh-skill\ndescription: A skill made in the NEW dialog.\n---\n\n# Fresh skill\n"

// The dialog calls of the fake backend. Its ops calls are recorded in calls, as
// the other calls are.

func (f *fakeBackend) Create(content []byte, opts ops.Options) (ops.Action, error) {
	f.calls = append(f.calls, fmt.Sprintf("create dry=%t force=%t", opts.DryRun, opts.Force))
	if err := f.opErr[ops.OpCreate]; err != nil {
		return ops.Action{}, err
	}
	s := vault.ParseContent(content)
	if !s.Valid {
		return ops.Action{}, fmt.Errorf("create: invalid skill: %s", strings.Join(s.Problems, "; "))
	}
	if f.inVault[s.Name] && !opts.Force {
		return ops.Action{}, refusal(fmt.Sprintf("create %s: already in the vault (use --force)", s.Name), "already in the vault")
	}
	return ops.Action{Op: ops.OpCreate, Skill: s.Name, Path: "/vault/skills/" + s.Name}, nil
}

func (f *fakeBackend) Import(src string, opts ops.Options) ([]ops.Action, error) {
	f.calls = append(f.calls, fmt.Sprintf("import %s dry=%t force=%t", src, opts.DryRun, opts.Force))
	if err := f.opErr[ops.OpImport]; err != nil {
		return nil, err
	}
	name := strings.TrimSuffix(filepath.Base(src), ".tar.gz")
	if f.inVault[name] && !opts.Force {
		if strings.HasSuffix(src, ".tar.gz") {
			return nil, refusal(fmt.Sprintf("import %s: already in the vault (use --force): %s", src, name), "already in the vault: "+name)
		}
		return nil, refusal(fmt.Sprintf("import %s: %s is already in the vault (use --force)", src, name), "already in the vault")
	}
	return []ops.Action{{Op: ops.OpImport, Skill: name, Path: "/vault/skills/" + name}}, nil
}

func (f *fakeBackend) Remove(skill string, opts ops.Options) (ops.Action, error) {
	f.calls = append(f.calls, fmt.Sprintf("remove %s dry=%t force=%t", skill, opts.DryRun, opts.Force))
	if err := f.opErr[ops.OpRemove]; err != nil {
		return ops.Action{}, err
	}
	for _, r := range f.rows {
		installed := r.Status == ops.Synced || r.Status == ops.Outdated || r.Status == ops.Modified
		if r.Skill == skill && installed && !opts.Force {
			return ops.Action{}, refusal(fmt.Sprintf("remove %s: installed in %s (use --force)", skill, r.Agent), "installed in "+r.Agent)
		}
	}
	return ops.Action{Op: ops.OpRemove, Skill: skill, Path: "/vault/skills/" + skill}, nil
}

func (f *fakeBackend) ExportName(skill string) (string, error) {
	f.calls = append(f.calls, "export-name "+skill)
	return skill + "-1.0.0.skill.tar.gz", nil
}

func (f *fakeBackend) Export(skill, path string) ([]ops.Action, error) {
	f.calls = append(f.calls, fmt.Sprintf("export %s %s", skill, path))
	return nil, f.opErr[opExport]
}

// typeText types text into the open dialog, one key per rune and enter for each
// line break.
func typeText(t *testing.T, m Model, text string) Model {
	t.Helper()
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			m = press(t, m, "enter")
		}
		for _, r := range line {
			m = press(t, m, string(r))
		}
	}
	return m
}

// dialogAgents are the agents the dialog tests run over.
var dialogAgents = []config.Agent{claude, cursorAgent}

func TestDialogKeysOpenTheirDialogs(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced)}}
	tests := []struct {
		name string
		keys []string
		want mode
	}{
		{name: "main a opens ADOPT", keys: []string{"a"}, want: modeAdopt},
		{name: "main n opens NEW", keys: []string{"n"}, want: modeNew},
		{name: "main I opens IMPORT", keys: []string{"I"}, want: modeImport},
		{name: "main t opens THEME", keys: []string{"t"}, want: modeTheme},
		{name: "skill x opens EXPORT", keys: []string{"enter", "x"}, want: modeExport},
		{name: "skill D asks before removing", keys: []string{"enter", "D"}, want: modeConfirm},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := press(t, loaded(t, b, []config.Agent{claude}), tt.keys...)
			if m.mode != tt.want {
				t.Errorf("mode = %v, want %v; result %q", m.mode, tt.want, m.lastMsg())
			}
		})
	}
}

func TestAdoptPickerAdoptsAndStaysOpen(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{
		row("alpha", "claude", ops.Unmanaged), row("beta", "cursor", ops.Unmanaged), row("gamma", "claude", ops.Synced),
	}}
	m := press(t, loaded(t, b, dialogAgents), "a")
	if got := len(m.unmanaged()); got != 2 {
		t.Fatalf("picker lists %d copies, want 2", got)
	}
	m = press(t, m, "down", "enter")
	want := []string{"adopt beta dry=true force=false", "adopt beta dry=false force=false", "status"}
	if !slices.Equal(b.calls, want) {
		t.Errorf("calls = %q, want %q", b.calls, want)
	}
	if m.mode != modeAdopt || m.lastErr() || m.lastMsg() != "adopt beta in cursor" {
		t.Errorf("mode %v, result %q (err %t); want the picker open with the result", m.mode, m.lastMsg(), m.lastErr())
	}
	if m = press(t, m, "esc"); m.mode != modeNormal {
		t.Errorf("mode after esc = %v, want the grid", m.mode)
	}
}

func TestAdoptPickerWithNothingToAdopt(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced)}}
	m := press(t, loaded(t, b, []config.Agent{claude}), "a", "enter")
	if len(b.calls) != 0 {
		t.Errorf("calls = %q, want none", b.calls)
	}
	if view := m.View(); !strings.Contains(view, "No unmanaged skills") {
		t.Errorf("empty picker lacks its message:\n%s", view)
	}
}

func TestAdoptPickerForceFlow(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Unmanaged)}, inVault: map[string]bool{"alpha": true}}
	m := press(t, loaded(t, b, []config.Agent{claude}), "a", "enter")
	if m.mode != modeConfirm || !m.pending.act.force {
		t.Fatalf("mode %v, force %t; want the force prompt", m.mode, m.pending.act.force)
	}
	m = press(t, m, "n")
	if last := m.lastNote(); m.mode != modeAdopt || last.title != "cancelled" || last.message != "adopt alpha in claude" {
		t.Errorf("after n: mode %v, note %q %q; want the picker and cancelled adopt", m.mode, last.title, last.message)
	}
	m = press(t, m, "enter", "y")
	if m.mode != modeAdopt || m.lastErr() || m.lastMsg() != "adopt alpha in claude" {
		t.Errorf("after y: mode %v, result %q (err %t)", m.mode, m.lastMsg(), m.lastErr())
	}
	if !slices.Contains(b.calls, "adopt alpha dry=false force=true") {
		t.Errorf("calls = %q, want the forced adopt", b.calls)
	}
}

func TestNewCancelsAndDiscards(t *testing.T) {
	b := &fakeBackend{}
	m := press(t, loaded(t, b, dialogAgents), "n")
	if m.mode != modeNew {
		t.Fatalf("mode = %v, want NEW", m.mode)
	}
	if got := press(t, m, "esc"); got.mode != modeNormal {
		t.Errorf("esc on an empty editor: mode = %v, want the grid", got.mode)
	}

	m = press(t, m, "x", "esc")
	if m.mode != modeConfirm || m.pending.act.op != opDiscard {
		t.Fatalf("esc on text: mode %v, pending %q; want the discard confirm", m.mode, m.pending.act.op)
	}
	if view := m.View(); !strings.Contains(view, "Discard the new skill?") {
		t.Errorf("discard confirm lacks its question:\n%s", view)
	}
	m = press(t, m, "n")
	if m.mode != modeNew || m.area.Value() != "x" {
		t.Errorf("after n: mode %v, text %q; want the editor with its text", m.mode, m.area.Value())
	}
	m = press(t, m, "esc", "y")
	if m.mode != modeNormal || m.lastMsg() != "new skill discarded" {
		t.Errorf("after y: mode %v, result %q; want the grid", m.mode, m.lastMsg())
	}
}

func TestNewLiveCheck(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "empty asks for a skill", text: "", want: "paste a complete SKILL.md"},
		{name: "valid names the skill", text: freshSkill, want: "valid: fresh-skill"},
		{name: "invalid shows the first problem", text: "no frontmatter here", want: "invalid: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := press(t, loaded(t, &fakeBackend{}, dialogAgents), "n")
			m.area.SetValue(tt.text)
			text, _ := m.newCheck()
			if !strings.HasPrefix(text, tt.want) {
				t.Errorf("check = %q, want prefix %q", text, tt.want)
			}
		})
	}
}

func TestNewSaveShowsErrorAndKeepsEditor(t *testing.T) {
	b := &fakeBackend{}
	m := press(t, loaded(t, b, dialogAgents), "n")
	m = typeText(t, m, "not a skill")
	m = press(t, m, "ctrl+s")
	if m.mode != modeNew || !m.lastErr() || !strings.Contains(m.lastMsg(), "invalid skill") {
		t.Errorf("mode %v, result %q (err %t); want the editor open with the error", m.mode, m.lastMsg(), m.lastErr())
	}
}

func TestNewSaveCreatesSkillOnFixtureCopy(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	m, vaultDir, _ := fixtureModel(t)
	m = sized(t, m, 100, 30)
	m = typeText(t, press(t, m, "n"), freshSkill)
	if text, _ := m.newCheck(); text != "valid: fresh-skill" {
		t.Fatalf("live check = %q, want valid", text)
	}
	m = press(t, m, "ctrl+s")
	if m.mode != modeNormal || m.lastErr() || m.lastMsg() != "create fresh-skill" {
		t.Fatalf("mode %v, result %q (err %t); want the grid and create fresh-skill", m.mode, m.lastMsg(), m.lastErr())
	}
	if _, err := os.Stat(filepath.Join(vaultDir, "skills", "fresh-skill", "SKILL.md")); err != nil {
		t.Errorf("vault lacks the new skill: %v", err)
	}
	if !slices.Contains(m.skills, "fresh-skill") {
		t.Errorf("grid does not list the new skill: %q", m.skills)
	}
}

func TestNewSaveOfExistingNameNeedsForce(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	m, vaultDir, _ := fixtureModel(t)
	m = press(t, m, "n")
	m = typeText(t, m, strings.ReplaceAll(freshSkill, "fresh-skill", "mocked-skill-00004"))
	m = press(t, m, "ctrl+s")
	if m.mode != modeConfirm || !m.pending.act.force {
		t.Fatalf("mode %v, force %t; want the force prompt", m.mode, m.pending.act.force)
	}
	m = press(t, m, "n")
	if m.mode != modeNew || m.area.Value() == "" {
		t.Errorf("after n: mode %v; want the editor with its text kept", m.mode)
	}
	m = press(t, m, "ctrl+s", "y")
	if m.mode != modeNormal || m.lastErr() || m.lastMsg() != "create mocked-skill-00004" {
		t.Errorf("after y: mode %v, result %q (err %t)", m.mode, m.lastMsg(), m.lastErr())
	}
	if _, err := os.Stat(filepath.Join(vaultDir, "skills", "mocked-skill-00004", "SKILL.md")); err != nil {
		t.Errorf("vault lacks the replaced skill: %v", err)
	}
}

func TestImportDialog(t *testing.T) {
	b := &fakeBackend{}
	m := press(t, loaded(t, b, dialogAgents), "I", "enter")
	if m.mode != modeImport || !m.lastErr() || m.lastMsg() != "import needs a path" {
		t.Fatalf("empty path: mode %v, result %q (err %t)", m.mode, m.lastMsg(), m.lastErr())
	}
	m = press(t, typeText(t, m, "/src/alpha"), "enter")
	want := []string{"import /src/alpha dry=true force=false", "import /src/alpha dry=false force=false", "status"}
	if !slices.Equal(b.calls, want) {
		t.Errorf("calls = %q, want %q", b.calls, want)
	}
	if m.mode != modeNormal || m.lastErr() || m.lastMsg() != "import alpha" {
		t.Errorf("mode %v, result %q (err %t); want the grid and import alpha", m.mode, m.lastMsg(), m.lastErr())
	}
}

func TestImportDialogShowsErrorAndForceFlow(t *testing.T) {
	b := &fakeBackend{opErr: map[string]error{ops.OpImport: errors.New("boom")}}
	m := press(t, typeText(t, press(t, loaded(t, b, dialogAgents), "I"), "/src/alpha"), "enter")
	if m.mode != modeImport || !m.lastErr() || m.lastMsg() != "boom" {
		t.Errorf("error: mode %v, result %q (err %t); want the prompt open with boom", m.mode, m.lastMsg(), m.lastErr())
	}

	b = &fakeBackend{inVault: map[string]bool{"alpha": true}}
	m = press(t, typeText(t, press(t, loaded(t, b, dialogAgents), "I"), "/src/alpha"), "enter")
	if m.mode != modeConfirm || !m.pending.act.force {
		t.Fatalf("mode %v, force %t; want the force prompt", m.mode, m.pending.act.force)
	}
	m = press(t, m, "n")
	if last := m.lastNote(); m.mode != modeImport || last.title != "cancelled" || last.message != "import /src/alpha" {
		t.Errorf("after n: mode %v, note %q %q", m.mode, last.title, last.message)
	}
	m = press(t, m, "enter", "y")
	if m.mode != modeNormal || m.lastMsg() != "import alpha" {
		t.Errorf("after y: mode %v, result %q", m.mode, m.lastMsg())
	}
}

func TestExportAndImportRoundTripOnFixture(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	m, vaultDir, _ := fixtureModel(t)
	m = sized(t, m, 120, 40)
	const skill = "mocked-skill-00004"
	archive := filepath.Join(t.TempDir(), "backup.tar.gz")

	m = onSkill(t, m, skill, "claude")
	m = press(t, m, "x")
	if m.mode != modeExport || !strings.HasPrefix(m.path.Value(), skill+"-") {
		t.Fatalf("mode %v, path %q; want the EXPORT prompt on the default name", m.mode, m.path.Value())
	}
	m.path.SetValue(archive)
	m = press(t, m, "enter")
	if m.mode != modeNormal || m.lastErr() || m.lastMsg() != "wrote "+archive {
		t.Fatalf("export: mode %v, result %q (err %t)", m.mode, m.lastMsg(), m.lastErr())
	}
	if _, err := os.Stat(archive); err != nil {
		t.Fatalf("export wrote no file: %v", err)
	}

	m = press(t, m, "x")
	m.path.SetValue(archive)
	m = press(t, m, "enter")
	if m.mode != modeExport || !m.lastErr() {
		t.Errorf("export over a file: mode %v, result %q (err %t); want the prompt open with an error", m.mode, m.lastMsg(), m.lastErr())
	}

	m = press(t, m, "esc", "D", "y")
	if _, err := os.Stat(filepath.Join(vaultDir, "skills", skill)); !os.IsNotExist(err) {
		t.Fatalf("skill still in the vault after remove: %v", err)
	}
	if m.screen != screenMain {
		t.Errorf("screen after removing its skill = %v, want the grid", m.screen)
	}

	m = press(t, m, "I")
	m.path.SetValue(archive)
	m = press(t, m, "enter")
	if m.mode != modeNormal || m.lastErr() || m.lastMsg() != "import "+skill {
		t.Fatalf("import: mode %v, result %q (err %t)", m.mode, m.lastMsg(), m.lastErr())
	}
	if _, err := os.Stat(filepath.Join(vaultDir, "skills", skill, "SKILL.md")); err != nil {
		t.Errorf("vault lacks the imported skill: %v", err)
	}

	m = press(t, m, "I")
	m.path.SetValue(archive)
	m = press(t, m, "enter")
	if m.mode != modeConfirm || !m.pending.act.force {
		t.Errorf("import over the skill: mode %v, force %t; want the force prompt", m.mode, m.pending.act.force)
	}
}

func TestRemoveForceFlowOnSkillScreen(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced)}}
	m := press(t, loaded(t, b, []config.Agent{claude}), "enter", "D")
	if m.mode != modeConfirm || !m.pending.act.force {
		t.Fatalf("mode %v, force %t; want the force prompt", m.mode, m.pending.act.force)
	}
	if view := m.View(); !strings.Contains(view, "╭─ Force remove ") || !strings.Contains(view, "Remove alpha?") || !strings.Contains(view, "Deletes the vault copy") {
		t.Errorf("force prompt lacks the question or its effect:\n%s", view)
	}
	m = press(t, m, "n")
	if last := m.lastNote(); m.mode != modeNormal || m.screen != screenSkill || last.title != "cancelled" || last.message != "remove alpha" {
		t.Errorf("after n: mode %v, screen %v, note %q %q", m.mode, m.screen, last.title, last.message)
	}
	m = press(t, m, "D", "y")
	if !slices.Contains(b.calls, "remove alpha dry=false force=true") || m.lastMsg() != "remove alpha" {
		t.Errorf("calls %q, result %q; want the forced remove", b.calls, m.lastMsg())
	}
}

func TestRemovePlainConfirm(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.NotInstalled)}}
	m := press(t, loaded(t, b, []config.Agent{claude}), "enter", "D")
	if m.mode != modeConfirm || m.pending.act.force {
		t.Fatalf("mode %v, force %t; want the plain confirm", m.mode, m.pending.act.force)
	}
	if view := m.View(); !strings.Contains(view, "Remove alpha?") || strings.Contains(view, "Force") {
		t.Errorf("plain confirm lacks its question or names a force:\n%s", view)
	}
	m = press(t, m, "y")
	if m.lastMsg() != "remove alpha" || m.lastErr() {
		t.Errorf("result %q (err %t), want remove alpha", m.lastMsg(), m.lastErr())
	}
}

func TestThemePreviewAndRestore(t *testing.T) {
	th, set := testLook(t)
	all := theme.All()
	next := all[slices.IndexFunc(all, func(x theme.Theme) bool { return x.Name == th.Name })+1]
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced)}}
	m := testModel(b, []config.Agent{claude}, th, set, false)
	m, _ = settle(t, m, m.Init())

	m = press(t, m, "t", "down")
	if m.mode != modeTheme || m.th.Name != next.Name {
		t.Fatalf("mode %v, theme %q; want THEME previewing %q", m.mode, m.th.Name, next.Name)
	}
	if got := m.styles.accent.GetForeground(); got != next.Accent {
		t.Errorf("accent while previewing = %v, want %v", got, next.Accent)
	}

	m = press(t, m, "esc")
	if m.mode != modeNormal || m.th.Name != th.Name {
		t.Errorf("after esc: mode %v, theme %q; want %q restored", m.mode, m.th.Name, th.Name)
	}
	if got := m.styles.accent.GetForeground(); got != th.Accent {
		t.Errorf("accent after esc = %v, want %v", got, th.Accent)
	}

	m = press(t, m, "t", "down", "enter")
	if last := m.lastNote(); m.mode != modeNormal || m.th.Name != next.Name || last.kind != noteInfo || last.title != "theme" || last.message != next.Name {
		t.Errorf("after enter: mode %v, theme %q, note %+v; want %q kept as info", m.mode, m.th.Name, last, next.Name)
	}
}

func TestDialogsFitTheWidth(t *testing.T) {
	long := strings.Repeat("a-very-long-path-segment/", 12)
	longSkill := strings.Repeat("a-long-skill-name-", 6)
	tests := []struct {
		name  string
		rows  []ops.Row // the grid rows; alpha and beta when nil
		keys  []string
		want  mode
		setup func(m Model) Model
	}{
		{name: "ADOPT", keys: []string{"a"}, want: modeAdopt},
		{name: "NEW", keys: []string{"n"}, want: modeNew, setup: func(m Model) Model { return typeText(t, m, strings.Repeat("x", 200)) }},
		{name: "IMPORT", keys: []string{"I"}, want: modeImport, setup: func(m Model) Model { m.path.SetValue(long); return m }},
		{name: "THEME", keys: []string{"t"}, want: modeTheme},
		{name: "EXPORT", keys: []string{"enter", "x"}, want: modeExport, setup: func(m Model) Model { m.path.SetValue(long); return m }},
		{name: "discard", keys: []string{"n", "x", "esc"}, want: modeConfirm},
		{name: "HELP", keys: []string{"?"}, want: modeHelp},
		{name: "skill HELP", keys: []string{"enter", "?"}, want: modeHelp},
		{name: "plain confirm", rows: []ops.Row{row(longSkill, "claude", ops.NotInstalled)}, keys: []string{"enter", "D"}, want: modeConfirm},
		{name: "force confirm", rows: []ops.Row{row(longSkill, "claude", ops.Synced)}, keys: []string{"enter", "D"}, want: modeConfirm},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := tt.rows
			if rows == nil {
				rows = []ops.Row{row("alpha", "claude", ops.Unmanaged), row("beta", "claude", ops.Synced)}
			}
			m := sized(t, loaded(t, &fakeBackend{rows: rows}, []config.Agent{claude}), 80, 24)
			m = press(t, m, tt.keys...)
			if m.mode != tt.want {
				t.Fatalf("mode = %v, want %v; result %q", m.mode, tt.want, m.lastMsg())
			}
			if tt.setup != nil {
				m = tt.setup(m)
			}
			// The body is checked before fit cuts it, so an overflow does not hide.
			assertFits(t, 80, m.body())
			lines := strings.Split(m.View(), "\n")
			if len(lines) != 24 {
				t.Errorf("view has %d rows, want 24", len(lines))
			}
			assertFits(t, 80, lines)
		})
	}
}

// TestDialogBlocksAreCentered checks that each dialog is one block, centered, and
// at least dialogMinW wide or the terminal when narrower. An input dialog is exactly
// that wide.
func TestDialogBlocksAreCentered(t *testing.T) {
	longPath := strings.Repeat("a-very-long-path-segment/", 12)
	defaultRows := []ops.Row{row("alpha", "claude", ops.Unmanaged), row("beta", "claude", ops.Synced)}
	tests := []struct {
		name  string
		rows  []ops.Row
		keys  []string
		want  mode
		input bool // the input sets the block width
		// setup runs after the keys, for a value that fills the block.
		setup func(m Model) Model
	}{
		{name: "ADOPT", keys: []string{"a"}, want: modeAdopt},
		{name: "NEW", keys: []string{"n"}, want: modeNew, input: true, setup: func(m Model) Model { return typeText(t, m, strings.Repeat("x", 200)) }},
		{name: "IMPORT", keys: []string{"I"}, want: modeImport, input: true, setup: func(m Model) Model { m.path.SetValue(longPath); return m }},
		{name: "EXPORT", keys: []string{"enter", "x"}, want: modeExport, input: true, setup: func(m Model) Model { m.path.SetValue(longPath); return m }},
		{name: "THEME", keys: []string{"t"}, want: modeTheme},
		{name: "HELP main", keys: []string{"?"}, want: modeHelp},
		{name: "HELP skill", keys: []string{"enter", "?"}, want: modeHelp},
	}
	for _, size := range [][2]int{{120, 40}, {50, 24}} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("%s at %dx%d", tt.name, size[0], size[1]), func(t *testing.T) {
				rows := tt.rows
				if rows == nil {
					rows = defaultRows
				}
				m := press(t, sized(t, loaded(t, &fakeBackend{rows: rows}, dialogAgents), size[0], size[1]), tt.keys...)
				if m.mode != tt.want {
					t.Fatalf("mode = %v, want %v; result %q", m.mode, tt.want, m.lastMsg())
				}
				if tt.setup != nil {
					m = tt.setup(m)
				}
				left, right := bodyMargins(t, m)
				if left-right > 1 || right-left > 1 {
					t.Errorf("left margin %d, right %d; want equal within 1:\n%s", left, right, m.View())
				}
				block := size[0] - left - right
				least := min(size[0], dialogMinW)
				if block < least {
					t.Errorf("block is %d wide, want at least %d", block, least)
				}
				if want := m.inputWidth(); tt.input && block != want {
					t.Errorf("input block is %d wide, want %d", block, want)
				}
			})
		}
	}
}

// TestConfirmBoxIsCentered checks the confirm box over the screen it came from: the
// box is centered in a column and a row, as wide as its text within the bounds, its
// border is Accent for a plain confirm and Red for a force, the screen still shows
// outside it, and the text it must hold is there, wrapped if it runs long.
func TestConfirmBoxIsCentered(t *testing.T) {
	longSkill := strings.Repeat("a-long-skill-name-", 6)
	tests := []struct {
		name  string
		rows  []ops.Row
		keys  []string
		start func(t *testing.T, w, h int) Model // builds the screen, for a confirm that keys do not reach
		title string                             // the title of the box's border
		force bool
		under string   // text the screen shows outside the box
		text  []string // text the box holds, each piece on one line whatever the width
	}{
		{name: "plain uninstall", rows: []ops.Row{row(longSkill, "claude", ops.Synced)}, keys: []string{"enter", "u"}, title: "Uninstall", under: "AGENTS"},
		{name: "plain remove", rows: []ops.Row{row(longSkill, "claude", ops.NotInstalled)}, keys: []string{"enter", "D"}, title: "Remove", under: "AGENTS"},
		{name: "force confirm", rows: []ops.Row{row(longSkill, "claude", ops.Synced)}, keys: []string{"enter", "D"}, title: "Force remove", force: true, under: "AGENTS"},
		{name: "discard confirm", keys: []string{"n", "x", "esc"}, title: "Discard", under: "invalid:"},
		{name: "config prompt", start: firstRun(false, true), title: "Create config", under: "SKILL", text: []string{"defaults for this run."}},
		{name: "vault prompt", start: firstRun(true, false), title: "Create vault", under: "SKILL", text: []string{"Creates sapato.toml", "exits."}},
	}
	for _, size := range [][2]int{{120, 40}, {60, 20}, {50, 24}} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("%s at %dx%d", tt.name, size[0], size[1]), func(t *testing.T) {
				var m Model
				if tt.start != nil {
					m = tt.start(t, size[0], size[1])
				} else {
					rows := tt.rows
					if rows == nil {
						rows = []ops.Row{row("alpha", "claude", ops.Unmanaged), row("beta", "claude", ops.Synced)}
					}
					m = press(t, sized(t, loaded(t, &fakeBackend{rows: rows}, dialogAgents), size[0], size[1]), tt.keys...)
				}
				if m.mode != modeConfirm || m.pending.act.force != tt.force {
					t.Fatalf("mode = %v, force = %t; want the confirm", m.mode, m.pending.act.force)
				}
				view := m.View()
				lines := strings.Split(view, "\n")
				plain := make([]string, len(lines))
				for i, l := range lines {
					plain[i] = sgr.ReplaceAllString(l, "")
				}
				// The box is the border titled with the action, and its rows run down to the
				// corner under the same column.
				top, left, right := -1, 0, 0
				for i, l := range plain {
					if idx := strings.Index(l, "╭─ "+tt.title+" "); idx >= 0 {
						top, left = i, lipgloss.Width(l[:idx])
						right = left + lipgloss.Width(l[idx:strings.Index(l[idx:], "╮")+idx]) + 1
						break
					}
				}
				bottom := -1
				for i := top + 1; top >= 0 && i < len(plain); i++ {
					if idx := strings.Index(plain[i], "╰"); idx >= 0 && lipgloss.Width(plain[i][:idx]) == left {
						bottom = i
						break
					}
				}
				if top < 0 || bottom < 0 {
					t.Fatalf("no confirm box titled %q in view:\n%s", tt.title, view)
				}
				boxW := right - left
				least := min(confirmMinW, size[0]-4)
				if boxW < least || boxW > size[0]-4 {
					t.Errorf("box is %d wide, want between %d and %d", boxW, least, size[0]-4)
				}
				if d := left - (size[0]-boxW)/2; d < -1 || d > 1 {
					t.Errorf("box starts at column %d, want it centered in %d columns", left, size[0])
				}
				bodyTop := m.headerHeight() + 1
				if d := top - bodyTop - (m.bodyHeight()-(bottom-top+1))/2; d < -1 || d > 1 {
					t.Errorf("box rows %d to %d, want them centered in the body", top, bottom)
				}
				if !strings.Contains(view, tt.under) {
					t.Errorf("the screen does not show %q outside the box:\n%s", tt.under, view)
				}
				for _, want := range tt.text {
					if !strings.Contains(view, want) {
						t.Errorf("confirm box lacks %q:\n%s", want, view)
					}
				}
				// The colors are checked on a colored set, since the test models draw without.
				m.styles = newStyles(m.th, false)
				want := m.th.Accent
				if tt.force {
					want = m.th.Red
				}
				if got := m.confirmBorder().GetForeground(); got != want {
					t.Errorf("border color = %v, want %v", got, want)
				}
			})
		}
	}
}

// firstRun returns the first-launch prompt for a missing config or vault, drawn at
// w by h over the main screen.
func firstRun(fileExists, vaultExists bool) func(t *testing.T, w, h int) Model {
	return func(t *testing.T, w, h int) Model {
		th, set := testLook(t)
		cfg := promptConfig(t, filepath.Join(t.TempDir(), "config.toml"), fileExists, vaultExists)
		return sized(t, promptModel(t, cfg, th, set), w, h)
	}
}

// TestConfirmModalDimsTheScreenOutsideTheBox checks that every row of the modal is
// either a row of the box or the screen's row with its styles stripped and drawn in
// Overlay.
func TestConfirmModalDimsTheScreenOutsideTheBox(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.NotInstalled)}}
	m := press(t, sized(t, loaded(t, b, []config.Agent{claude}), 80, 24), "enter", "D")
	under := m
	under.mode = m.pending.act.back
	screen := fitRows(under.body(), m.bodyHeight())
	box, _ := m.confirmBox()
	lines := m.confirmModal()
	if len(lines) != len(screen) {
		t.Fatalf("modal has %d rows, want the screen's %d", len(lines), len(screen))
	}
	for i, l := range lines {
		if slices.ContainsFunc(box, func(row string) bool { return strings.Contains(l, row) }) {
			continue
		}
		if want := m.styles.dim.Render(ansi.Strip(screen[i])); l != want {
			t.Errorf("row %d = %q, want the screen row dimmed %q", i, l, want)
		}
	}
}

// TestPathDialogsEscLeavesNoTrace checks that esc closes IMPORT and EXPORT without
// running anything or changing the bar.
func TestPathDialogsEscLeavesNoTrace(t *testing.T) {
	tests := []struct {
		name string
		open []string
		want mode
	}{
		{name: "IMPORT", open: []string{"I"}, want: modeImport},
		{name: "EXPORT", open: []string{"enter", "x"}, want: modeExport},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced)}}
			m := press(t, loaded(t, b, []config.Agent{claude}), tt.open...)
			if m.mode != tt.want {
				t.Fatalf("mode = %v, want %v", m.mode, tt.want)
			}
			m = typeText(t, m, "/src/alpha")
			result, calls := m.lastMsg(), len(b.calls)
			m = press(t, m, "esc")
			if m.mode != modeNormal || m.lastErr() || m.lastMsg() != result {
				t.Errorf("after esc: mode %v, result %q (err %t); want the grid with %q", m.mode, m.lastMsg(), m.lastErr(), result)
			}
			if len(b.calls) != calls {
				t.Errorf("esc made calls %q", b.calls[calls:])
			}
		})
	}
}

func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tests := []struct {
		name, in, want string
	}{
		{name: "bare tilde is home", in: "~", want: home},
		{name: "tilde slash is under home", in: "~/skills/alpha", want: filepath.Join(home, "skills/alpha")},
		{name: "absolute path is kept", in: "/srv/alpha", want: "/srv/alpha"},
		{name: "relative path is kept", in: "skills/alpha", want: "skills/alpha"},
		{name: "tilde user is kept", in: "~other/alpha", want: "~other/alpha"},
		{name: "tilde in the middle is kept", in: "a/~/alpha", want: "a/~/alpha"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expandHome(tt.in); got != tt.want {
				t.Errorf("expandHome(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
