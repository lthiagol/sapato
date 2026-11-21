package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap holds the bindings of every mode. Each binding's help text feeds the
// hint line and the help view.
type keyMap struct {
	up, down, left, right key.Binding
	install, uninstall    key.Binding
	update, adopt         key.Binding
	diff, refresh         key.Binding
	filter, help, quit    key.Binding
	forceQuit             key.Binding
	enter, esc            key.Binding
	yes, no               key.Binding
	closeView             key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		left:      key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "left")),
		right:     key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "right")),
		install:   key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "install")),
		uninstall: key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "uninstall")),
		update:    key.NewBinding(key.WithKeys("U"), key.WithHelp("U", "update")),
		adopt:     key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "adopt")),
		diff:      key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "diff")),
		refresh:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		filter:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		quit:      key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		forceQuit: key.NewBinding(key.WithKeys("ctrl+c")),
		enter:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "apply filter")),
		esc:       key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear filter")),
		yes:       key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yes")),
		// no has no keys: any key other than y cancels a confirm. It labels the hint.
		no:        key.NewBinding(key.WithHelp("n", "no")),
		closeView: key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc", "back")),
	}
}

// moveHint is a help-only entry for the four cursor keys. It binds no keys.
var moveHint = key.NewBinding(key.WithHelp("↑↓←→/hjkl", "move"))

// matrixHints lists the matrix bindings in hint-line order.
func (k keyMap) matrixHints() []key.Binding {
	return []key.Binding{moveHint, k.install, k.uninstall, k.update, k.adopt, k.diff, k.refresh, k.filter, k.help, k.quit}
}
