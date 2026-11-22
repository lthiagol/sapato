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

	"sapato/internal/config"
	"sapato/internal/ops"
)

// writeSkillFolder writes a valid skill called name into dir/name and returns that
// folder.
func writeSkillFolder(t *testing.T, dir, name string) string {
	t.Helper()
	folder := filepath.Join(dir, name)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: A " + name + " skill.\n---\n\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(folder, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return folder
}

// realImport returns the UI over a copy of the fixture vault, sized to w×h, with
// the home dir set to a temp dir. It also returns the vault folder.
func realImport(t *testing.T, w, h int) (Model, string) {
	t.Helper()
	t.Setenv("TMPDIR", t.TempDir())
	m, vaultDir, _ := fixtureModel(t)
	m = sized(t, m, w, h)
	m.home = t.TempDir()
	return m, vaultDir
}

// inVaultDir reports whether the vault folder holds a skill called name.
func inVaultDir(vaultDir, name string) bool {
	_, err := os.Stat(filepath.Join(vaultDir, "skills", name, "SKILL.md"))
	return err == nil
}

// typeSource opens IMPORT, types src into it and presses enter.
func typeSource(t *testing.T, m Model, src string) Model {
	t.Helper()
	return press(t, typeText(t, press(t, m, "I"), src), "enter")
}

// onEntry moves the picker cursor onto the entry called name.
func onEntry(t *testing.T, m Model, name string) Model {
	t.Helper()
	i := slices.IndexFunc(m.browse.entries, func(e browseEntry) bool { return e.name == name })
	if i < 0 {
		t.Fatalf("picker has no entry %s; entries %+v", name, m.browse.entries)
	}
	m.browse.cur = i
	return m
}

// entryNames lists the names of the picker entries, in order.
func entryNames(m Model) []string {
	var names []string
	for _, e := range m.browse.entries {
		names = append(names, e.name)
	}
	return names
}

func TestImportSingleSkillFromFolder(t *testing.T) {
	m, vaultDir := realImport(t, 120, 40)
	src := writeSkillFolder(t, t.TempDir(), "fresh-skill")
	m = typeSource(t, m, src)
	if m.mode != modeNormal || m.lastErr() || m.lastMsg() != "import fresh-skill" {
		t.Fatalf("mode %v, result %q (err %t); want the grid and import fresh-skill", m.mode, m.lastMsg(), m.lastErr())
	}
	if !inVaultDir(vaultDir, "fresh-skill") {
		t.Error("vault lacks fresh-skill")
	}
}

func TestImportListImportsMarkedOnly(t *testing.T) {
	pack := t.TempDir()
	for _, name := range []string{"alpha", "beta", "gamma"} {
		writeSkillFolder(t, pack, name)
	}
	tests := []struct {
		name string
		keys []string
		want []string // the skills the import leaves in the vault
	}{
		{name: "marked skills are imported", keys: []string{"space", "down", "down", "space", "enter"}, want: []string{"alpha", "gamma"}},
		{name: "mark all imports every skill", keys: []string{"a", "enter"}, want: []string{"alpha", "beta", "gamma"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, vaultDir := realImport(t, 120, 40)
			m = typeSource(t, m, pack)
			if m.mode != modeImportList || len(m.multi.cands) != 3 {
				t.Fatalf("mode %v, %d candidates; want the list of 3", m.mode, len(m.multi.cands))
			}
			m = press(t, m, tt.keys...)
			if m.mode != modeNormal || m.lastErr() {
				t.Fatalf("mode %v, result %q (err %t)", m.mode, m.lastMsg(), m.lastErr())
			}
			for _, name := range []string{"alpha", "beta", "gamma"} {
				if got := inVaultDir(vaultDir, name); got != slices.Contains(tt.want, name) {
					t.Errorf("%s in vault = %t, want %t", name, got, !got)
				}
			}
		})
	}
}

// writeInvalidSkillFolder writes a skill called name without a description into
// dir/name, so it is invalid and --fix can repair it.
func writeInvalidSkillFolder(t *testing.T, dir, name string) {
	t.Helper()
	folder := filepath.Join(dir, name)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "SKILL.md"), []byte("---\nname: "+name+"\n---\n\n# "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestImportListInvalidSkillsCannotBeMarked(t *testing.T) {
	pack := t.TempDir()
	writeSkillFolder(t, pack, "alpha")
	writeInvalidSkillFolder(t, pack, "beta")
	tests := []struct {
		name         string
		keys         []string
		wantMode     mode
		wantErr      bool
		wantMsg      string // a substring of the newest notification
		wantImported []string
	}{
		{name: "space on an invalid skill marks nothing and says why", keys: []string{"down", "space"}, wantMode: modeImportList, wantErr: true, wantMsg: "beta is invalid: "},
		{name: "mark all marks the valid skills only", keys: []string{"a", "enter"}, wantMode: modeNormal, wantImported: []string{"alpha"}},
		{name: "enter with only the invalid one marked is refused", keys: []string{"down", "space", "enter"}, wantMode: modeImportList, wantErr: true, wantMsg: "no skill marked"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, vaultDir := realImport(t, 120, 40)
			m = press(t, typeSource(t, m, pack), tt.keys...)
			if m.mode != tt.wantMode || m.lastErr() != tt.wantErr || !strings.Contains(m.lastMsg(), tt.wantMsg) {
				t.Errorf("mode %v, result %q (err %t); want mode %v, error %t, result containing %q", m.mode, m.lastMsg(), m.lastErr(), tt.wantMode, tt.wantErr, tt.wantMsg)
			}
			if m.multi.marks["beta"] {
				t.Error("invalid skill beta is marked")
			}
			for _, name := range []string{"alpha", "beta"} {
				if got := inVaultDir(vaultDir, name); got != slices.Contains(tt.wantImported, name) {
					t.Errorf("%s in vault = %t, want %t", name, got, !got)
				}
			}
		})
	}
}

// TestImportListStatusStyles checks that a valid candidate's status is its name in the
// text style, and an invalid one's is its first problem in the dim style, not the
// fail style.
func TestImportListStatusStyles(t *testing.T) {
	th, set := testLook(t)
	m := testModel(&fakeBackend{}, nil, th, set, false)
	tests := []struct {
		name string
		cand ops.Candidate
		text string
		fg   lipgloss.TerminalColor
	}{
		{name: "valid shows its name", cand: ops.Candidate{Rel: "a", Name: "alpha", Valid: true}, text: "alpha", fg: m.styles.text.GetForeground()},
		{name: "invalid shows its first problem, dimmed", cand: ops.Candidate{Rel: "b", Problems: []string{"description is missing", "more"}}, text: "invalid: description is missing", fg: m.styles.dim.GetForeground()},
		{name: "invalid without problems says invalid, dimmed", cand: ops.Candidate{Rel: "c"}, text: "invalid", fg: m.styles.dim.GetForeground()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, style := m.candStatus(tt.cand)
			if text != tt.text || style.GetForeground() != tt.fg {
				t.Errorf("status = %q in %v; want %q in %v", text, style.GetForeground(), tt.text, tt.fg)
			}
		})
	}
	if m.styles.dim.GetForeground() == m.styles.fail.GetForeground() {
		t.Fatal("the dim and fail styles share a color, so the test cannot tell them apart")
	}
}

