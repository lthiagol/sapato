package tui

import (
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"sapato/internal/config"
	"sapato/internal/ops"
	"sapato/internal/tui/icons"
	"sapato/internal/tui/theme"
)

// testLook returns the theme and glyph set the tests draw with. The expectations
// in the other tests name the unicode glyphs, which the defaults before task 08 used.
func testLook(t *testing.T) (theme.Theme, icons.Set) {
	t.Helper()
	th, ok := theme.ByName("catppuccin-mocha")
	if !ok {
		t.Fatal("theme catppuccin-mocha is missing")
	}
	set, ok := icons.ByName("unicode")
	if !ok {
		t.Fatal("icon set unicode is missing")
	}
	return th, set
}

func TestStylesUseThemeRoles(t *testing.T) {
	for _, th := range theme.All() {
		t.Run(th.Name, func(t *testing.T) {
			s := newStyles(th, false)
			roles := []struct {
				name  string
				style lipgloss.Style
				want  lipgloss.Color
			}{
				{"dim", s.dim, th.Overlay},
				{"accent", s.accent, th.Accent},
				{"fail", s.fail, th.Red},
				{"added", s.added, th.Green},
				{"removed", s.removed, th.Red},
			}
			for _, r := range roles {
				if got := r.style.GetForeground(); got != r.want {
					t.Errorf("%s foreground = %v, want %v", r.name, got, r.want)
				}
			}
			for _, st := range states {
				if got := s.glyph[st.state].GetForeground(); got != th.StatusColor(st.state) {
					t.Errorf("%s glyph foreground = %v, want %v", st.state, got, th.StatusColor(st.state))
				}
			}
		})
	}
}

func TestIconSetDrawsTheMatrix(t *testing.T) {
	th, _ := testLook(t)
	for _, name := range []string{"nerd", "unicode", "ascii"} {
		t.Run(name, func(t *testing.T) {
			set, ok := icons.ByName(name)
			if !ok {
				t.Fatalf("icon set %s is missing", name)
			}
			b := &fakeBackend{rows: []ops.Row{row("alpha", "claude", ops.Synced)}}
			m := New(b, []config.Agent{claude}, th, set, true)
			m, _ = settle(t, m, m.Init())

			var rowLine string
			for _, line := range strings.Split(m.View(), "\n") {
				if strings.HasPrefix(line, set.Marker+" alpha") {
					rowLine = line
				}
			}
			if rowLine == "" {
				t.Fatalf("no selected row drawn with marker %q:\n%s", set.Marker, m.View())
			}
			if !strings.Contains(rowLine, set.Status[ops.Synced]) {
				t.Errorf("row %q lacks the synced glyph %q", rowLine, set.Status[ops.Synced])
			}
		})
	}
}

func TestRunRejectsUnknownLook(t *testing.T) {
	tests := []struct {
		name string
		ui   config.UI
		want string
	}{
		{"theme", config.UI{Theme: "solarized", Icons: "nerd"}, `unknown theme "solarized"`},
		{"icons", config.UI{Theme: "nord", Icons: "emoji"}, `unknown icon set "emoji"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{Vault: t.TempDir(), UI: tt.ui}
			err := Run(cfg, strings.NewReader(""), io.Discard)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Run() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}
