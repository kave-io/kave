package store

import (
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
)


func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "store",
		Short: "Store management",
		Long:  "Database and store operations.",
	}
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newMigrateCmd())
	cmd.AddCommand(newTestCmd())
	cmd.AddCommand(newStatusCmd())


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

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunMigrate(cmd.Context(), MigrateInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Migrate")
		},
	}
	return cmd
}

func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunTest(cmd.Context(), TestInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Test")
		},
	}
	return cmd
}

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Status",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunStatus(cmd.Context(), StatusInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Status")
		},
	}
	return cmd
}

