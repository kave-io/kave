package admin

import (
	"github.com/spf13/cobra"
	"github.com/kave-io/kave/cli/internal/commands/admin/store"
)


func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Operator tools",
		Long:  "Low-level store management (not for daily drivers).",
	}

	store.Register(cmd)

	parent.AddCommand(cmd)
}

