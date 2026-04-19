package flags

import "github.com/spf13/cobra"

// These are persistent flags handled by root.go,
// but provided here for reference and potential per-command overrides.

type ServerInput struct {
	Server  string
	Timeout string
}

func AddServerFlags(cmd *cobra.Command, s *ServerInput) {
	cmd.Flags().StringVar(&s.Server, "server", "", "Remote daemon address")
	cmd.Flags().StringVar(&s.Timeout, "timeout", "30s", "Request timeout")
}
