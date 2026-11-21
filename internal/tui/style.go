package tui

import (
	"github.com/charmbracelet/lipgloss"

	"sapato/internal/ops"
	"sapato/internal/tui/theme"
)

// stateInfo describes one status in the matrix and the help legend.
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
// theme. With noColor set, no style sets a foreground color, so the glyphs and
// bold/reverse carry the meaning.
type styles struct {
	glyph   map[ops.State]lipgloss.Style
	bold    lipgloss.Style
	dim     lipgloss.Style
	accent  lipgloss.Style
	fail    lipgloss.Style
	added   lipgloss.Style
	removed lipgloss.Style
}

func newStyles(th theme.Theme, noColor bool) styles {
	base := lipgloss.NewStyle()
	color := func(c lipgloss.Color) lipgloss.Style {
		if noColor {
			return base
		}
		return base.Foreground(c)
	}
	s := styles{
		glyph:   make(map[ops.State]lipgloss.Style, len(states)),
		bold:    base.Bold(true),
		dim:     color(th.Overlay),
		accent:  color(th.Accent),
		fail:    color(th.Red),
		added:   color(th.Green),
		removed: color(th.Red),
	}
	for _, st := range states {
		s.glyph[st.state] = color(th.StatusColor(st.state))
	}
	return s
}
