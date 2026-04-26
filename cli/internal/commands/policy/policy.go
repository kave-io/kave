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
	in := CreateInput{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a policy",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunCreate(cmd.Context(), in)
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Create")
		},
	}
	cmd.Flags().StringVar(&in.EnvID, "env", "", "Environment ID")
	cmd.Flags().StringVar(&in.Name, "name", "", "Policy name")
	cmd.Flags().StringVar(&in.Description, "description", "", "Description")
	cmd.Flags().StringVar(&in.Mode, "mode", "", "Mode: enforce | shadow")
	return cmd
}

func newUpdateCmd() *cobra.Command {
	in := UpdateInput{}
	var description string
	var setDescription bool
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a policy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in.ID = args[0]
			if setDescription {
				in.Description = &description
			}
			out, err := RunUpdate(cmd.Context(), in)
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Update")
		},
	}
	cmd.Flags().StringSliceVar(&in.AllowedTypes, "allowed-types", nil, "Replace allowed action types")
	cmd.Flags().StringSliceVar(&in.AllowedConnectors, "allowed-connectors", nil, "Replace allowed connectors")
	cmd.Flags().StringSliceVar(&in.AllowedMethods, "allowed-methods", nil, "Replace allowed methods")
	cmd.Flags().StringVar(&description, "description", "", "Description")
	cmd.PreRunE = func(c *cobra.Command, _ []string) error {
		setDescription = c.Flags().Changed("description")
		return nil
	}
	return cmd
}

func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export <id>",
		Short: "Export a policy as YAML",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunExport(cmd.Context(), ExportInput{ID: args[0]})
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
		Use:   "delete <id>",
		Short: "Delete a policy",
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

func newValidateCmd() *cobra.Command {
	in := ValidateInput{}
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a policy YAML document",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunValidate(cmd.Context(), in)
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Validate")
		},
	}
	cmd.Flags().StringVar(&in.File, "file", "", "Path to a policy YAML file")
	cmd.Flags().StringVar(&in.YAML, "yaml", "", "Inline policy YAML")
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

