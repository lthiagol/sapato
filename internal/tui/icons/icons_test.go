package icons

import (
	"maps"
	"slices"
	"testing"

	"sapato/internal/config"
	"sapato/internal/ops"
)

// TestSetsMatchSpec pins every glyph of spec §9.5 and checks that each set
// covers the eight states of §6.
func TestSetsMatchSpec(t *testing.T) {
	tests := []struct {
		name         string
		status       map[ops.State]string
		marker, mark string
		exec, info   string
	}{
		{
			name: "nerd",
			status: map[ops.State]string{
				ops.Synced: "", ops.Outdated: "", ops.Modified: "", ops.NotInstalled: "",
				ops.Invalid: "", ops.Orphaned: "", ops.Unmanaged: "", ops.Missing: "",
			},
			marker: "", mark: "", exec: "", info: "",
		},
		{
			name: "unicode",
			status: map[ops.State]string{
				ops.Synced: "✔", ops.Outdated: "↑", ops.Modified: "✎", ops.NotInstalled: "·",
				ops.Invalid: "✖", ops.Orphaned: "∅", ops.Unmanaged: "◆", ops.Missing: "!",
			},
			marker: "▶", mark: "■", exec: "⚙", info: "ℹ",
		},
		{
			name: "ascii",
			status: map[ops.State]string{
				ops.Synced: "ok", ops.Outdated: "up", ops.Modified: "mod", ops.NotInstalled: ".",
				ops.Invalid: "x", ops.Orphaned: "orp", ops.Unmanaged: "?", ops.Missing: "!",
			},
			marker: ">", mark: "*", exec: "x", info: "i",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set, ok := ByName(tt.name)
			if !ok {
				t.Fatalf("ByName(%q) not found", tt.name)
			}
			if !maps.Equal(set.Status, tt.status) {
				t.Errorf("Status = %q, want %q", set.Status, tt.status)
			}
			if set.Marker != tt.marker || set.Mark != tt.mark {
				t.Errorf("Marker, Mark = %q, %q; want %q, %q", set.Marker, set.Mark, tt.marker, tt.mark)
			}
			if set.Exec != tt.exec || set.Info != tt.info {
				t.Errorf("Exec, Info = %q, %q; want %q, %q", set.Exec, set.Info, tt.exec, tt.info)
			}
		})
	}
}

func TestSetNamesMatchConfig(t *testing.T) {
	if got, want := len(sets), len(config.IconNames()); got != want {
		t.Errorf("%d sets, want %d", got, want)
	}
	for _, name := range config.IconNames() {
		if _, ok := ByName(name); !ok {
			t.Errorf("ByName(%q) not found", name)
		}
	}
	if _, ok := ByName("emoji"); ok {
		t.Error("ByName of an unknown name reports ok")
	}
	if !slices.Equal(config.IconNames(), []string{"nerd", "unicode", "ascii"}) {
		t.Errorf("IconNames() = %v", config.IconNames())
	}
}

// TestRule checks the glyph that draws the rules. No set carries separators.
func TestRule(t *testing.T) {
	tests := []struct {
		name string
		rule string
	}{
		{name: "nerd", rule: "─"},
		{name: "unicode", rule: "─"},
		{name: "ascii", rule: "-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set, ok := ByName(tt.name)
			if !ok {
				t.Fatalf("ByName(%q) not found", tt.name)
			}
			if set.Rule != tt.rule {
				t.Errorf("Rule = %q, want %q", set.Rule, tt.rule)
			}
		})
	}
}
