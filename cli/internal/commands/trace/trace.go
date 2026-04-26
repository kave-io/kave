package trace

import (
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
)


func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "trace",
		Short: "Agent invocation traces",
		Long:  "A trace is the tree of spans for one agent invocation.",
	}
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newTailCmd())
	cmd.AddCommand(newGraphCmd())
	cmd.AddCommand(newExportCmd())


	parent.AddCommand(cmd)
}

func newListCmd() *cobra.Command {
	var agentID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List traces",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunList(cmd.Context(), ListInput{AgentID: agentID})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "List")
		},
	}
	cmd.Flags().StringVar(&agentID, "agent", "", "Filter by agent ID")
	return cmd
}

func newGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a trace by run ID",
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

func newGraphCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Graph",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunGraph(cmd.Context(), GraphInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Graph")
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

