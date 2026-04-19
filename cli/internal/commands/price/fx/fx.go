package fx

import (
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
)


func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "fx",
		Short: "Foreign exchange",
		Long:  "Currency conversion and rates.",
	}
	cmd.AddCommand(newRatesCmd())
	cmd.AddCommand(newConvertCmd())
	cmd.AddCommand(newRefreshCmd())


	parent.AddCommand(cmd)
}

func newRatesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rates",
		Short: "Rates",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunRates(cmd.Context(), RatesInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Rates")
		},
	}
	return cmd
}

func newConvertCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "convert",
		Short: "Convert",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunConvert(cmd.Context(), ConvertInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Convert")
		},
	}
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

