package flags

import (
	"github.com/spf13/cobra"
)

type OutputInput struct {
	Output  string
	NoColor bool
}

func AddOutputFlags(cmd *cobra.Command, o *OutputInput) {
	cmd.Flags().StringVar(&o.Output, "output", "auto", "Output format: table, json, or yaml")
	cmd.Flags().BoolVar(&o.NoColor, "no-color", false, "Disable ANSI colors")
}
