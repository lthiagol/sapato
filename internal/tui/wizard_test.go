package tui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"sapato/internal/ops"
)

// wizardEditor stubs the editor of m: argv is recorded, and the editor "exits" at
// once. It returns the recorded argv, nil until the editor runs.
func wizardEditor(m *Model) *[]string {
	var argv []string
	m.editArgv = func(path string) []string { return []string{"ed", path} }
	m.editRun = func(a []string) tea.Cmd {
		argv = a
		return func() tea.Msg { return editDoneMsg{} }
	}
	return &argv
}

func TestWizardOpensWithEveryAgentChecked(t *testing.T) {
	m := press(t, loaded(t, &fakeBackend{}, dialogAgents), "n")
	if m.mode != modeNew || m.newPaste {
		t.Fatalf("mode %v, paste %t; want the wizard", m.mode, m.newPaste)
	}
	if m.wiz.focus != 0 || !m.wiz.name.Focused() {
		t.Errorf("focus %d, name focused %t; want the name", m.wiz.focus, m.wiz.name.Focused())
	}
	if want := []bool{true, true}; !slices.Equal(m.wiz.checked, want) {
		t.Errorf("checked = %v, want %v", m.wiz.checked, want)
	}
}

func TestWizardNameCheck(t *testing.T) {
	b := &fakeBackend{rows: []ops.Row{row("mocked-skill-00004", "claude", ops.Synced)}}
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "empty hints the rules", text: "", want: "lowercase letters, digits and dashes"},
		{name: "upper case breaks rule 5", text: "New_Skill", want: "invalid: lowercase"},
		{name: "longer than 64 characters", text: strings.Repeat("a", 65), want: "invalid: lowercase"},
		{name: "a name in the vault", text: "mocked-skill-00004", want: "in the vault: save asks to replace it"},
		{name: "a new valid name", text: "fresh-skill", want: "valid: fresh-skill"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := press(t, loaded(t, b, dialogAgents), "n")
			m.wiz.name.SetValue(tt.text)
			if text, _ := m.nameCheck(); !strings.HasPrefix(text, tt.want) {
				t.Errorf("check = %q, want prefix %q", text, tt.want)
			}
		})
	}
}

func TestWizardDescriptionCount(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "empty is required", text: "", want: "0/1024 · required"},
		{name: "counts characters", text: "A skill.", want: "8/1024"},
		{name: "over the limit", text: strings.Repeat("d", 1025), want: "1025/1024 · too long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := press(t, loaded(t, &fakeBackend{}, dialogAgents), "n")
			m.wiz.desc.SetValue(tt.text)
			if text, _ := m.descCheck(); text != tt.want {
				t.Errorf("count = %q, want %q", text, tt.want)
			}
		})
	}
}

func TestWizardFieldsMoveAndToggle(t *testing.T) {
	m := press(t, loaded(t, &fakeBackend{}, dialogAgents), "n")
	m = press(t, m, "j", "k")
	if m.wiz.name.Value() != "jk" || m.wiz.focus != 0 {
		t.Errorf("j and k typed into name %q at focus %d; want jk at 0", m.wiz.name.Value(), m.wiz.focus)
	}
	m = press(t, m, "down")
	if m.wiz.focus != 1 || !m.wiz.desc.Focused() {
		t.Errorf("down: focus %d; want the description", m.wiz.focus)
	}
	m = press(t, m, "enter", "enter", "space")
	if m.wiz.focus != 3 || m.wiz.checked[1] || !m.wiz.checked[0] {
		t.Errorf("enter, enter, space: focus %d, checked %v; want focus 3 with the second agent unchecked", m.wiz.focus, m.wiz.checked)
	}
	m = press(t, m, "down", "up", "up", "up", "up")
	if m.wiz.focus != 0 {
		t.Errorf("up past the top: focus %d, want 0", m.wiz.focus)
	}
	if view := m.View(); !strings.Contains(view, "1 of 2 agents") {
		t.Errorf("view lacks the agent count:\n%s", view)
	}
}

func TestWizardTabKeepsEachModesText(t *testing.T) {
	m := press(t, loaded(t, &fakeBackend{}, dialogAgents), "n", "tab")
	m = typeText(t, m, "pasted")
	m = press(t, m, "tab")
	if m.newPaste {
		t.Fatal("tab did not return to the wizard")
	}
	m = typeText(t, m, "w")
	m = press(t, m, "tab")
	if !m.newPaste || m.area.Value() != "pasted" {
		t.Errorf("paste %t, text %q; want the paste text kept", m.newPaste, m.area.Value())
	}
	if m = press(t, m, "tab"); m.wiz.name.Value() != "w" {
		t.Errorf("wizard name = %q, want w kept", m.wiz.name.Value())
	}
}

func TestWizardNeedsAnAgent(t *testing.T) {
	b := &fakeBackend{}
	m := press(t, typeText(t, press(t, loaded(t, b, dialogAgents), "n"), "alpha"), "down", "down", "space", "down", "space", "ctrl+s")
	if !m.lastErr() || m.lastMsg() != "pick at least one agent" {
		t.Errorf("result %q (err %t); want the pick-an-agent error", m.lastMsg(), m.lastErr())
	}
	if len(b.calls) != 0 {
		t.Errorf("calls = %q, want none", b.calls)
	}
	if view := m.View(); !strings.Contains(view, "pick at least one agent") {
		t.Errorf("view lacks the hint:\n%s", view)
	}
}

