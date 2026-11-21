// Package icons holds the glyph sets of spec §9.5. The UI draws glyphs from the
// fields of Set only.
package icons

import "sapato/internal/ops"

// namedSet pairs a glyph set with its name.
type namedSet struct {
	name string
	set  Set
}

// Set is one glyph set. Status has a glyph for each of the eight states. Info
// marks an info notification and Exec an executable file. Branch, Last and Pipe draw
// the file tree: a branch, the last branch of a folder, and the line that runs down
// past a branch. Sets carry no separators.
type Set struct {
	Status       map[ops.State]string
	Marker, Mark string
	Exec, Info   string
	Rule         string
	Branch, Last string
	Pipe         string
}

// sets lists the glyph sets in UI order. Names match config.IconNames.
var sets = []namedSet{
	{name: "nerd", set: Set{
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
		Branch: "├── ",
		Last:   "└── ",
		Pipe:   "│   ",
	}},
	{name: "unicode", set: Set{
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
		Branch: "├── ",
		Last:   "└── ",
		Pipe:   "│   ",
	}},
	{name: "ascii", set: Set{
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
		Branch: "|-- ",
		Last:   "`-- ",
		Pipe:   "|   ",
	}},
}

// ByName returns the glyph set called name: "nerd", "unicode" or "ascii". ok is
// false for any other name.
func ByName(name string) (Set, bool) {
	for _, s := range sets {
		if s.name == name {
			return s.set, true
		}
	}
	return Set{}, false
}

// Names returns the glyph set names in UI order.
func Names() []string {
	names := make([]string, len(sets))
	for i, s := range sets {
		names[i] = s.name
	}
	return names
}
