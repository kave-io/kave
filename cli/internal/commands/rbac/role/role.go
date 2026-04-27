package role

import (
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
)


func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "role",
		Short: "Role management",
		Long:  "Create and manage roles.",
	}
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newDeleteCmd())


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
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunGet(cmd.Context(), GetInput{ID: id})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Get")
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "role ID")
	return cmd
}

func newCreateCmd() *cobra.Command {
	var name string
	var permissions []string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunCreate(cmd.Context(), CreateInput{Name: name, Permissions: permissions})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Create")
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "role name")
	cmd.Flags().StringSliceVar(&permissions, "permissions", nil, "permissions")
	return cmd
}

func newDeleteCmd() *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunDelete(cmd.Context(), DeleteInput{ID: id})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Delete")
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "role ID")
	return cmd
}

