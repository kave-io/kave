package credential

import (
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
)


func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "credential",
		Short: "Manage credentials",
		Long:  "Outbound secrets that agents use to call external services.",
	}
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newRotateCmd())
	cmd.AddCommand(newTestCmd())
	cmd.AddCommand(newRevokeCmd())
	cmd.AddCommand(newDeleteCmd())


	parent.AddCommand(cmd)
}

func newListCmd() *cobra.Command {
	var connectorType string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunList(cmd.Context(), ListInput{ConnectorType: connectorType})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "List")
		},
	}
	cmd.Flags().StringVar(&connectorType, "connector", "", "Filter by connector type")
	return cmd
}

func newGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a credential by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunGet(cmd.Context(), GetInput{ID: args[0]})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Get")
		},
	}
	return cmd
}

func newCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunCreate(cmd.Context(), CreateInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Create")
		},
	}
	return cmd
}

func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunUpdate(cmd.Context(), UpdateInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Update")
		},
	}
	return cmd
}

func newRotateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rotate",
		Short: "Rotate",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunRotate(cmd.Context(), RotateInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Rotate")
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

func newRevokeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunRevoke(cmd.Context(), RevokeInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Revoke")
		},
	}
	return cmd
}

func newDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunDelete(cmd.Context(), DeleteInput{ID: args[0]})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Delete")
		},
	}
	return cmd
}

