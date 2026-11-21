package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"sapato/internal/config"
	"sapato/internal/ops"
	"sapato/internal/vault"
)

func newConfigCmd(opts *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Print the resolved config",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "vault: %s\n\n", cfg.Vault)
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "AGENT\tENABLED\tPATH")
			for _, a := range cfg.Agents {
				fmt.Fprintf(w, "%s\t%s\t%s\n", a.ID, yesNo(a.Enabled), a.Path)
			}
			return w.Flush()
		},
	}
}

func newInitCmd(opts *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "init [path]",
		Short: "Create a vault at path, or at the configured vault",
		Args:  usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			path := cfg.Vault
			if len(args) == 1 {
				path = args[0]
			}
			out := cmd.OutOrStdout()
			if opts.dryRun {
				exists, err := vault.Exists(path)
				if err != nil {
					return err
				}
				if exists {
					return fmt.Errorf("init vault %s: vault already exists", path)
				}
				fmt.Fprintf(out, "would create vault at %s\n", path)
				return nil
			}
			if err := vault.Init(path); err != nil {
				return err
			}
			fmt.Fprintf(out, "created vault at %s\n", path)
			return nil
		},
	}
}

func newListCmd(opts *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List vault skills",
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
			skills, err := v.Skills()
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tVERSION\tHASH\tSTATUS")
			for _, s := range skills {
				status := "valid"
				if !s.Valid {
					status = "invalid: " + strings.Join(s.Problems, "; ")
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Name, orDash(s.Version), shortHash(s.Hash), status)
			}
			return w.Flush()
		},
	}
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// shortHash keeps the algorithm prefix and the first 12 hex digits.
func shortHash(h string) string {
	algo, hex, _ := strings.Cut(h, ":")
	if len(hex) > 12 {
		hex = hex[:12]
	}
	return algo + ":" + hex
}

func newStatusCmd(opts *globalOpts) *cobra.Command {
	var agentIDs []string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status [skill...]",
		Short: "Show each skill's status in each agent",
		Args:  usageArgs(cobra.ArbitraryArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			agents, err := ops.SelectAgents(cfg.Agents, agentIDs)
			if err != nil {
				return usageError{err}
			}
			v, err := vault.Open(cfg.Vault)
			if err != nil {
				return err
			}
			rows, err := ops.Status(v, agents, ops.Filter{Skills: args})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}
			return printStatusTable(out, agents, rows)
		},
	}
	cmd.Flags().StringArrayVarP(&agentIDs, "agent", "a", nil, "agent to show, repeatable (default: enabled agents)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print rows as JSON")
	return cmd
}

// printStatusTable prints skills as rows and agents as columns. A skill with
// no row for an agent gets "-".
func printStatusTable(w io.Writer, agents []config.Agent, rows []ops.Row) error {
	var skills []string
	cells := make(map[string]map[string]ops.State)
	for _, r := range rows {
		if _, ok := cells[r.Skill]; !ok {
			cells[r.Skill] = make(map[string]ops.State)
			skills = append(skills, r.Skill)
		}
		cells[r.Skill][r.Agent] = r.Status
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	header := []string{"SKILL"}
	for _, a := range agents {
		header = append(header, strings.ToUpper(a.ID))
	}
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, skill := range skills {
		line := []string{skill}
		for _, a := range agents {
			line = append(line, orDash(string(cells[skill][a.ID])))
		}
		fmt.Fprintln(tw, strings.Join(line, "\t"))
	}
	return tw.Flush()
}
