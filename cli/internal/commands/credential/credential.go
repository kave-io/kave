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
	in := CreateInput{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a credential",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunCreate(cmd.Context(), in)
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Create")
		},
	}
	cmd.Flags().StringVar(&in.EnvID, "env", "", "Environment ID")
	cmd.Flags().StringVar(&in.ConnectorType, "connector-type", "", "Connector type (e.g. openai, github)")
	cmd.Flags().StringVar(&in.Label, "label", "", "Label")
	cmd.Flags().StringVar(&in.Secret, "secret", "", "Raw secret (server encrypts at rest)")
	return cmd
}

func newUpdateCmd() *cobra.Command {
	var label, description, accountID string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in := UpdateInput{ID: args[0]}
			if cmd.Flags().Changed("label") {
				in.Label = &label
			}
			if cmd.Flags().Changed("description") {
				in.Description = &description
			}
			if cmd.Flags().Changed("account-id") {
				in.AccountID = &accountID
			}
			out, err := RunUpdate(cmd.Context(), in)
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Update")
		},
	}
	cmd.Flags().StringVar(&label, "label", "", "Label")
	cmd.Flags().StringVar(&description, "description", "", "Description")
	cmd.Flags().StringVar(&accountID, "account-id", "", "Account ID")
	return cmd
}

func newRotateCmd() *cobra.Command {
	in := RotateInput{}
	cmd := &cobra.Command{
		Use:   "rotate <id>",
		Short: "Rotate a credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in.ID = args[0]
			out, err := RunRotate(cmd.Context(), in)
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Rotate")
		},
	}
	cmd.Flags().StringVar(&in.Secret, "secret", "", "New raw secret")
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
	var reason string
	cmd := &cobra.Command{
		Use:   "revoke <id>",
		Short: "Revoke a credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunRevoke(cmd.Context(), RevokeInput{ID: args[0], Reason: reason})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Revoke")
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "Revocation reason")
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

