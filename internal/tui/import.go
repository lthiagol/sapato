package tui

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"sapato/internal/ops"
	"sapato/internal/vault"
)

// The IMPORT flow reads its source first. Several skills open the list, and a skill
// that --fix repairs opens the fix-up form. Any other source is imported straight
// away. Each step runs through the plan path, so a refusal that --force overrides
// gets the force prompt, and the list and the form keep their state while it shows.

// candList is the IMPORT list of several skills: the source, its candidates, the
// cursor and the marks by candidate folder (Rel).
type candList struct {
	src      string
	cands    []ops.Candidate
	cur, top int
	marks    map[string]bool
}

// fixForm is the fix-up form of a skill that --fix repairs: the candidate and its
// editable name and description. focus is 0 while the name takes keys, 1 for the
// description.
type fixForm struct {
	src        string
	cand       ops.Candidate
	name, desc textinput.Model
	focus      int
}

// browser is the IMPORT file picker: the folder it lists, its entries and the cursor.
// dot shows the entries whose names start with a dot.
type browser struct {
	dir      string
	entries  []browseEntry
	cur, top int
	dot      bool
}

// browseEntry is one row of the file picker. Folders, .md files and tarballs are
// choosable. Other files are listed, dimmed, but not chosen.
type browseEntry struct {
	name      string
	dir       bool
	choosable bool
}

// discoveredMsg carries the candidates of an import source, read by discoverCmd.
type discoveredMsg struct {
	src   string
	cands []ops.Candidate
	err   error
}

// discoverCmd reads the candidates of src off the update loop.
func discoverCmd(b Backend, src string) tea.Cmd {
	return func() tea.Msg {
		cands, err := b.Discover(src)
		return discoveredMsg{src: src, cands: cands, err: err}
	}
}

// startImport reads the typed source. What enter does next depends on the candidates.
func (m Model) startImport() (Model, tea.Cmd) {
	src := strings.TrimSpace(m.path.Value())
	if src == "" {
		m.notify(noteError, "error", "import needs a path")
		return m, nil
	}
	return m, discoverCmd(m.backend, src)
}

// onDiscovered picks the next step for the candidates of the source. A reading that
// is no longer for the typed source, or that arrives after IMPORT closed, is dropped.
func (m Model) onDiscovered(msg discoveredMsg) (Model, tea.Cmd) {
	if m.mode != modeImport || strings.TrimSpace(m.path.Value()) != msg.src {
		return m, nil
	}
	if msg.err != nil {
		m.notify(noteError, "error", msg.err.Error())
		return m, nil
	}
	switch {
	case len(msg.cands) > 1:
		m.multi = candList{src: msg.src, cands: msg.cands, marks: map[string]bool{}}
		m.mode = modeImportList
		return m, nil
	case len(msg.cands) == 1 && !msg.cands[0].Valid && msg.cands[0].Fixable:
		m.fixup = newFixForm(msg.src, msg.cands[0])
		m.sizeInputs()
		m.mode = modeImportFix
		return m, m.fixup.name.Focus()
	}
	return m, planCmd(m.backend, action{op: ops.OpImport, src: msg.src, back: modeImport})
}

// newFixForm returns the fix-up form of candidate c from source src. The name is
// prefilled with the frontmatter name when that is valid, else with the name hint
// normalized, as --fix would take it (spec §7.1).
func newFixForm(src string, c ops.Candidate) fixForm {
	name := c.Name
	if !vault.ValidName(name) {
		name = ops.NormalizeName(c.Hint)
	}
	f := fixForm{src: src, cand: c, name: newPathInput(), desc: newPathInput()}
	f.name.SetValue(name)
	f.name.CursorEnd()
	return f
}

