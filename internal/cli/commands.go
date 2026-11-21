package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

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
