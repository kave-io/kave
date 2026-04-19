package completion

import (
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
)


func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "completion",
		Short: "Shell completion",
		Long:  "Generate shell completion scripts.",
	}
	cmd.AddCommand(newBashCmd())
	cmd.AddCommand(newZshCmd())
	cmd.AddCommand(newFishCmd())
	cmd.AddCommand(newPowershellCmd())


	parent.AddCommand(cmd)
}

func newBashCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bash",
		Short: "Bash",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunBash(cmd.Context(), BashInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Bash")
		},
	}
	return cmd
}

func newZshCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "zsh",
		Short: "Zsh",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunZsh(cmd.Context(), ZshInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Zsh")
		},
	}
	return cmd
}

func newFishCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fish",
		Short: "Fish",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunFish(cmd.Context(), FishInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Fish")
		},
	}
	return cmd
}

func newPowershellCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "powershell",
		Short: "Powershell",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunPowershell(cmd.Context(), PowershellInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Powershell")
		},
	}
	return cmd
}