// importListKey moves the cursor over the list and marks its skills. Invalid skills
// cannot be marked: space says why, and a marks only the valid ones. enter imports
// the marked skills, and esc returns to the source input.
func (m Model) importListKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	l := &m.multi
	switch {
	case key.Matches(msg, m.keys.up):
		l.cur = clampIndex(l.cur-1, len(l.cands))
	case key.Matches(msg, m.keys.down):
		l.cur = clampIndex(l.cur+1, len(l.cands))
	case key.Matches(msg, m.keys.mark):
		c := l.cands[l.cur]
		switch {
		case !c.Valid:
			m.notify(noteError, "error", c.Rel+" is "+invalidText(c))
		case l.marks[c.Rel]:
			delete(l.marks, c.Rel)
		default:
			l.marks[c.Rel] = true
		}
	case key.Matches(msg, m.keys.markAll):
		for _, c := range l.cands {
			if c.Valid {
				l.marks[c.Rel] = true
			}
		}
	case key.Matches(msg, m.keys.enter):
		return m.importMarked()
	case key.Matches(msg, m.keys.esc):
		m.mode = modeImport
	}
	scrollWindow(&l.top, l.cur, m.listRows())
	return m, nil
}

// invalidText says why candidate c is not imported: invalid, with its first problem.
func invalidText(c ops.Candidate) string {
	if len(c.Problems) == 0 {
		return "invalid"
	}
	return "invalid: " + c.Problems[0]
}

// importMarked plans the import of the marked skills, in list order. With none
// marked it refuses and says how to mark.
func (m Model) importMarked() (Model, tea.Cmd) {
	var pick []string
	for _, c := range m.multi.cands {
		if m.multi.marks[c.Rel] {
			pick = append(pick, c.Rel)
		}
	}
	if len(pick) == 0 {
		m.notify(noteError, "error", "no skill marked: space marks one, a marks all")
		return m, nil
	}
	act := action{op: ops.OpImport, src: m.multi.src, importOpts: ops.ImportOptions{Pick: pick}, back: modeImportList}
	return m, planCmd(m.backend, act)
}

// importFixKey types into the fix-up form. tab moves between the name and the
// description. ctrl+s imports the skill with the fix, and esc returns to the source
// input.
func (m Model) importFixKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	f := &m.fixup
	switch {
	case key.Matches(msg, m.keys.save):
		act := action{
			op:  ops.OpImport,
			src: f.src,
			importOpts: ops.ImportOptions{
				Name:        strings.TrimSpace(f.name.Value()),
				Description: strings.TrimSpace(f.desc.Value()),
				Fix:         true,
			},
			back: modeImportFix,
		}
		return m, planCmd(m.backend, act)
	case key.Matches(msg, m.keys.esc):
		m.mode = modeImport
		return m, nil
	case key.Matches(msg, m.keys.field):
		f.focus = 1 - f.focus
		if f.focus == 0 {
			f.desc.Blur()
			return m, f.name.Focus()
		}
		f.name.Blur()
		return m, f.desc.Focus()
	}
	var cmd tea.Cmd
	in := &f.name
	if f.focus == 1 {
		in = &f.desc
	}
	*in, cmd = in.Update(msg)
	return m, cmd
}

// openBrowse opens the file picker on the folder the source input names, or else on
// the home dir.
func (m Model) openBrowse() Model {
	b, err := listBrowser(m.browseStart(), "", m.browse.dot)
	if err != nil {
		m.notify(noteError, "error", err.Error())
		return m
	}
	m.browse = b
	m.mode = modeImportBrowse
	return m
}

// browseStart is the folder the file picker opens in (spec §9.4): the input's own
// folder when the input names one, else its parent folder when that exists, else the
// home dir.
func (m Model) browseStart() string {
	if p := expandHome(strings.TrimSpace(m.path.Value())); p != "" {
		for _, dir := range []string{p, filepath.Dir(p)} {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				return dir
			}
		}
	}
	if m.home != "" {
		return m.home
	}
	return string(filepath.Separator)
}

