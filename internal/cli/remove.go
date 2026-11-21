package cli

import (
	"github.com/spf13/cobra"

	"sapato/internal/ops"
	"sapato/internal/vault"
)

func newRemoveCmd(opts *globalOpts) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "remove <skill>",
		Short: "Delete a skill from the vault",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			v, err := vault.Open(cfg.Vault)
			if err != nil {
				return err
			}
			action, err := ops.Remove(v, cfg.Agents, args[0], ops.Options{Force: force, DryRun: opts.dryRun})
			if err != nil {
				return err
			}
			return printActions(cmd.OutOrStdout(), []ops.Action{action}, opts.dryRun)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "remove a skill that is installed in an agent")
	return cmd
}
