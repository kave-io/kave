package lifecycle

import (
	"github.com/kave-io/kave/cli/internal/output"
	"github.com/spf13/cobra"
)

func Register(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "lifecycle",
		Short: "Daemon lifecycle",
		Long:  "Start, stop, and manage the embedded daemon.",
	}
	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newDoctorCmd())
	cmd.AddCommand(newStartCmd())
	cmd.AddCommand(newStopCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newLogsCmd())
	cmd.AddCommand(newWatchCmd())

	parent.AddCommand(cmd)
	// Top-level compatibility commands (kave status, kave watch, ...).
	parent.AddCommand(newInitCmd())
	parent.AddCommand(newDoctorCmd())
	parent.AddCommand(newStartCmd())
	parent.AddCommand(newStopCmd())
	parent.AddCommand(newStatusCmd())
	parent.AddCommand(newLogsCmd())
	parent.AddCommand(newWatchCmd())
}

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Init",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunInit(cmd.Context(), InitInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Init")
		},
	}
	return cmd
}

func newDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Doctor",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunDoctor(cmd.Context(), DoctorInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Doctor")
		},
	}
	return cmd
}

func newStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunStart(cmd.Context(), StartInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Start")
		},
	}
	return cmd
}

func newStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunStop(cmd.Context(), StopInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Stop")
		},
	}
	return cmd
}

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Status",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunStatus(cmd.Context(), StatusInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Status")
		},
	}
	return cmd
}

func newLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunLogs(cmd.Context(), LogsInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Logs")
		},
	}
	return cmd
}

func newWatchCmd() *cobra.Command {
	var in WatchInput
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Live terminal watch cockpit",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunWatch(cmd.Context(), in)
		},
	}
	cmd.Flags().StringVar(&in.Agent, "agent", "", "Filter by agent ID")
	cmd.Flags().StringVar(&in.Run, "run", "", "Filter by run ID")
	cmd.Flags().StringVar(&in.Trace, "trace", "", "Filter by trace ID")
	cmd.Flags().StringVar(&in.Status, "status", "", "Filter by status")
	cmd.Flags().StringVar(&in.Type, "type", "", "Filter by event type")
	cmd.Flags().DurationVar(&in.Since, "since", 0, "Show events since duration ago (e.g. 10m, 1h)")
	cmd.Flags().IntVar(&in.Limit, "limit", 200, "Max timeline rows to retain")
	cmd.Flags().BoolVar(&in.Compact, "compact", false, "Use compact rows")
	return cmd
}