func TestImportListNothingMarkedIsRefused(t *testing.T) {
	pack := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		writeSkillFolder(t, pack, name)
	}
	m, vaultDir := realImport(t, 120, 40)
	m = press(t, typeSource(t, m, pack), "enter")
	if m.mode != modeImportList || !m.lastErr() || !strings.Contains(m.lastMsg(), "no skill marked") {
		t.Fatalf("mode %v, result %q (err %t); want the list with a hint to mark", m.mode, m.lastMsg(), m.lastErr())
	}
	if inVaultDir(vaultDir, "alpha") || inVaultDir(vaultDir, "beta") {
		t.Error("refused import changed the vault")
	}
	if m = press(t, m, "esc"); m.mode != modeImport || m.path.Value() != pack {
		t.Errorf("esc: mode %v, source %q; want the source input with %q", m.mode, m.path.Value(), pack)
	}
}

func TestImportListForceConfirmKeepsMarks(t *testing.T) {
	pack := t.TempDir()
	writeSkillFolder(t, pack, "mocked-skill-00001")
	writeSkillFolder(t, pack, "beta")
	m, vaultDir := realImport(t, 120, 40)
	m = press(t, typeSource(t, m, pack), "a", "enter")
	if m.mode != modeConfirm || !m.pending.act.force {
		t.Fatalf("mode %v, force %t; want the force prompt", m.mode, m.pending.act.force)
	}
	if m = press(t, m, "n"); m.mode != modeImportList || !m.multi.marks["beta"] || !m.multi.marks["mocked-skill-00001"] {
		t.Errorf("after n: mode %v, marks %v; want the list with its marks kept", m.mode, m.multi.marks)
	}
	if m = press(t, m, "enter", "y"); m.mode != modeNormal || m.lastErr() {
		t.Errorf("after y: mode %v, result %q (err %t)", m.mode, m.lastMsg(), m.lastErr())
	}
	if !inVaultDir(vaultDir, "beta") {
		t.Error("vault lacks beta after the forced import")
	}
}

