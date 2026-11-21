package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"sapato/internal/ops"
	"sapato/internal/vault"
)

func newDiffCmd(opts *globalOpts) *cobra.Command {
	var agentID string
	cmd := &cobra.Command{
		Use:   "diff <skill>",
		Short: "Show how an installed skill differs from the vault",
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
			changes, err := ops.Diff(v, selected[0], args[0])
			if err != nil {
				return err
			}
			return printDiff(cmd.OutOrStdout(), changes)
		},
	}
	cmd.Flags().StringVarP(&agentID, "agent", "a", "", "agent whose installed copy to compare, required")
	return cmd
}

// printDiff lists each changed file, then the unified text diffs.
func printDiff(w io.Writer, changes []ops.FileChange) error {
	if len(changes) == 0 {
		_, err := fmt.Fprintln(w, "no differences")
		return err
	}
	for _, c := range changes {
		note := ""
		switch {
		case c.Binary:
			note = " (binary differs)"
		case c.Mode:
			note = " (mode differs)"
		}
		if _, err := fmt.Fprintf(w, "%s %s%s\n", c.Kind, c.Rel, note); err != nil {
			return err
		}
	}
	for _, c := range changes {
		if c.Diff == "" {
			continue
		}
		if _, err := fmt.Fprintf(w, "\n%s", c.Diff); err != nil {
			return err
		}
	}
	return nil
}
