package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"sapato/internal/config"
	"sapato/internal/ops"
	"sapato/internal/vault"
)

// runEditor runs an editor command line and waits for it to exit. Tests replace it.
var runEditor = func(argv []string) error {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func newNewCmd(opts *globalOpts) *cobra.Command {
	var force, edit bool
	var name, description string
	var compatible []string
	cmd := &cobra.Command{
		Use:   "new [file]",
		Short: "Create a vault skill from a complete SKILL.md, or scaffold one with --name",
		Long:  "Read a complete SKILL.md from file or stdin, or with --name and --description write the template for a new skill. --compatible sets the agents it works with, and --edit opens the new SKILL.md in $VISUAL, $EDITOR or vi.",
		Args:  usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			hasName, hasDesc, hasCompat := flags.Changed("name"), flags.Changed("description"), flags.Changed("compatible")
			switch {
			case len(args) > 0 && (hasName || hasDesc || hasCompat || edit):
				return usageError{errors.New("new takes a file or --name, not both")}
			case hasName != hasDesc:
				return usageError{errors.New("--name and --description go together")}
			case !hasName && (hasCompat || edit):
				return usageError{errors.New("--compatible and --edit need --name")}
			case hasName:
				a := scaffoldArgs{name: name, description: description, force: force, edit: edit}
				if hasCompat {
					a.compatible = append([]string{}, compatible...)
				}
				return scaffold(cmd, opts, a)
			}
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
	cmd.Flags().StringVar(&name, "name", "", "scaffold a skill with this name from the built-in template")
	cmd.Flags().StringVar(&description, "description", "", "description of the scaffolded skill (with --name)")
	cmd.Flags().StringSliceVar(&compatible, "compatible", nil, "agents the scaffolded skill works with, comma-separated (default: all)")
	cmd.Flags().BoolVar(&edit, "edit", false, "open the scaffolded SKILL.md in $VISUAL, $EDITOR or vi (with --name)")
	return cmd
}

// scaffoldArgs are the inputs of new --name. compatible is nil when --compatible is
// not given.
type scaffoldArgs struct {
	name, description string
	compatible        []string
	force, edit       bool
}

// scaffold writes the template of a new skill and, with --edit, opens it in the editor.
// A dry run prints the plan and does not open the editor.
func scaffold(cmd *cobra.Command, opts *globalOpts, a scaffoldArgs) error {
	cfg, err := loadConfig(opts)
	if err != nil {
		return err
	}
	v, err := vault.Open(cfg.Vault)
	if err != nil {
		return err
	}
	var list []string
	if a.compatible != nil {
		list = ops.NormalizeCompat(a.compatible, configuredIDs(cfg))
	}
	action, err := ops.Scaffold(v, a.name, a.description, list, ops.Options{Force: a.force, DryRun: opts.dryRun})
	if err != nil {
		return err
	}
	if err := printActions(cmd.OutOrStdout(), []ops.Action{action}, opts.dryRun); err != nil {
		return err
	}
	if !a.edit || opts.dryRun {
		return nil
	}
	file := filepath.Join(action.Path, "SKILL.md")
	if err := runEditor(ops.EditorCommand(os.Getenv("VISUAL"), os.Getenv("EDITOR"), file)); err != nil {
		return fmt.Errorf("edit %s: %w", file, err)
	}
	return nil
}

// configuredIDs are the IDs of the configured agents, in order.
func configuredIDs(cfg config.Config) []string {
	ids := make([]string, 0, len(cfg.Agents))
	for _, a := range cfg.Agents {
		ids = append(ids, a.ID)
	}
	return ids
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
