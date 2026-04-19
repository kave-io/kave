package config

import (
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
)


func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Daemon and CLI config",
		Long:  "View, edit, and manage kave.yaml configuration.",
	}
	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newViewCmd())
	cmd.AddCommand(newEditCmd())
	cmd.AddCommand(newPathCmd())
	cmd.AddCommand(newReloadCmd())
	cmd.AddCommand(newDiffCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newSetCmd())
	cmd.AddCommand(newValidateCmd())


	parent.AddCommand(cmd)
}

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Init",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunInit(cmd.Context(), InitInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Init")
		},
	}
	return cmd
}

func newViewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view",
		Short: "View",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunView(cmd.Context(), ViewInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "View")
		},
	}
	return cmd
}

func newEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunEdit(cmd.Context(), EditInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Edit")
		},
	}
	return cmd
}

func newPathCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "path",
		Short: "Path",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunPath(cmd.Context(), PathInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Path")
		},
	}
	return cmd
}

func newReloadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reload",
		Short: "Reload",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunReload(cmd.Context(), ReloadInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Reload")
		},
	}
	return cmd
}

func newDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Diff",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunDiff(cmd.Context(), DiffInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Diff")
		},
	}
	return cmd
}

func newGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunGet(cmd.Context(), GetInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Get")
		},
	}
	return cmd
}

func newSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunSet(cmd.Context(), SetInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Set")
		},
	}
	return cmd
}

func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunValidate(cmd.Context(), ValidateInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Validate")
		},
	}
	return cmd
}