// listBrowser lists the folder dir for the file picker, with the cursor on the entry
// named sel when there is one. Folders come first, then files, each by name.
func listBrowser(dir, sel string, dot bool) (browser, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return browser{}, err
	}
	b := browser{dir: dir, dot: dot}
	for _, de := range des {
		name := de.Name()
		if !dot && strings.HasPrefix(name, ".") {
			continue
		}
		isDir := false
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil {
			isDir = info.IsDir()
		}
		b.entries = append(b.entries, browseEntry{name: name, dir: isDir, choosable: isDir || importableFile(name)})
	}
	slices.SortFunc(b.entries, func(x, y browseEntry) int {
		if x.dir != y.dir {
			if x.dir {
				return -1
			}
			return 1
		}
		return cmp.Compare(x.name, y.name)
	})
	b.cur = max(0, slices.IndexFunc(b.entries, func(e browseEntry) bool { return e.name == sel }))
	return b, nil
}

// importableFile reports whether a file name is a source IMPORT takes: a .md file or
// a tarball (spec §7.1).
func importableFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz")
}

// browseKey moves the picker cursor. enter opens a folder or chooses a file, and space
// chooses the entry under the cursor. backspace goes up a folder, "." shows or hides
// the dot entries, and esc returns to the source input.
func (m Model) browseKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	b := &m.browse
	sel, hasSel := b.selected()
	switch {
	case key.Matches(msg, m.keys.up):
		b.cur = clampIndex(b.cur-1, len(b.entries))
	case key.Matches(msg, m.keys.down):
		b.cur = clampIndex(b.cur+1, len(b.entries))
	case key.Matches(msg, m.keys.pageUp):
		b.cur = clampIndex(b.cur-m.browseRows(), len(b.entries))
	case key.Matches(msg, m.keys.pageDown):
		b.cur = clampIndex(b.cur+m.browseRows(), len(b.entries))
	case key.Matches(msg, m.keys.enter):
		if !hasSel {
			break
		}
		if sel.dir {
			return m.browseGo(filepath.Join(b.dir, sel.name), ""), nil
		}
		if sel.choosable {
			return m.chooseBrowse(sel.name), nil
		}
	case key.Matches(msg, m.keys.mark):
		if hasSel && sel.choosable {
			return m.chooseBrowse(sel.name), nil
		}
	case key.Matches(msg, m.keys.parent):
		if parent := filepath.Dir(b.dir); parent != b.dir {
			return m.browseGo(parent, filepath.Base(b.dir)), nil
		}
	case key.Matches(msg, m.keys.dotfiles):
		b.dot = !b.dot
		return m.browseGo(b.dir, sel.name), nil
	case key.Matches(msg, m.keys.esc):
		m.mode = modeImport
	}
	scrollWindow(&b.top, b.cur, m.browseRows())
	return m, nil
}

// selected is the entry under the cursor, and false when the folder is empty.
func (b browser) selected() (browseEntry, bool) {
	if b.cur < len(b.entries) {
		return b.entries[b.cur], true
	}
	return browseEntry{}, false
}

// browseGo lists dir in the picker, with the cursor on sel. A folder that cannot be
// read is reported, and the picker stays where it was.
func (m Model) browseGo(dir, sel string) Model {
	b, err := listBrowser(dir, sel, m.browse.dot)
	if err != nil {
		m.notify(noteError, "error", err.Error())
		return m
	}
	m.browse = b
	scrollWindow(&m.browse.top, m.browse.cur, m.browseRows())
	return m
}

// chooseBrowse fills the source input with the entry name of the picker's folder, and
// returns to IMPORT.
func (m Model) chooseBrowse(name string) Model {
	m.path.SetValue(filepath.Join(m.browse.dir, name))
	m.path.CursorEnd()
	m.mode = modeImport
	return m
}

// listRows is how many rows of the IMPORT list fit under its title and above its
// count. Before the first size message it is every row.
func (m Model) listRows() int {
	if m.height == 0 {
		return max(1, len(m.multi.cands))
	}
	return max(1, m.bodyHeight()-4)
}

// browseRows is how many rows of the file picker fit under its title and folder.
// Before the first size message it is every row.
func (m Model) browseRows() int {
	if m.height == 0 {
		return max(1, len(m.browse.entries))
	}
	return max(1, m.bodyHeight()-3)
}
