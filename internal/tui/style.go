package tui

import (
	"github.com/charmbracelet/lipgloss"

	"sapato/internal/ops"
)

// stateInfo describes one status in the matrix and the help legend.
type stateInfo struct {
	state ops.State
	glyph string
	color lipgloss.AdaptiveColor
	desc  string
}

// states lists the eight statuses of spec §6, in spec order.
var states = []stateInfo{
	{ops.NotInstalled, "·", lipgloss.AdaptiveColor{Light: "246", Dark: "244"}, "in the vault, not in this agent"},
	{ops.Invalid, "✖", lipgloss.AdaptiveColor{Light: "160", Dark: "203"}, "in the vault but invalid; never installed"},
	{ops.Synced, "✔", lipgloss.AdaptiveColor{Light: "28", Dark: "78"}, "installed and matching the vault"},
	{ops.Outdated, "↑", lipgloss.AdaptiveColor{Light: "130", Dark: "214"}, "installed; the vault has a newer version"},
	{ops.Modified, "✎", lipgloss.AdaptiveColor{Light: "127", Dark: "213"}, "installed; local edits differ from the lock"},
	{ops.Orphaned, "∅", lipgloss.AdaptiveColor{Light: "94", Dark: "180"}, "installed; no longer in the vault"},
	{ops.Unmanaged, "◆", lipgloss.AdaptiveColor{Light: "25", Dark: "75"}, "in this agent, not managed by sapato"},
	{ops.Missing, "!", lipgloss.AdaptiveColor{Light: "166", Dark: "208"}, "in the lock, but its folder is gone"},
}

// styles holds every Lip Gloss style the UI uses. With noColor set, no style
// sets a foreground color, so the glyphs and bold/reverse carry the meaning.
type styles struct {
	glyph   map[ops.State]lipgloss.Style
	bold    lipgloss.Style
	dim     lipgloss.Style
	fail    lipgloss.Style
	added   lipgloss.Style
	removed lipgloss.Style
}

func newStyles(noColor bool) styles {
	base := lipgloss.NewStyle()
	color := func(c lipgloss.AdaptiveColor) lipgloss.Style {
		if noColor {
			return base
		}
		return base.Foreground(c)
	}
	s := styles{
		glyph:   make(map[ops.State]lipgloss.Style, len(states)),
		bold:    base.Bold(true),
		dim:     color(lipgloss.AdaptiveColor{Light: "245", Dark: "243"}),
		fail:    color(lipgloss.AdaptiveColor{Light: "160", Dark: "203"}),
		added:   color(lipgloss.AdaptiveColor{Light: "28", Dark: "78"}),
		removed: color(lipgloss.AdaptiveColor{Light: "160", Dark: "203"}),
	}
	for _, st := range states {
		s.glyph[st.state] = color(st.color)
	}
	return s
}