func TestWizardScaffoldCompatibleList(t *testing.T) {
	tests := []struct {
		name     string
		uncheck  bool
		wantCall string
	}{
		{name: "every agent checked writes no list", wantCall: "scaffold alpha compat=all dry=true force=false"},
		{name: "a subset writes the list", uncheck: true, wantCall: "scaffold alpha compat=claude dry=true force=false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &fakeBackend{}
			m := press(t, typeText(t, press(t, loaded(t, b, dialogAgents), "n"), "alpha"), "down", "down")
			if tt.uncheck {
				m = press(t, m, "down", "space")
			}
			m = press(t, m, "ctrl+s", "n")
			if len(b.calls) == 0 || b.calls[0] != tt.wantCall {
				t.Errorf("calls = %q, want first %q", b.calls, tt.wantCall)
			}
		})
	}
}

func TestWizardForceFlow(t *testing.T) {
	b := &fakeBackend{inVault: map[string]bool{"alpha": true}}
	m := press(t, typeText(t, press(t, loaded(t, b, dialogAgents), "n"), "alpha"), "ctrl+s")
	if m.mode != modeConfirm || !m.pending.act.force || m.pending.act.op != opScaffold {
		t.Fatalf("mode %v, force %t, op %q; want the scaffold force prompt", m.mode, m.pending.act.force, m.pending.act.op)
	}
	view := sized(t, m, 100, 30).View()
	for _, want := range []string{"Force create", "Create alpha?", "Overwrites the same-named skill and drops its compatible list."} {
		if !strings.Contains(view, want) {
			t.Errorf("force prompt lacks %q:\n%s", want, view)
		}
	}
	m = press(t, m, "n")
	if m.mode != modeNew || m.wiz.name.Value() != "alpha" {
		t.Errorf("after n: mode %v, name %q; want the wizard with its name", m.mode, m.wiz.name.Value())
	}
	m = press(t, m, "ctrl+s", "y")
	if !slices.Contains(b.calls, "scaffold alpha compat=all dry=false force=true") {
		t.Errorf("calls = %q, want the forced scaffold", b.calls)
	}
	if m.mode != modeConfirm || m.pending.act.op != opEdit {
		t.Errorf("after the forced scaffold: mode %v, op %q; want the editor prompt", m.mode, m.pending.act.op)
	}
	if len(m.notes) == 0 || m.notes[len(m.notes)-1].title != "created" {
		t.Errorf("notes = %+v, want a created notification", m.notes)
	}
}

func TestWizardSaveOffersTheEditor(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	m, vaultDir, _ := fixtureModel(t)
	m = sized(t, m, 100, 30)
	argv := wizardEditor(&m)
	m = press(t, m, "n")
	m = typeText(t, m, "fresh-wizard")
	m = press(t, m, "down")
	m = typeText(t, m, "Made in the wizard.")
	m = press(t, m, "ctrl+s")

	path := filepath.Join(vaultDir, "skills", "fresh-wizard", "SKILL.md")
	if m.mode != modeConfirm || m.pending.act.op != opEdit || m.pending.act.path != path {
		t.Fatalf("mode %v, op %q, path %q; want the editor prompt for %s", m.mode, m.pending.act.op, m.pending.act.path, path)
	}
	if got := m.editorName(path); got != "ed" {
		t.Errorf("editor name = %q, want ed", got)
	}
	if view := m.View(); !strings.Contains(view, "Open in editor") {
		t.Errorf("prompt lacks its title:\n%s", view)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "---\nname: fresh-wizard\ndescription: Made in the wizard.\n---\n") {
		t.Errorf("SKILL.md starts %q", data)
	}

	m = press(t, m, "y")
	if want := []string{"ed", path}; !slices.Equal(*argv, want) {
		t.Errorf("editor argv = %q, want %q", *argv, want)
	}
	if m.mode != modeNormal || !slices.Contains(m.skills, "fresh-wizard") {
		t.Errorf("after y: mode %v, grid lists fresh-wizard %t; want the grid with it", m.mode, slices.Contains(m.skills, "fresh-wizard"))
	}
}

func TestWizardEditorPromptNo(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	m, vaultDir, _ := fixtureModel(t)
	argv := wizardEditor(&m)
	m = press(t, typeText(t, press(t, m, "n"), "fresh-wizard"), "down")
	m = press(t, typeText(t, m, "Declined."), "ctrl+s")
	if m.mode != modeConfirm || m.pending.act.op != opEdit {
		t.Fatalf("mode %v, op %q; want the editor prompt", m.mode, m.pending.act.op)
	}
	m = press(t, m, "n")
	if *argv != nil {
		t.Errorf("editor ran after n: %q", *argv)
	}
	path := filepath.Join(vaultDir, "skills", "fresh-wizard", "SKILL.md")
	if last := m.lastNote(); m.mode != modeNormal || last.title != "cancelled" || last.message != "open "+path {
		t.Errorf("after n: mode %v, note %q %q; want the grid and a cancelled open", m.mode, last.title, last.message)
	}
}

func TestWizardEscAsksWhenTyped(t *testing.T) {
	m := press(t, loaded(t, &fakeBackend{}, dialogAgents), "n")
	if got := press(t, m, "esc"); got.mode != modeNormal {
		t.Errorf("esc on an empty wizard: mode %v, want the grid", got.mode)
	}
	m = press(t, m, "x", "esc")
	if m.mode != modeConfirm || m.pending.act.op != opDiscard {
		t.Fatalf("esc on text: mode %v, op %q; want the discard confirm", m.mode, m.pending.act.op)
	}
	if m = press(t, m, "n"); m.mode != modeNew || m.wiz.name.Value() != "x" {
		t.Errorf("after n: mode %v, name %q; want the wizard with its text", m.mode, m.wiz.name.Value())
	}
	if m = press(t, m, "esc", "y"); m.mode != modeNormal || m.lastMsg() != "new skill discarded" {
		t.Errorf("after y: mode %v, result %q", m.mode, m.lastMsg())
	}
}
