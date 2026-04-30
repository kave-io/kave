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
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Watch",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := RunWatch(cmd.Context(), WatchInput{})
			if err != nil {
				return err
			}
			return output.Render(cmd, out, "Watch")
		},
	}
	return cmd
}
