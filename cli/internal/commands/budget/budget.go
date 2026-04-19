package budget

import (
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
)


func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "budget",
		Short: "Spend caps and reports",
		Long:  "Manage agent and policy budgets.",
	}
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newSetCmd())
	cmd.AddCommand(newReportCmd())


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

func newReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Report",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunReport(cmd.Context(), ReportInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Report")
		},
	}
	return cmd
}

