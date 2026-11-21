package theme

import (
	"reflect"
	"regexp"
	"slices"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"sapato/internal/config"
	"sapato/internal/ops"
)

var hexColor = regexp.MustCompile(`^#[0-9a-f]{6}$`)

func TestEveryThemeFillsEveryRole(t *testing.T) {
	const roles = 14 // docs/themes.md: Base through Pink
	for _, th := range All() {
		v := reflect.ValueOf(th)
		filled := 0
		for i := range v.NumField() {
			name := v.Type().Field(i).Name
			if name == "Name" {
				continue
			}
			filled++
			if c := v.Field(i).String(); !hexColor.MatchString(c) {
				t.Errorf("%s.%s = %q, want #rrggbb", th.Name, name, c)
			}
		}
		if filled != roles {
			t.Errorf("%s has %d roles, want %d", th.Name, filled, roles)
		}
	}
}

func TestThemeNamesAreUniqueAndMatchConfig(t *testing.T) {
	var names []string
	seen := map[string]bool{}
	for _, th := range All() {
		if seen[th.Name] {
			t.Errorf("duplicate theme name %s", th.Name)
		}
		seen[th.Name] = true
		names = append(names, th.Name)
	}
	if !slices.Equal(names, config.ThemeNames()) {
		t.Errorf("All() names = %v, want %v", names, config.ThemeNames())
	}
	if !slices.Equal(Names(), config.ThemeNames()) {
		t.Errorf("Names() = %v, want %v", Names(), config.ThemeNames())
	}
	for _, name := range names {
		if th, ok := ByName(name); !ok || th.Name != name {
			t.Errorf("ByName(%q) = %q, %t", name, th.Name, ok)
		}
	}
	if _, ok := ByName("solarized"); ok {
		t.Error("ByName of an unknown name reports ok")
	}
}

func TestPaletteSpotCheck(t *testing.T) {
	// The Accent of each theme, as written in docs/themes.md.
	tests := []struct {
		name   string
		accent lipgloss.Color
	}{
		{"catppuccin-latte", "#1e66f5"},
		{"catppuccin-frappe", "#8caaee"},
		{"catppuccin-macchiato", "#8aadf4"},
		{"catppuccin-mocha", "#89b4fa"},
		{"dracula", "#bd93f9"},
		{"dracula-alucard", "#644ac9"},
		{"nord", "#88c0d0"},
		{"nord-light", "#5e81ac"},
	}
	for _, tt := range tests {
		th, ok := ByName(tt.name)
		if !ok {
			t.Fatalf("theme %s is missing", tt.name)
		}
		if th.Accent != tt.accent {
			t.Errorf("%s Accent = %s, want %s", tt.name, th.Accent, tt.accent)
		}
	}
}

func TestStatusColorUsesRoles(t *testing.T) {
	th, _ := ByName("catppuccin-mocha")
	tests := []struct {
		state ops.State
		want  lipgloss.Color
	}{
		{ops.Synced, th.Green},
		{ops.Outdated, th.Yellow},
		{ops.Modified, th.Orange},
		{ops.NotInstalled, th.Overlay},
		{ops.Invalid, th.Red},
		{ops.Orphaned, th.Purple},
		{ops.Unmanaged, th.Cyan},
		{ops.Missing, th.Pink},
	}
	for _, tt := range tests {
		if got := th.StatusColor(tt.state); got != tt.want {
			t.Errorf("StatusColor(%s) = %s, want %s", tt.state, got, tt.want)
		}
	}
}

func TestModeColorUsesRoles(t *testing.T) {
	th, _ := ByName("catppuccin-mocha")
	tests := []struct {
		mode string
		want lipgloss.Color
	}{
		{"MAIN", th.Accent},
		{"SKILL", th.Green},
		{"FILTER", th.Yellow},
		{"ADOPT", th.Purple},
		{"NEW", th.Orange},
		{"IMPORT", th.Cyan},
		{"EXPORT", th.Cyan},
		{"THEME", th.Pink},
		{"HELP", th.Overlay},
		{"UNKNOWN", th.Overlay},
	}
	for _, tt := range tests {
		if got := th.ModeColor(tt.mode); got != tt.want {
			t.Errorf("ModeColor(%s) = %s, want %s", tt.mode, got, tt.want)
		}
	}
}
