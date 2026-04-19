package agent

import (
	"github.com/kave-io/kave/cli/internal/commands/agent/token"
	"github.com/kave-io/kave/cli/internal/flags"
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
)

func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage agents",
		Long:  "CRUD and token-management commands for agent identities.",
	}
	cmd.AddCommand(
		newListCmd(),
		newGetCmd(),
		newCreateCmd(),
		newUpdateCmd(),
		newExportCmd(),
		newDeleteCmd(),
		newRestoreCmd(),
	)
	token.Register(cmd)
	parent.AddCommand(cmd)
}

func newListCmd() *cobra.Command {
	var in ListInput
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agents",
		Long:  "Lists agents with pagination.",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunList(cmd.Context(), in)
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "AgentList")
		},
	}
	flags.AddPageFlags(cmd, &in.Page)
	return cmd
}

func newGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id|name>",
		Short: "Get one agent",
		Long:  "Looks up an agent by ID or name.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunGet(cmd.Context(), GetInput{Identifier: args[0]})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Agent")
		},
	}
	return cmd
}

func newCreateCmd() *cobra.Command {
	var in CreateInput
	cmd := &cobra.Command{
		Use:     "create",
		Short:   "Create a new agent",
		Long:    "Registers a new agent in the target environment.",
		Example: "kave agent create --name prod-bot --policy strict --credential oai-primary",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunCreate(cmd.Context(), in)
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Agent")
		},
	}
	flags.AddResourceFlags(cmd, &in.Resource)
	cmd.Flags().StringVar(&in.Env, "env", "", "Environment name")
	cmd.Flags().StringVar(&in.Policy, "policy", "", "Policy name or ID")
	cmd.Flags().StringArrayVar(&in.Credentials, "credential", nil, "Outbound credential name or ID (repeatable)")
	cmd.Flags().StringVar(&in.MonthlyBudget, "monthly-budget", "", "Amount with currency")
	return cmd
}

func newUpdateCmd() *cobra.Command {
	var in UpdateInput
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update an agent",
		Long:  "Updates an existing agent.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in.ID = args[0]
			out, err := RunUpdate(cmd.Context(), in)
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Agent")
		},
	}
	flags.AddResourceFlags(cmd, &in.Resource)
	cmd.Flags().StringVar(&in.Policy, "policy", "", "Policy name or ID")
	cmd.Flags().StringArrayVar(&in.Credentials, "credential", nil, "Outbound credential name or ID (repeatable)")
	cmd.Flags().StringVar(&in.MonthlyBudget, "monthly-budget", "", "Amount with currency")
	return cmd
}

func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export <id>",
		Short: "Export an agent",
		Long:  "Exports a single agent definition.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunExport(cmd.Context(), ExportInput{ID: args[0]})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Agent")
		},
	}
	return cmd
}

func newDeleteCmd() *cobra.Command {
	var in DeleteInput
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Soft-delete an agent",
		Long:  "Soft-deletes an agent by default.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in.ID = args[0]
			out, err := RunDelete(cmd.Context(), in)
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Agent")
		},
	}
	cmd.Flags().BoolVar(&in.Hard, "hard", false, "Permanently delete the record")
	return cmd
}

func newRestoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore <id>",
		Short: "Restore an agent",
		Long:  "Restores a soft-deleted agent.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunRestore(cmd.Context(), RestoreInput{ID: args[0]})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Agent")
		},
	}
	return cmd
}
