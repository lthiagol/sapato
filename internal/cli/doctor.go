package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"sapato/internal/ops"
	"sapato/internal/vault"
)

func newDoctorCmd(opts *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Report problems in the vault and the agents' skill dirs",
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
			findings, err := ops.Doctor(v, cfg.Agents)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(findings) == 0 {
				_, err := fmt.Fprintln(out, "no problems found")
				return err
			}
			errCount := 0
			for _, f := range findings {
				if f.Severity == ops.SeverityError {
					errCount++
				}
				if _, err := fmt.Fprintf(out, "%s: %s\n", f.Severity, f.Message); err != nil {
					return err
				}
			}
			if errCount > 0 {
				return fmt.Errorf("doctor found %d error(s)", errCount)
			}
			return nil
		},
	}
}
