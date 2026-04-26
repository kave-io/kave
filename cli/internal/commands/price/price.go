package price

import (
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/kave-io/kave/cli/internal/commands/price/fx"
)


func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "price",
		Short: "Pricing models and FX",
		Long:  "Manage pricing tables and exchange rates.",
	}
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newRefreshCmd())
	cmd.AddCommand(newImportCmd())
	cmd.AddCommand(newExportCmd())

	fx.Register(cmd)

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

func newGetCmd() *cobra.Command {
	in := GetInput{}
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a price entry",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunGet(cmd.Context(), in)
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Get")
		},
	}
	cmd.Flags().StringVar(&in.Provider, "provider", "", "Provider (e.g. openai, anthropic)")
	cmd.Flags().StringVar(&in.Match, "match", "", "Model match string")
	return cmd
}

func newRefreshCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunRefresh(cmd.Context(), RefreshInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Refresh")
		},
	}
	return cmd
}

func newImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunImport(cmd.Context(), ImportInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Import")
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

