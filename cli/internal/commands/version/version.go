package version

import (
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
)


func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Version information",
		Long:  "Show version and build information.",
	}
	cmd.AddCommand(newVersionCmd())


	parent.AddCommand(cmd)
}

func newVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Version",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunVersion(cmd.Context(), VersionInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Version")
		},
	}
	return cmd
}

