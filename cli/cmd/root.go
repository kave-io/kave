package cmd

import (
	"fmt"
	"os"

	"github.com/kave-io/kave/cli/internal/config"
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/kave-io/kave/cli/internal/runtime"
	"github.com/spf13/cobra"
)

var (
	rootOptions config.RootOptions
	activeRT    *runtime.Runtime
	rootCmd     = newRootCmd()
)

func init() {
	cobra.EnableCommandSorting = false
	configureTemplates()
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		writeCommandError(err)
		os.Exit(exitCode(err))
	}
}

func Root() *cobra.Command {
	return rootCmd
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kave",
		Short: "Local-first CLI skeleton for Kave v1",
		Long:  "Kave v1 CLI skeleton with the documented command tree, config resolution hooks, output envelope plumbing, and placeholder service wiring.",
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if jsonFlag := cmd.Flags().Lookup("json"); jsonFlag != nil {
				if enabled, err := cmd.Flags().GetBool("json"); err == nil && enabled {
					rootOptions.Output = string(output.FormatJSON)
				}
			}

			resolution, err := config.Resolve(rootOptions)
			if err != nil {
				return err
			}

			format, err := output.ParseFormat(rootOptions.Output)
			if err != nil {
				return err
			}

			activeRT = runtime.New(resolution, format)
			cmd.SetContext(runtime.WithContext(cmd.Context(), activeRT))
			return nil
		},
	}

	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetHelpCommand(&cobra.Command{Hidden: true})

	addRootFlags(cmd)
	registerCommands(cmd)
	cmd.InitDefaultCompletionCmd()
	cmd.Annotations = map[string]string{
		"kave.see_also": "kave init, kave doctor, kave start, kave stop, kave status, kave logs, kave watch, kave version, kave completion, kave apply, kave diff, kave trace, kave span, kave events, kave agent, kave policy, kave credential, kave budget, kave price, kave connector, kave auth, kave rbac, kave ctx, kave config",
	}

	return cmd
}

func addRootFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&rootOptions.ConfigPath, "config", "", "Override project kave.yaml path")
	cmd.PersistentFlags().StringVar(&rootOptions.Context, "context", "", "Named context selection")
	cmd.PersistentFlags().StringVar(&rootOptions.Context, "ctx", "", "Alias for --context")
	cmd.PersistentFlags().StringVar(&rootOptions.Profile, "profile", "", "Back-compat alias for --context")
	cmd.PersistentFlags().StringVar(&rootOptions.Server, "server", "", "Remote daemon address")
	cmd.PersistentFlags().StringVar(&rootOptions.Output, "output", string(output.FormatAuto), "Output format: table, json, or yaml")
	cmd.PersistentFlags().BoolVar(&rootOptions.NoColor, "no-color", false, "Disable ANSI colors")
	cmd.PersistentFlags().StringVar(&rootOptions.Timeout, "timeout", "30s", "Request timeout")
	cmd.PersistentFlags().CountVarP(&rootOptions.Verbose, "verbose", "v", "Increase verbosity")
}

func writeCommandError(err error) {
	if activeRT == nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return
	}

	format := output.ResolveFormat(activeRT.Output, os.Stdout)
	if writeErr := output.WriteError(os.Stderr, format, err); writeErr != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if coded, ok := err.(interface{ ExitCode() int }); ok {
		return coded.ExitCode()
	}
	return 1
}
