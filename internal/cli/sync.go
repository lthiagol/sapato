package cli

import (
	"github.com/spf13/cobra"

	"sapato/internal/ops"
	"sapato/internal/vault"
)

func newSyncCmd(opts *globalOpts) *cobra.Command {
	var prune, force bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Apply the vault's assignments to the agents",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			v, err := vault.Open(cfg.Vault)
			if err != nil {
				return err
			}
			sopts := ops.SyncOptions{
				Options: ops.Options{Force: force, DryRun: opts.dryRun},
				Prune:   prune,
			}
			actions, err := ops.Sync(v, cfg.Agents, sopts)
			if err != nil {
				return err
			}
			return printActions(cmd.OutOrStdout(), actions, opts.dryRun)
		},
	}
	cmd.Flags().BoolVar(&prune, "prune", false, "uninstall managed skills that the assignments no longer name")
	cmd.Flags().BoolVar(&force, "force", false, "also overwrite unmanaged or modified skill folders, and remove modified ones")
	return cmd
}
