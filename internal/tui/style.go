package tui

import (
	"github.com/charmbracelet/lipgloss"

	"sapato/internal/ops"
	"sapato/internal/tui/theme"
)

// stateInfo describes one status in the grid and the help legend.
type stateInfo struct {
	state ops.State
	desc  string
}

// states lists the eight statuses of spec §6, in spec order.
var states = []stateInfo{
	{ops.NotInstalled, "in the vault, not in this agent"},
	{ops.Invalid, "in the vault but invalid; never installed"},
	{ops.Synced, "installed and matching the vault"},
	{ops.Outdated, "installed; the vault has a newer version"},
	{ops.Modified, "installed; local edits differ from the lock"},
	{ops.Orphaned, "installed; no longer in the vault"},
	{ops.Unmanaged, "in this agent, not managed by sapato"},
	{ops.Missing, "in the lock, but its folder is gone"},
}

// styles holds every Lip Gloss style the UI uses, colored from the roles of the
// theme. With noColor set, no style sets a color, so the glyphs and bold/reverse
// carry the meaning.
type styles struct {
	th      theme.Theme
	noColor bool

	glyph   map[ops.State]lipgloss.Style
	bold    lipgloss.Style
	dim     lipgloss.Style
	accent  lipgloss.Style
	subtext lipgloss.Style
	fail    lipgloss.Style
	added   lipgloss.Style
	removed lipgloss.Style
	rule    lipgloss.Style
	bar     lipgloss.Style
	barDim  lipgloss.Style
	barFail lipgloss.Style
	seg     lipgloss.Style
	row     lipgloss.Style
}

func newStyles(th theme.Theme, noColor bool) styles {
	base := lipgloss.NewStyle()
	// paint sets the foreground and background roles, unless noColor is set.
	paint := func(fg, bg lipgloss.Color) lipgloss.Style {
		if noColor {
			return base
		}
		s := base.Foreground(fg)
		if bg != "" {
			s = s.Background(bg)
		}
		return s
	}
	s := styles{
		th:      th,
		noColor: noColor,
		glyph:   make(map[ops.State]lipgloss.Style, len(states)),
		bold:    base.Bold(true),
		dim:     paint(th.Overlay, ""),
		accent:  paint(th.Accent, ""),
		subtext: paint(th.Subtext, ""),
		fail:    paint(th.Red, ""),
		added:   paint(th.Green, ""),
		removed: paint(th.Red, ""),
		rule:    paint(th.Overlay, ""),
		bar:     paint(th.Text, th.Mantle),
		barDim:  paint(th.Overlay, th.Mantle),
		barFail: paint(th.Red, th.Mantle),
		seg:     paint(th.Text, th.Surface),
	}
	for _, st := range states {
		s.glyph[st.state] = paint(th.StatusColor(st.state), "")
	}
	s.row = base.Reverse(true)
	if !noColor {
		s.row = base.Background(th.Surface)
	}
	return s
}

// badge paints the mode badge of name in its mode color.
func (s styles) badge(name string) lipgloss.Style {
	if s.noColor {
		return s.bold.Reverse(true)
	}
	return lipgloss.NewStyle().Bold(true).Foreground(s.th.Base).Background(s.th.ModeColor(name))
}

// powerline paints a separator that points from the segment of color fg into
// the bar.
func (s styles) powerline(fg lipgloss.Color) lipgloss.Style {
	if s.noColor {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(fg).Background(s.th.Mantle)
}
