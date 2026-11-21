package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigUI(t *testing.T) {
	tests := []struct {
		name      string
		content   string // config file; empty means no file
		opts      globalOpts
		wantTheme string
		wantIcons string
		wantErr   string
		wantUsage bool
	}{
		{name: "defaults without a file", wantTheme: "catppuccin-mocha", wantIcons: "nerd"},
		{
			name:      "config file values",
			content:   "[ui]\ntheme = \"nord\"\nicons = \"ascii\"\n",
			wantTheme: "nord", wantIcons: "ascii",
		},
		{
			name:      "flags override the config file",
			content:   "[ui]\ntheme = \"nord\"\nicons = \"ascii\"\n",
			opts:      globalOpts{theme: "dracula", icons: "unicode"},
			wantTheme: "dracula", wantIcons: "unicode",
		},
		{
			name:      "one flag keeps the other config value",
			content:   "[ui]\ntheme = \"nord\"\nicons = \"ascii\"\n",
			opts:      globalOpts{icons: "unicode"},
			wantTheme: "nord", wantIcons: "unicode",
		},
		{name: "unknown theme flag is a usage error", opts: globalOpts{theme: "solarized"}, wantErr: `--theme: unknown theme "solarized"`, wantUsage: true},
		{name: "unknown icons flag is a usage error", opts: globalOpts{icons: "emoji"}, wantErr: `--icons: unknown icon set "emoji"`, wantUsage: true},
		{name: "unknown theme in the file is not a usage error", content: "[ui]\ntheme = \"solarized\"\n", wantErr: "ui.theme", wantUsage: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			cfgPath := filepath.Join(t.TempDir(), "config.toml")
			if tt.content != "" {
				if err := os.WriteFile(cfgPath, []byte(tt.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			opts := tt.opts
			opts.configPath = cfgPath

			cfg, err := loadConfig(&opts)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("loadConfig() error = %v, want containing %q", err, tt.wantErr)
				}
				var ue usageError
				if errors.As(err, &ue) != tt.wantUsage {
					t.Errorf("usage error = %t, want %t", errors.As(err, &ue), tt.wantUsage)
				}
				return
			}
			if err != nil {
				t.Fatalf("loadConfig() error = %v", err)
			}
			if cfg.UI.Theme != tt.wantTheme || cfg.UI.Icons != tt.wantIcons {
				t.Errorf("ui = %+v, want theme %s icons %s", cfg.UI, tt.wantTheme, tt.wantIcons)
			}
		})
	}
}

func TestUnknownUIFlagExitsWithUsageCode(t *testing.T) {
	cfgPath, _ := setup(t)
	code, _, stderr := runCLI("--config", cfgPath, "--icons", "emoji", "config")
	if code != 2 {
		t.Errorf("exit code = %d, want 2; stderr %q", code, stderr)
	}
	if !strings.Contains(stderr, `--icons: unknown icon set "emoji" (valid: nerd, unicode, ascii)`) {
		t.Errorf("stderr = %q", stderr)
	}
}
