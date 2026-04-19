package auth

import (
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/kave-io/kave/cli/internal/commands/auth/token"
)


func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "User authentication",
		Long:  "User session management and API tokens.",
	}
	cmd.AddCommand(newWhoamiCmd())
	cmd.AddCommand(newLoginCmd())
	cmd.AddCommand(newLogoutCmd())

	token.Register(cmd)

	parent.AddCommand(cmd)
}

func newWhoamiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Whoami",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunWhoami(cmd.Context(), WhoamiInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Whoami")
		},
	}
	return cmd
}

func newLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunLogin(cmd.Context(), LoginInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Login")
		},
	}
	return cmd
}

func newLogoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Logout",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunLogout(cmd.Context(), LogoutInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Logout")
		},
	}
	return cmd
}