func TestImportExistingNameAsksForce(t *testing.T) {
	m, _ := realImport(t, 120, 40)
	m = typeSource(t, m, writeSkillFolder(t, t.TempDir(), "mocked-skill-00001"))
	if m.mode != modeConfirm || !m.pending.act.force {
		t.Fatalf("mode %v, force %t; want the force prompt", m.mode, m.pending.act.force)
	}
	if m = press(t, m, "n"); m.mode != modeImport {
		t.Errorf("after n: mode %v, want the source input", m.mode)
	}
}

func TestImportFixUpForm(t *testing.T) {
	const draft = "---\nname: draft-note\n---\n\n# Draft\n"
	setup := func(t *testing.T) (Model, string, string) {
		t.Helper()
		folder := filepath.Join(t.TempDir(), "draft-note")
		if err := os.MkdirAll(folder, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, "SKILL.md"), []byte(draft), 0o644); err != nil {
			t.Fatal(err)
		}
		m, vaultDir := realImport(t, 120, 40)
		return typeSource(t, m, folder), folder, vaultDir
	}

	t.Run("edited name and description are imported", func(t *testing.T) {
		m, folder, vaultDir := setup(t)
		if m.mode != modeImportFix {
			t.Fatalf("mode %v, result %q (err %t); want the fix-up form", m.mode, m.lastMsg(), m.lastErr())
		}
		if got := m.fixup.name.Value(); got != "draft-note" {
			t.Errorf("name prefilled %q, want draft-note", got)
		}
		if !slices.ContainsFunc(m.fixup.cand.Problems, func(p string) bool { return strings.Contains(p, "description") }) {
			t.Errorf("problems %v lack the missing description", m.fixup.cand.Problems)
		}
		m.fixup.name.SetValue("renamed-draft")
		m = typeText(t, press(t, m, "tab"), "A drafted note.")
		m = press(t, m, "ctrl+s")
		if m.mode != modeNormal || m.lastErr() || m.lastMsg() != "import renamed-draft" {
			t.Fatalf("mode %v, result %q (err %t)", m.mode, m.lastMsg(), m.lastErr())
		}
		body, err := os.ReadFile(filepath.Join(vaultDir, "skills", "renamed-draft", "SKILL.md"))
		if err != nil || !strings.Contains(string(body), "description: A drafted note.") {
			t.Errorf("vault copy = %q, %v; want the typed description", body, err)
		}
		if src, _ := os.ReadFile(filepath.Join(folder, "SKILL.md")); string(src) != draft {
			t.Error("the source was changed by the fix")
		}
	})

	t.Run("missing description is refused and the form stays", func(t *testing.T) {
		m, _, vaultDir := setup(t)
		m = press(t, m, "ctrl+s")
		if m.mode != modeImportFix || !m.lastErr() || !strings.Contains(m.lastMsg(), "description is missing") {
			t.Errorf("mode %v, result %q (err %t); want the form with a description hint", m.mode, m.lastMsg(), m.lastErr())
		}
		if inVaultDir(vaultDir, "draft-note") {
			t.Error("refused import changed the vault")
		}
	})

	t.Run("name in the vault asks for force and n returns to the form", func(t *testing.T) {
		m, _, _ := setup(t)
		m.fixup.name.SetValue("mocked-skill-00001")
		m = press(t, typeText(t, press(t, m, "tab"), "A note."), "shift+tab", "ctrl+s")
		if m.mode != modeConfirm || !m.pending.act.force {
			t.Fatalf("mode %v, force %t; want the force prompt", m.mode, m.pending.act.force)
		}
		if m = press(t, m, "n"); m.mode != modeImportFix || m.fixup.name.Value() != "mocked-skill-00001" {
			t.Errorf("after n: mode %v, name %q; want the form with its name", m.mode, m.fixup.name.Value())
		}
	})
}

