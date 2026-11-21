package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"sapato/internal/ops"
	"sapato/internal/vault"
)

func newNewCmd(opts *globalOpts) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "new [file]",
		Short: "Create a vault skill from a complete SKILL.md, read from file or stdin",
		Args:  usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			content, err := readSkillInput(cmd, args)
			if err != nil {
				return err
			}
			v, err := vault.Open(cfg.Vault)
			if err != nil {
				return err
			}
			action, err := ops.Create(v, content, ops.Options{Force: force, DryRun: opts.dryRun})
			if err != nil {
				return err
			}
			return printActions(cmd.OutOrStdout(), []ops.Action{action}, opts.dryRun)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "replace a skill of the same name in the vault")
	return cmd
}

// readSkillInput reads the file named by args[0], or stdin when there is no argument
// or the argument is "-".
func readSkillInput(cmd *cobra.Command, args []string) ([]byte, error) {
	if len(args) == 0 || args[0] == "-" {
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return data, nil
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", args[0], err)
	}
	return data, nil
}
