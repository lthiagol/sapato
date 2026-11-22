package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"sapato/internal/config"
	"sapato/internal/ops"
	"sapato/internal/vault"
)

// agentFlags holds the -a/--agent and --all flags of the mutating commands.
type agentFlags struct {
	ids []string
	all bool
}

func (f *agentFlags) add(cmd *cobra.Command) {
	cmd.Flags().StringArrayVarP(&f.ids, "agent", "a", nil, "agent to act on, repeatable")
	cmd.Flags().BoolVar(&f.all, "all", false, "act on all configured agents")
}

// resolve returns the agents the flags name. Exactly one of -a and --all is required.
func (f agentFlags) resolve(agents []config.Agent) ([]config.Agent, error) {
	if !f.all && len(f.ids) == 0 {
		return nil, usageError{errors.New("choose agents with --agent or --all")}
	}
	return f.resolveDefault(agents)
}

// resolveDefault is resolve, except that neither flag selects every configured agent.
func (f agentFlags) resolveDefault(agents []config.Agent) ([]config.Agent, error) {
	if f.all && len(f.ids) > 0 {
		return nil, usageError{errors.New("use either --agent or --all, not both")}
	}
	selected, err := ops.SelectAgents(agents, f.ids)
	if err != nil {
		return nil, usageError{err}
	}
	return selected, nil
}

func newInstallCmd(opts *globalOpts) *cobra.Command {
	var agents agentFlags
	var force bool
	cmd := &cobra.Command{
		Use:   "install <skill...>",
		Short: "Install vault skills into agents",
		Args:  usageArgs(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			selected, err := agents.resolve(cfg.Agents)
			if err != nil {
				return err
			}
			v, err := vault.Open(cfg.Vault)
			if err != nil {
				return err
			}
			actions, err := ops.Install(v, selected, args, ops.Options{
				Force:            force,
				DryRun:           opts.dryRun,
				SkipIncompatible: agents.all,
			})
			if err != nil {
				return err
			}
			return printActions(cmd.OutOrStdout(), actions, opts.dryRun)
		},
	}
	agents.add(cmd)
	cmd.Flags().BoolVar(&force, "force", false, "overwrite unmanaged or modified skill folders")
	return cmd
}

func newUpdateCmd(opts *globalOpts) *cobra.Command {
	var agents agentFlags
	var force bool
	cmd := &cobra.Command{
		Use:   "update [skill...]",
		Short: "Re-install outdated skills (default: all skills in configured agents)",
		Args:  usageArgs(cobra.ArbitraryArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			selected, err := agents.resolveDefault(cfg.Agents)
			if err != nil {
				return err
			}
			v, err := vault.Open(cfg.Vault)
			if err != nil {
				return err
			}
			actions, err := ops.Update(v, selected, args, ops.Options{Force: force, DryRun: opts.dryRun})
			if err != nil {
				return err
			}
			return printActions(cmd.OutOrStdout(), actions, opts.dryRun)
		},
	}
	agents.add(cmd)
	cmd.Flags().BoolVar(&force, "force", false, "also re-install modified skill folders")
	return cmd
}

func newUninstallCmd(opts *globalOpts) *cobra.Command {
	var agents agentFlags
	var force bool
	cmd := &cobra.Command{
		Use:   "uninstall <skill...>",
		Short: "Remove managed skills from agents",
		Args:  usageArgs(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			selected, err := agents.resolve(cfg.Agents)
			if err != nil {
				return err
			}
			v, err := vault.Open(cfg.Vault)
			if err != nil {
				return err
			}
			actions, err := ops.Uninstall(v, selected, args, ops.Options{Force: force, DryRun: opts.dryRun})
			if err != nil {
				return err
			}
			return printActions(cmd.OutOrStdout(), actions, opts.dryRun)
		},
	}
	agents.add(cmd)
	cmd.Flags().BoolVar(&force, "force", false, "remove modified skill folders")
	return cmd
}

// printActions prints one line per action. A dry run says "would" before the verb.
// Actions that name no agent, such as import, print without the agent.
func printActions(w io.Writer, actions []ops.Action, dryRun bool) error {
	if len(actions) == 0 {
		_, err := fmt.Fprintln(w, "nothing to do")
		return err
	}
	for _, a := range actions {
		if a.Op == ops.OpSkip {
			if _, err := fmt.Fprintf(w, "skip %s in %s: %s\n", a.Skill, a.Agent, a.Reason); err != nil {
				return err
			}
			continue
		}
		verb := a.Op
		if dryRun {
			verb = "would " + verb
		}
		where := ""
		if a.Agent != "" {
			where = " in " + a.Agent
		}
		if _, err := fmt.Fprintf(w, "%s %s%s: %s\n", verb, a.Skill, where, a.Path); err != nil {
			return err
		}
	}
	return nil
}
