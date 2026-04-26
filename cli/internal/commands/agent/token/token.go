package token

import (
	"github.com/kave-io/kave/cli/internal/flags"
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
)

func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Agent authentication tokens",
		Long:  "Issue, list, and revoke agent → daemon auth tokens.",
	}
	cmd.AddCommand(
		newIssueCmd(),
		newListCmd(),
		newRevokeCmd(),
	)
	parent.AddCommand(cmd)
}

func newIssueCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue <agent>",
		Short: "Issue a new token",
		Long:  "Prints the raw token exactly once; never recoverable after the command returns.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunIssue(cmd.Context(), IssueInput{Agent: args[0]})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Token")
		},
	}
	return cmd
}

func newListCmd() *cobra.Command {
	var in ListInput
	cmd := &cobra.Command{
		Use:   "list <agent>",
		Short: "List tokens for an agent",
		Long:  "Lists all non-revoked tokens for an agent.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in.Agent = args[0]
			out, err := RunList(cmd.Context(), in)
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "TokenList")
		},
	}
	flags.AddPageFlags(cmd, &in.Page)
	return cmd
}

func newRevokeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke <token-id>",
		Short: "Revoke a token",
		Long:  "Soft-revokes a token by ID.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunRevoke(cmd.Context(), RevokeInput{TokenID: args[0]})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Token")
		},
	}
	return cmd
}
