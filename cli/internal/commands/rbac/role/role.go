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

