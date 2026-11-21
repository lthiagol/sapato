package cli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"sapato/internal/ops"
	"sapato/internal/vault"
)

func newExportCmd(opts *globalOpts) *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "export <skill>...",
		Short: "Write skills to a .skill.tar.gz tarball",
		Args:  usageArgs(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			v, err := vault.Open(cfg.Vault)
			if err != nil {
				return err
			}
			name, err := ops.ExportName(v, args, time.Now())
			if err != nil {
				return err
			}
			if output != "" {
				name = output
			}
			out := cmd.OutOrStdout()
			if opts.dryRun {
				_, err := fmt.Fprintf(out, "would write %s\n", name)
				return err
			}
			return writeExport(out, v, args, name)
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "file to write (default: named after the skills)")
	return cmd
}

// writeExport writes the export of skills to a new file at name. An existing file
// is refused, and a partly written file is removed.
func writeExport(out io.Writer, v *vault.Vault, skills []string, name string) error {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("export %s: %w", name, err)
	}
	skips, err := ops.Export(v, skills, f)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(name)
		return err
	}
	for _, s := range skips {
		if _, err := fmt.Fprintf(out, "skip %s: %s\n", s.Skill, s.Reason); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(out, "wrote %s\n", name)
	return err
}
