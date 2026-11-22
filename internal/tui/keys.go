package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap holds the bindings of every screen and mode. Each binding's help text
// feeds the bar hints and the help view.
type keyMap struct {
	up, down, top, bottom key.Binding
	pageUp, pageDown      key.Binding
	enter, esc            key.Binding
	filter, refresh       key.Binding
	help, quit, forceQuit key.Binding
	// log opens the LOG view of the notifications.
	log key.Binding
	// The main screen's keys for work that is not built yet.
	adopt, newSkill, importTar, theme key.Binding
	// The skill screen's keys. adopt is also a main key, above.
	mark, install, uninstall key.Binding
	update, diff, installAll key.Binding
	compat, export, remove   key.Binding
	yes, no                  key.Binding
	closeView                key.Binding
	// save saves the NEW editor and the IMPORT fix-up form.
	save key.Binding
	// The IMPORT keys: openFile opens the file picker, markAll marks every skill of
	// the list, parent goes up a folder in the picker, dotfiles shows or hides the
	// dot entries, and field moves between the fix-up form's inputs.
	openFile, markAll, parent, dotfiles, field key.Binding
	// The NEW keys: tab switches between the wizard and the paste mode. The wizard
	// moves between its fields with the arrows alone, since its inputs take j and k.
	tab, fieldUp, fieldDown key.Binding
	// The ADOPT keys: agentNext and agentPrev switch the agent the picker shows.
	agentNext, agentPrev key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		top:       key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
		bottom:    key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		pageUp:    key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
		pageDown:  key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "page down")),
		enter:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		esc:       key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear filter")),
		filter:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		refresh:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		quit:      key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		forceQuit: key.NewBinding(key.WithKeys("ctrl+c")),
		log:       key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "log")),

		adopt:     key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "adopt")),
		newSkill:  key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
		importTar: key.NewBinding(key.WithKeys("I"), key.WithHelp("I", "import")),
		theme:     key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),

		mark:       key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "mark")),
		install:    key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "install")),
		uninstall:  key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "uninstall")),
		update:     key.NewBinding(key.WithKeys("U"), key.WithHelp("U", "update")),
		diff:       key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "diff")),
		installAll: key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "install all")),
		compat:     key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "compat")),
		export:     key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "export")),
		remove:     key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "remove")),

		yes: key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yes")),
		// no has no keys: any key other than y cancels a confirm. It labels the hint.
		no:        key.NewBinding(key.WithHelp("n", "no")),
		closeView: key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc", "back")),
		save:      key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "save")),

		openFile: key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("ctrl+o", "browse")),
		markAll:  key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "mark all")),
		parent:   key.NewBinding(key.WithKeys("backspace"), key.WithHelp("backspace", "up")),
		dotfiles: key.NewBinding(key.WithKeys("."), key.WithHelp(".", "dotfiles")),
		field:    key.NewBinding(key.WithKeys("tab", "shift+tab"), key.WithHelp("tab", "next field")),

		tab:       key.NewBinding(key.WithKeys("tab")),
		fieldUp:   key.NewBinding(key.WithKeys("up")),
		fieldDown: key.NewBinding(key.WithKeys("down")),

		agentNext: key.NewBinding(key.WithKeys("tab", "right")),
		agentPrev: key.NewBinding(key.WithKeys("shift+tab", "left")),
	}
}

// moveHint and pageHint are help-only entries. They bind no keys, so the hints
// name the cursor and page keys that the bindings above handle.
var (
	moveHint = key.NewBinding(key.WithHelp("↑↓/jk", "move"))
	pageHint = key.NewBinding(key.WithHelp("pgup/pgdn", "page"))
	backHint = key.NewBinding(key.WithHelp("esc", "back"))
	// filterApplyHint and filterClearHint label the keys of the filter mode.
	filterApplyHint = key.NewBinding(key.WithHelp("enter", "apply"))
	filterClearHint = key.NewBinding(key.WithHelp("esc", "clear"))
	// scrollHint is a help-only entry for the pager's scroll keys.
	scrollHint = key.NewBinding(key.WithHelp("↑↓ pgup/pgdn", "scroll"))
	// The dialog hints name the keys that the dialogs match with the bindings above.
	adoptHint   = key.NewBinding(key.WithHelp("enter", "adopt"))
	agentHint   = key.NewBinding(key.WithHelp("tab/←→", "agent"))
	runHint     = key.NewBinding(key.WithHelp("enter", "run"))
	keepHint    = key.NewBinding(key.WithHelp("enter", "keep"))
	cancelHint  = key.NewBinding(key.WithHelp("esc", "cancel"))
	restoreHint = key.NewBinding(key.WithHelp("esc", "restore"))
	// The IMPORT hints name the keys of its list, file picker and fix-up form.
	importHint = key.NewBinding(key.WithHelp("enter", "import"))
	fixHint    = key.NewBinding(key.WithHelp("ctrl+s", "import"))
	fieldHint  = key.NewBinding(key.WithHelp("tab", "next field"))
	chooseHint = key.NewBinding(key.WithHelp("space", "choose"))
	openHint   = key.NewBinding(key.WithHelp("enter", "open"))
	upHint     = key.NewBinding(key.WithHelp("backspace", "up"))
	// The NEW hints name the keys of the wizard and the paste mode.
	pasteHint  = key.NewBinding(key.WithHelp("tab", "paste"))
	wizardHint = key.NewBinding(key.WithHelp("tab", "wizard"))
	fieldsHint = key.NewBinding(key.WithHelp("↑↓", "fields"))
)

// mainHints lists the main screen's own bindings in hint order: the actions the bar
// shows in its first row.
func (k keyMap) mainHints() []key.Binding {
	return []key.Binding{k.enter, k.filter, k.refresh, k.adopt, k.newSkill, k.importTar}
}

// mainGlobals lists the main screen's global bindings, which the bar shows in its
// second row.
func (k keyMap) mainGlobals() []key.Binding {
	return []key.Binding{moveHint, k.theme, k.log, k.help, k.quit}
}

// filterHints lists the bindings the bar shows while filtering.
func (k keyMap) filterHints() []key.Binding {
	return []key.Binding{filterApplyHint, filterClearHint}
}

// skillHints lists the skill screen's own bindings in hint order: the actions the bar
// shows in its first row.
func (k keyMap) skillHints() []key.Binding {
	return []key.Binding{k.mark, k.install, k.uninstall, k.update, k.diff, k.adopt, k.installAll, k.compat, k.export, k.remove}
}

// skillGlobals lists the skill screen's global bindings, which the bar shows in its
// second row, back first.
func (k keyMap) skillGlobals() []key.Binding {
	return []key.Binding{moveHint, pageHint, backHint, k.theme, k.log, k.help, k.quit}
}