func TestBrowseNavigation(t *testing.T) {
	root := t.TempDir()
	writeSkillFolder(t, root, "alpha")
	for _, f := range []string{"sub/beta.md", ".hidden/x.md", ".secret.md", "notes.txt", "pack.tar.gz"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m, _ := realImport(t, 120, 40)
	m = press(t, typeText(t, press(t, m, "I"), root), "ctrl+o")
	if m.mode != modeImportBrowse || m.browse.dir != root {
		t.Fatalf("mode %v, folder %q; want the picker on %s", m.mode, m.browse.dir, root)
	}
	if got, want := entryNames(m), []string{"alpha", "sub", "notes.txt", "pack.tar.gz"}; !slices.Equal(got, want) {
		t.Fatalf("entries = %v, want %v (folders first, dot entries hidden)", got, want)
	}

	m = press(t, onEntry(t, m, "notes.txt"), "space")
	if m.mode != modeImportBrowse || m.path.Value() != root {
		t.Errorf("space on a file IMPORT does not take: mode %v, source %q; want no change", m.mode, m.path.Value())
	}

	m = press(t, onEntry(t, m, "sub"), "enter")
	if m.browse.dir != filepath.Join(root, "sub") || !slices.Equal(entryNames(m), []string{"beta.md"}) {
		t.Fatalf("enter on sub: folder %q, entries %v", m.browse.dir, entryNames(m))
	}
	m = press(t, m, "backspace")
	if m.browse.dir != root || m.browse.entries[m.browse.cur].name != "sub" {
		t.Errorf("backspace: folder %q, cursor on %v; want the root with sub under the cursor", m.browse.dir, m.browse.entries[m.browse.cur].name)
	}

	m = press(t, m, ".")
	if got, want := entryNames(m), []string{".hidden", "alpha", "sub", ".secret.md", "notes.txt", "pack.tar.gz"}; !slices.Equal(got, want) {
		t.Errorf("after . : entries = %v, want %v", got, want)
	}
	m = press(t, m, ".")
	if slices.Contains(entryNames(m), ".hidden") {
		t.Error("second . did not hide the dot entries")
	}

	m = press(t, onEntry(t, m, "pack.tar.gz"), "space")
	if m.mode != modeImport || m.path.Value() != filepath.Join(root, "pack.tar.gz") {
		t.Errorf("space on a tarball: mode %v, source %q; want the tarball chosen", m.mode, m.path.Value())
	}

	m = press(t, m, "ctrl+o")
	m = press(t, onEntry(t, m, "sub"), "enter")
	m = press(t, onEntry(t, m, "beta.md"), "enter")
	if m.mode != modeImport || m.path.Value() != filepath.Join(root, "sub", "beta.md") {
		t.Errorf("enter on a .md file: mode %v, source %q; want it chosen", m.mode, m.path.Value())
	}

	m = press(t, m, "ctrl+o", "esc")
	if m.mode != modeImport || m.path.Value() != filepath.Join(root, "sub", "beta.md") {
		t.Errorf("esc: mode %v, source %q; want the input unchanged", m.mode, m.path.Value())
	}
}

func TestBrowseStartsWhereSpecSays(t *testing.T) {
	home := t.TempDir()
	folder := t.TempDir()
	file := filepath.Join(folder, "SKILL.md")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, input, want string
	}{
		{name: "empty input opens the home dir", input: "", want: home},
		{name: "an existing folder opens itself", input: folder, want: folder},
		{name: "a file opens its folder", input: file, want: folder},
		{name: "a missing path opens the home dir", input: filepath.Join(t.TempDir(), "missing", "x"), want: home},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := sized(t, loaded(t, &fakeBackend{}, dialogAgents), 120, 40)
			m.home = home
			m = press(t, m, "I")
			m.path.SetValue(tt.input)
			m = press(t, m, "ctrl+o")
			if m.mode != modeImportBrowse || m.browse.dir != tt.want {
				t.Errorf("mode %v, folder %q; want the picker on %q", m.mode, m.browse.dir, tt.want)
			}
		})
	}
}

