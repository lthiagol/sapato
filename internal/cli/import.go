package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"sapato/internal/ops"
	"sapato/internal/vault"
)

func newImportCmd(opts *globalOpts) *cobra.Command {
	var name, subdir, description string
	var force, all, fix bool
	cmd := &cobra.Command{
		Use:   "import <path-or-url>",
		Short: "Copy skills from a folder, SKILL.md file, URL, git repository or tarball into the vault",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			if all && name != "" {
				return usageError{errors.New("--name does not apply with --all")}
			}
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			v, err := vault.Open(cfg.Vault)
			if err != nil {
				return err
			}
			iopts := ops.ImportOptions{
				Options:     ops.Options{Force: force, DryRun: opts.dryRun},
				Name:        name,
				Subdir:      subdir,
				All:         all,
				Fix:         fix,
				Description: description,
			}
			actions, err := ops.Import(v, args[0], iopts)
			if err != nil {
				return err
			}
			return printActions(cmd.OutOrStdout(), actions, opts.dryRun)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "skill name, which must match the frontmatter name (with --fix: the name to repair to; tarball: single-skill archives only)")
	cmd.Flags().StringVar(&subdir, "subdir", "", "folder of the source that holds the skill")
	cmd.Flags().BoolVar(&all, "all", false, "import every skill the source holds, all or nothing")
	cmd.Flags().BoolVar(&fix, "fix", false, "repair the name and description of an invalid skill in the vault copy")
	cmd.Flags().StringVar(&description, "description", "", "description for a repaired skill that has none (with --fix)")
	cmd.Flags().BoolVar(&force, "force", false, "replace a skill of the same name in the vault")
	return cmd
}
