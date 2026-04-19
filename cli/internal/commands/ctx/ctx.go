package ctx

import (
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
)


func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "ctx",
		Short: "Named contexts",
		Long:  "Switch between server, user, project, and environment settings.",
	}
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newCurrentCmd())
	cmd.AddCommand(newUseCmd())


	parent.AddCommand(cmd)
}

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunList(cmd.Context(), ListInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "List")
		},
	}
	return cmd
}

func newCurrentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "current",
		Short: "Current",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunCurrent(cmd.Context(), CurrentInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Current")
		},
	}
	return cmd
}

func newUseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use",
		Short: "Use",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunUse(cmd.Context(), UseInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Use")
		},
	}
	return cmd
}

