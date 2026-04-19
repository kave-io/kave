package events

import (
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
)


func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "events",
		Short: "Control-plane events",
		Long:  "Policy decisions, budget alerts, credential changes.",
	}
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newTailCmd())


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

func newTailCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tail",
		Short: "Tail",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunTail(cmd.Context(), TailInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Tail")
		},
	}
	return cmd
}

