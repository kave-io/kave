package connector

import (
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
)


func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "connector",
		Short: "Available connectors",
		Long:  "LLM provider connectors (Anthropic, OpenAI, Ollama, etc.)",
	}
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newEnableCmd())
	cmd.AddCommand(newDisableCmd())
	cmd.AddCommand(newTestCmd())


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
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunGet(cmd.Context(), GetInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Get")
		},
	}
	return cmd
}

func newEnableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunEnable(cmd.Context(), EnableInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Enable")
		},
	}
	return cmd
}

func newDisableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunDisable(cmd.Context(), DisableInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Disable")
		},
	}
	return cmd
}

func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunTest(cmd.Context(), TestInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Test")
		},
	}
	return cmd
}

