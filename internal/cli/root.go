// Package cli wires sapato's commands to the lower packages. It parses flags,
// calls into them and prints results.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"sapato/internal/config"
)

// usageError marks a command-line usage mistake, which exits with code 2.
type usageError struct {
	err error
}

func (e usageError) Error() string { return e.err.Error() }

func (e usageError) Unwrap() error { return e.err }

type globalOpts struct {
	configPath string
	dryRun     bool
	theme      string
	icons      string
}

// Run executes sapato with args and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	root := newRootCmd(stdout, stderr)
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		return 0
	}
	fmt.Fprintf(stderr, "error: %v\n", err)
	var ue usageError
	if errors.As(err, &ue) {
		fmt.Fprintln(stderr, "Run 'sapato --help' for usage.")
		return 2
	}
	return 1
}

func newRootCmd(stdout, stderr io.Writer) *cobra.Command {
	var opts globalOpts
	root := &cobra.Command{
		Use:           "sapato",
		Short:         "Manage a portable skill vault and install skills into coding agents",
		Long:          "Run with no subcommand to open the interactive UI. The global --dry-run flag has no effect there.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          usageArgs(cobra.NoArgs),
		RunE:          func(cmd *cobra.Command, _ []string) error { return runTUI(cmd, &opts) },
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err} })
	root.PersistentFlags().StringVar(&opts.configPath, "config", "", "config file (default ~/.config/sapato/config.toml)")
	root.PersistentFlags().BoolVar(&opts.dryRun, "dry-run", false, "print the plan of mutating commands and change nothing")
	root.PersistentFlags().StringVar(&opts.theme, "theme", "", "UI theme, overrides [ui] theme: "+strings.Join(config.ThemeNames(), ", "))
	root.PersistentFlags().StringVar(&opts.icons, "icons", "", "UI icon set, overrides [ui] icons: "+strings.Join(config.IconNames(), ", "))
	root.AddCommand(newConfigCmd(&opts), newInitCmd(&opts), newListCmd(&opts), newStatusCmd(&opts),
		newInstallCmd(&opts), newUninstallCmd(&opts), newUpdateCmd(&opts), newDiffCmd(&opts),
		newImportCmd(&opts), newAdoptCmd(&opts), newRemoveCmd(&opts), newSyncCmd(&opts), newDoctorCmd(&opts),
		newNewCmd(&opts), newExportCmd(&opts), newCompatCmd(&opts), newTUICmd(&opts))
	return root
}

// usageArgs marks argument validation failures as usage errors.
func usageArgs(v cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := v(cmd, args); err != nil {
			return usageError{err}
		}
		return nil
	}
}

// loadConfig reads the config and applies the --theme and --icons flags over
// its [ui] section. A flag value that is not a known name is a usage error.
func loadConfig(opts *globalOpts) (config.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return config.Config{}, fmt.Errorf("find home dir: %w", err)
	}
	cfg, err := config.Load(opts.configPath, home)
	if err != nil {
		return config.Config{}, err
	}
	if opts.theme != "" {
		if err := config.ValidateTheme(opts.theme); err != nil {
			return config.Config{}, usageError{fmt.Errorf("--theme: %w", err)}
		}
		cfg.UI.Theme = opts.theme
	}
	if opts.icons != "" {
		if err := config.ValidateIcons(opts.icons); err != nil {
			return config.Config{}, usageError{fmt.Errorf("--icons: %w", err)}
		}
		cfg.UI.Icons = opts.icons
	}
	return cfg, nil
}
