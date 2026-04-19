package cmd

import (
	"github.com/kave-io/kave/cli/internal/commands/admin"
	"github.com/kave-io/kave/cli/internal/commands/agent"
	"github.com/kave-io/kave/cli/internal/commands/apply"
	"github.com/kave-io/kave/cli/internal/commands/auth"
	"github.com/kave-io/kave/cli/internal/commands/budget"
	"github.com/kave-io/kave/cli/internal/commands/completion"
	"github.com/kave-io/kave/cli/internal/commands/config"
	"github.com/kave-io/kave/cli/internal/commands/connector"
	"github.com/kave-io/kave/cli/internal/commands/credential"
	"github.com/kave-io/kave/cli/internal/commands/ctx"
	"github.com/kave-io/kave/cli/internal/commands/events"
	"github.com/kave-io/kave/cli/internal/commands/lifecycle"
	"github.com/kave-io/kave/cli/internal/commands/policy"
	"github.com/kave-io/kave/cli/internal/commands/price"
	"github.com/kave-io/kave/cli/internal/commands/rbac"
	"github.com/kave-io/kave/cli/internal/commands/span"
	"github.com/kave-io/kave/cli/internal/commands/trace"
	"github.com/kave-io/kave/cli/internal/commands/version"
	"github.com/spf13/cobra"
)

// This file is the single source of truth for "what commands exist."
// Each subtree package's Register() function is called here to build the command tree.

func registerCommands(root *cobra.Command) {
	// Top-level lifecycle commands
	lifecycle.Register(root)

	// Declarative provisioning
	apply.Register(root)

	// Observability
	trace.Register(root)
	span.Register(root)
	events.Register(root)

	// Resource management
	agent.Register(root)
	policy.Register(root)
	credential.Register(root)
	budget.Register(root)
	price.Register(root)

	// Infrastructure
	connector.Register(root)

	// Access control
	auth.Register(root)
	rbac.Register(root)

	// Configuration and context
	ctx.Register(root)
	config.Register(root)

	// Administration and metadata
	admin.Register(root)
	version.Register(root)
	completion.Register(root)
}