func TestDiscoveryErrorIsShown(t *testing.T) {
	b := &fakeBackend{discoverErr: errors.New("no such folder")}
	m := press(t, typeText(t, press(t, loaded(t, b, dialogAgents), "I"), "/src/gone"), "enter")
	if m.mode != modeImport || !m.lastErr() || m.lastMsg() != "no such folder" {
		t.Errorf("mode %v, result %q (err %t); want the input with the error", m.mode, m.lastMsg(), m.lastErr())
	}
}

func TestStaleDiscoveryIsDropped(t *testing.T) {
	b := &fakeBackend{discovered: map[string][]ops.Candidate{
		"/src/pack": {{Rel: "a", Name: "a", Valid: true}, {Rel: "b", Name: "b", Valid: true}},
	}}
	m := typeText(t, press(t, loaded(t, b, dialogAgents), "I"), "/src/now")
	before := len(b.calls)
	m, _ = send(t, m, discoveredMsg{src: "/src/pack", cands: b.candidates("/src/pack")})
	if m.mode != modeImport || len(m.multi.cands) != 0 {
		t.Errorf("reading of another source: mode %v, %d candidates; want it dropped", m.mode, len(m.multi.cands))
	}
	m = press(t, m, "esc")
	m, _ = send(t, m, discoveredMsg{src: "/src/now", cands: b.candidates("/src/now")})
	if m.mode != modeNormal || len(b.calls) != before {
		t.Errorf("reading after esc: mode %v, calls %q; want it dropped", m.mode, b.calls[before:])
	}
}

func TestImportListFakeCallsPickMarkedOnly(t *testing.T) {
	cands := []ops.Candidate{
		{Rel: "a/alpha", Name: "alpha", Hint: "alpha", Valid: true},
		{Rel: "b/beta", Name: "beta", Hint: "beta", Valid: true},
	}
	b := &fakeBackend{discovered: map[string][]ops.Candidate{"/src/pack": cands}}
	m := press(t, typeText(t, press(t, loaded(t, b, dialogAgents), "I"), "/src/pack"), "enter")
	m = press(t, m, "down", "space", "enter")
	want := []string{"import /src/pack dry=true force=false pick=b/beta", "import /src/pack dry=false force=false pick=b/beta", "status"}
	if !slices.Equal(b.calls, want) {
		t.Errorf("calls = %q, want %q", b.calls, want)
	}
}

