// Package icons holds the glyph sets of spec §9.5. The UI draws glyphs from the
// fields of Set only.
package icons

import "sapato/internal/ops"

// Set is one glyph set. Status has a glyph for each of the eight states. Info
// marks an info notification and Exec an executable file. Sets carry no separators.
type Set struct {
	Status       map[ops.State]string
	Marker, Mark string
	Exec, Info   string
	Rule         string
}

// sets holds the glyph sets by name. Names match config.IconNames.
var sets = map[string]Set{
	"nerd": {
		Status: map[ops.State]string{
			ops.Synced:       "\uf058",
			ops.Outdated:     "\uf0aa",
			ops.Modified:     "\uf040",
			ops.NotInstalled: "\uf10c",
			ops.Invalid:      "\uf057",
			ops.Orphaned:     "\uf127",
			ops.Unmanaged:    "\uf059",
			ops.Missing:      "\uf071",
		},
		Marker: "\uf054",
		Mark:   "\uf14a",
		Exec:   "\uf013",
		Info:   "\uf05a",
		Rule:   "─",
	},
	"unicode": {
		Status: map[ops.State]string{
			ops.Synced:       "✔",
			ops.Outdated:     "↑",
			ops.Modified:     "✎",
			ops.NotInstalled: "·",
			ops.Invalid:      "✖",
			ops.Orphaned:     "∅",
			ops.Unmanaged:    "◆",
			ops.Missing:      "!",
		},
		Marker: "▶",
		Mark:   "■",
		Exec:   "⚙",
		Info:   "ℹ",
		Rule:   "─",
	},
	"ascii": {
		Status: map[ops.State]string{
			ops.Synced:       "ok",
			ops.Outdated:     "up",
			ops.Modified:     "mod",
			ops.NotInstalled: ".",
			ops.Invalid:      "x",
			ops.Orphaned:     "orp",
			ops.Unmanaged:    "?",
			ops.Missing:      "!",
		},
		Marker: ">",
		Mark:   "*",
		Exec:   "x",
		Info:   "i",
		Rule:   "-",
	},
}

// ByName returns the glyph set called name: "nerd", "unicode" or "ascii". ok is
// false for any other name.
func ByName(name string) (Set, bool) {
	set, ok := sets[name]
	return set, ok
}
