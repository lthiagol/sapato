package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"sapato/internal/ops"
	"sapato/internal/vault"
)

func newAdoptCmd(opts *globalOpts) *cobra.Command {
	var agentID string
	var force bool
	cmd := &cobra.Command{
		Use:   "adopt <skill>",
		Short: "Copy an unmanaged agent skill into the vault and manage it",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			if agentID == "" {
				return usageError{errors.New("choose an agent with --agent")}
			}
			selected, err := ops.SelectAgents(cfg.Agents, []string{agentID})
			if err != nil {
				return usageError{err}
			}
			v, err := vault.Open(cfg.Vault)
			if err != nil {
				return err
			}
			action, err := ops.Adopt(v, selected[0], args[0], ops.Options{Force: force, DryRun: opts.dryRun})
			if err != nil {
				return err
			}
			return printActions(cmd.OutOrStdout(), []ops.Action{action}, opts.dryRun)
		},
	}
	cmd.Flags().StringVarP(&agentID, "agent", "a", "", "agent whose unmanaged skill to adopt, required")
	cmd.Flags().BoolVar(&force, "force", false, "replace a skill of the same name in the vault")
	return cmd
}
