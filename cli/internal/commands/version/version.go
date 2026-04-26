package version

import (
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show version and build information",
		Long:  "Show version and build information.",
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

