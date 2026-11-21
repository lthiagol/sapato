package tui

import (
	"sapato/internal/config"
	"sapato/internal/tui/icons"
	"sapato/internal/tui/theme"
)

// StarterConfig is the commented config that config init writes. It lists the
// themes and icon sets of this build.
func StarterConfig() string {
	return config.Starter(theme.Names(), icons.Names())
}

// WriteStarterConfig writes StarterConfig to path, as config init does. An
// existing file is an error unless force is set.
func WriteStarterConfig(path string, force bool) error {
	return config.WriteStarter(path, StarterConfig(), force)
}
