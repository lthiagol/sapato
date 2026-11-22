package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"sapato/internal/ops"
	"sapato/internal/vault"
)

func newGuideCmd(opts *globalOpts) *cobra.Command {
	var install, force bool
	cmd := &cobra.Command{
		Use:   "guide",
		Short: "Print a SKILL.md that teaches an agent to work with this vault",
		Long:  "Print the generated SKILL.md for this vault and its configured agents. --install writes it to the vault as the skill sapato, and refuses a different existing copy unless --force is given. Run --install again after changing the config.",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if force && !install {
				return usageError{errors.New("--force needs --install")}
			}
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			vaultPath, err := filepath.Abs(cfg.Vault)
			if err != nil {
				return fmt.Errorf("resolve vault %s: %w", cfg.Vault, err)
			}
			if !install {
				_, err = cmd.OutOrStdout().Write(ops.Guide(vaultPath, cfg.Agents))
				return err
			}
			v, err := vault.Open(vaultPath)
			if err != nil {
				return err
			}
			action, err := ops.GuideInstall(v, cfg.Agents, ops.Options{Force: force, DryRun: opts.dryRun})
			if err != nil {
				return err
			}
			return printActions(cmd.OutOrStdout(), []ops.Action{action}, opts.dryRun)
		},
	}
	cmd.Flags().BoolVar(&install, "install", false, "write the guide to the vault as the skill sapato")
	cmd.Flags().BoolVar(&force, "force", false, "replace a sapato skill with different content (with --install)")
	return cmd
}
