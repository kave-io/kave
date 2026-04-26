package policy

import (
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
)


func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Manage policies",
		Long:  "Policies define auth, cost, validation, and tracing rules.",
	}
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newExportCmd())
	cmd.AddCommand(newDeleteCmd())
	cmd.AddCommand(newValidateCmd())
	cmd.AddCommand(newTestCmd())


	parent.AddCommand(cmd)
}

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List policies",
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
		Use:   "get <id>",
		Short: "Get a policy by ID",
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

func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunExport(cmd.Context(), ExportInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Export")
		},
	}
	return cmd
}

func newDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunDelete(cmd.Context(), DeleteInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Delete")
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

