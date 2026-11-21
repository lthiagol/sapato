package cli

import (
	"github.com/spf13/cobra"

	"sapato/internal/ops"
	"sapato/internal/vault"
)

func newImportCmd(opts *globalOpts) *cobra.Command {
	var name, subdir string
	var force bool
	cmd := &cobra.Command{
		Use:   "import <path-or-git-url>",
		Short: "Copy a skill from a folder or git repository into the vault",
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
			iopts := ops.ImportOptions{
				Options: ops.Options{Force: force, DryRun: opts.dryRun},
				Name:    name,
				Subdir:  subdir,
			}
			action, err := ops.Import(v, args[0], iopts)
			if err != nil {
				return err
			}
			return printActions(cmd.OutOrStdout(), []ops.Action{action}, opts.dryRun)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "skill name, which must match the frontmatter name")
	cmd.Flags().StringVar(&subdir, "subdir", "", "folder of the source that holds the skill")
	cmd.Flags().BoolVar(&force, "force", false, "replace a skill of the same name in the vault")
	return cmd
}
