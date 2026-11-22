package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"sapato/internal/ops"
	"sapato/internal/vault"
)

func newCompatCmd(opts *globalOpts) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "compat <skill> [agent...]",
		Short: "Show or set the agents a vault skill is compatible with",
		Long:  "With no agents, print the skill's compatibility. With agents, set it to exactly those agents. With --all, it is compatible with every agent.",
		Args:  usageArgs(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			skill, agents := args[0], args[1:]
			if all && len(agents) > 0 {
				return usageError{errors.New("use either agents or --all, not both")}
			}
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			v, err := vault.Open(cfg.Vault)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if !all && len(agents) == 0 {
				list, err := ops.Compat(v, skill)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(out, "%s: %s\n", skill, describeCompat(list))
				return err
			}
			var list []string
			if !all {
				list = agents
			}
			if err := ops.SetCompat(v, skill, list, ops.Options{DryRun: opts.dryRun}); err != nil {
				return err
			}
			verb := "compat"
			if opts.dryRun {
				verb = "would compat"
			}
			_, err = fmt.Fprintf(out, "%s %s: %s\n", verb, skill, describeCompat(list))
			return err
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "compatible with every agent (removes the compatible list)")
	return cmd
}

// describeCompat returns the compatibility of a skill for printing: its agents, or
// "all agents" when list is nil.
func describeCompat(list []string) string {
	if list == nil {
		return "all agents"
	}
	return strings.Join(list, ", ")
}
