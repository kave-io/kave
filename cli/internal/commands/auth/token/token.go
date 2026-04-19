package token

import (
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
)


func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "User API tokens",
		Long:  "Long-lived tokens for scripts and CI.",
	}
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newRevokeCmd())


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

