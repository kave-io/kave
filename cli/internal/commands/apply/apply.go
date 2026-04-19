package apply

import (
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
)


func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Declarative provisioning",
		Long:  "Apply a kave.yaml file to the daemon.",
	}
	cmd.AddCommand(newApplyCmd())


	parent.AddCommand(cmd)
}

func newApplyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunApply(cmd.Context(), ApplyInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Apply")
		},
	}
	return cmd
}

