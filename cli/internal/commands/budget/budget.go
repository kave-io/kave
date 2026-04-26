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
	in := GetInput{}
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a budget",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunGet(cmd.Context(), in)
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Get")
		},
	}
	cmd.Flags().StringVar(&in.AgentID, "agent", "", "Agent ID")
	return cmd
}

func newSetCmd() *cobra.Command {
	in := SetInput{}
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set a budget",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunSet(cmd.Context(), in)
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Set")
		},
	}
	cmd.Flags().StringVar(&in.AgentID, "agent", "", "Agent ID")
	cmd.Flags().StringVar(&in.HardCap, "hard-cap", "", "Hard cap (format: currency,amount)")
	cmd.Flags().StringVar(&in.SoftCap, "soft-cap", "", "Soft cap (format: currency,amount)")
	cmd.Flags().StringVar(&in.Period, "period", "", "Billing period (daily|monthly|yearly)")
	return cmd
}

func newReportCmd() *cobra.Command {
	in := ReportInput{}
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Spend report",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunReport(cmd.Context(), in)
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Report")
		},
	}
	cmd.Flags().StringVar(&in.ProjectID, "project", "", "Project ID")
	cmd.Flags().StringVar(&in.EnvID, "env", "", "Environment ID")
	cmd.Flags().StringVar(&in.PolicyID, "policy", "", "Policy ID")
	return cmd
}