// TestImportFlowFitsTheTerminal draws each IMPORT step at 60 and 120 columns: nothing
// is wider than the terminal, and each step fills the terminal's rows.
func TestImportFlowFitsTheTerminal(t *testing.T) {
	long := strings.Repeat("a-very-long-folder-name-", 4)
	cands := []ops.Candidate{
		{Rel: long + "1", Name: long, Hint: long, Valid: true},
		{Rel: "b", Name: "", Problems: []string{"description is missing and " + strings.Repeat("very ", 12)}},
		{Rel: "c", Name: "gamma", Valid: true},
	}
	fixable := ops.Candidate{Rel: ".", Name: "draft", Hint: "draft", Problems: []string{strings.Repeat("problem ", 12)}, Fixable: true}
	steps := []struct {
		name  string
		setup func(t *testing.T, m Model) Model
		mode  mode
	}{
		{name: "source", setup: func(t *testing.T, m Model) Model { return typeText(t, press(t, m, "I"), long) }, mode: modeImport},
		{name: "list", setup: func(t *testing.T, m Model) Model {
			return press(t, typeText(t, press(t, m, "I"), "/src/pack"), "enter", "space")
		}, mode: modeImportList},
		{name: "fix-up", setup: func(t *testing.T, m Model) Model {
			return press(t, typeText(t, press(t, m, "I"), "/src/bad"), "enter")
		}, mode: modeImportFix},
		{name: "picker", setup: func(t *testing.T, m Model) Model {
			return press(t, typeText(t, press(t, m, "I"), m.home), "ctrl+o")
		}, mode: modeImportBrowse},
	}
	for _, size := range [][2]int{{60, 20}, {120, 40}} {
		for _, st := range steps {
			t.Run(fmt.Sprintf("%s at %dx%d", st.name, size[0], size[1]), func(t *testing.T) {
				b := &fakeBackend{discovered: map[string][]ops.Candidate{
					"/src/pack": cands,
					"/src/bad":  {fixable},
				}}
				m := sized(t, loaded(t, b, []config.Agent{claude}), size[0], size[1])
				m.home = t.TempDir()
				for i := range 30 {
					writeSkillFolder(t, m.home, fmt.Sprintf("entry-%d-%s", i, long))
				}
				m = st.setup(t, m)
				if m.mode != st.mode {
					t.Fatalf("mode = %v, want %v; result %q", m.mode, st.mode, m.lastMsg())
				}
				assertFits(t, size[0], m.body())
				lines := strings.Split(m.View(), "\n")
				if len(lines) != size[1] {
					t.Errorf("view has %d rows, want %d", len(lines), size[1])
				}
				assertFits(t, size[0], lines)
			})
		}
	}
}

func TestImportBarNamesItsKeys(t *testing.T) {
	m := loaded(t, &fakeBackend{}, dialogAgents)
	tests := []struct {
		mode mode
		want []string
	}{
		{mode: modeImport, want: []string{"enter run", "ctrl+o browse"}},
		{mode: modeImportList, want: []string{"space mark", "a mark all", "enter import"}},
		{mode: modeImportFix, want: []string{"ctrl+s import", "tab next field"}},
		{mode: modeImportBrowse, want: []string{"space choose", "enter open", "backspace up", ". dotfiles"}},
	}
	for _, tt := range tests {
		m.mode = tt.mode
		own, _ := m.barHints()
		text := sgr.ReplaceAllString(m.hintText(own), "")
		for _, w := range tt.want {
			if !strings.Contains(text, w) {
				t.Errorf("mode %v: bar %q lacks %q", tt.mode, text, w)
			}
		}
	}
}
