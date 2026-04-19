package rbac

import (
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/kave-io/kave/cli/internal/commands/rbac/role"
)


func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "rbac",
		Short: "Role-based access control",
		Long:  "Users, roles, and permissions on the daemon.",
	}
	cmd.AddCommand(newGrantCmd())
	cmd.AddCommand(newRevokeCmd())
	cmd.AddCommand(newListCmd())

	role.Register(cmd)

	parent.AddCommand(cmd)
}

func newGrantCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "grant",
		Short: "Grant",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunGrant(cmd.Context(), GrantInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Grant")
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

