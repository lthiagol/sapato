package cli

import (
	"github.com/spf13/cobra"

	"sapato/internal/tui"
)

func newTUICmd(opts *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Open the interactive UI",
		Long:  "Open the interactive UI on the configured vault. The global --dry-run flag has no effect here.",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTUI(cmd, opts)
		},
	}
}

// runTUI opens the interactive UI on the configured vault.
func runTUI(cmd *cobra.Command, opts *globalOpts) error {
	cfg, err := loadConfig(opts)
	if err != nil {
		return err
	}
	return tui.Run(cfg, cmd.InOrStdin(), cmd.OutOrStdout())
}
