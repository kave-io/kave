# Kave Dashboard Experience v1

**Product sentence:** Kave Dashboard is the local control room for agent traffic: see every run, understand every cost, enforce every policy, and prove what happened.

It should feel less like a generic SaaS dashboard and more like **Docker Desktop + Datadog trace explorer + Stripe cost ledger for agents**.

## 1. Core design principles

**One contract, one truth.**
Dashboard uses generated RPC clients only. No `/api/v1/*`, no fake REST layer, no static demo health.

**Observe first, control second.**
The first magic moment is: “I ran my agent, and Kave immediately showed what happened, what it cost, what policy allowed it, and what would have blocked it.”

**Local-first, cloud-ready.**
The embedded dashboard must work beautifully for one developer on localhost. Kave Cloud should extend the same UX with orgs, teams, users, billing, RBAC, invitations, and compliance.

**No fake cards.**
If connector health is not implemented, do not show fake “Healthy” rows. Empty states should teach the user how to produce real data.

**Developer-native.**
IDs, traces, tokens, config, proxy URLs, JSON, policy documents, and export buttons are first-class UI elements, not hidden behind “friendly” abstractions.

---

# 2. Main information architecture

The current nav is close but too shallow. I would reshape it into this:

```txt
Overview
Monitor
Runs
Traces
Agents
Policies
Connectors
Credentials
Budgets
Audit
Settings
```

For embedded OSS v1, you can start smaller:

```txt
Overview
Monitor
Runs
Traces
Agents
Policies
Connectors
Budgets
Settings
```

Cloud later adds:

```txt
Organization
Projects
Environments
Users
Teams
Roles
Billing
Compliance
```

---

# 3. App shell

## Sidebar

The sidebar should be grouped by product job, not by backend object only.

```txt
Kave

Observe
  Overview
  Monitor
  Runs
  Traces
  Audit

Control
  Agents
  Policies
  Connectors
  Credentials

Spend
  Budgets
  Price Book
  FX Rates

System
  Settings
  Daemon
```

For OSS v1, `Credentials`, `Price Book`, `FX Rates`, and `Daemon` can live under Settings until they deserve full pages.

## Top bar

The top bar should always contain:

```txt
[sidebar toggle] [project/env selector] [time range] [currency] [live daemon status] [command/search]
```

For local embedded mode:

```txt
Project: default
Environment: dev
Range: last 15m / 1h / 24h / 7d
Currency: USD / Toman
Daemon: Live / Offline / Reconnecting
```

For cloud:

```txt
Org → Project → Environment
User avatar
Role / permission state
```

## Command palette

Keyboard shortcut: `Cmd/Ctrl + K`.

Actions:

```txt
Search run id
Search trace id
Search agent
Open proxy URLs
Create agent
Create policy
Issue agent token
Open pricing book
Copy OpenAI proxy URL
Copy Anthropic proxy URL
Export current trace
```

This is very Kave. Hardcore developers will love it.

---

# 4. Page design

## 4.1 Overview — “Command Center”

Route:

```txt
/
```

Purpose: answer “Is Kave seeing traffic, is anything blocked, and what is it costing?”

Layout:

```txt
Header
  Kave Command Center
  Local daemon / cloud workspace status
  Live badge

Top stat cards
  Runs
  Active runs
  Blocked actions
  Spend
  Error rate
  Avg latency

Main grid
  Left: Live activity stream
  Right: Alerts / policy blocks / budget warnings

Second grid
  Spend by provider/model
  Recent failed or blocked runs
  Top agents by spend
```

Important: the overview should **not** become the place for everything. It should be the “what needs my attention?” page.

### Components

```txt
DashboardStatCard
LiveActivityFeed
AlertStack
SpendBreakdownCard
TopAgentsCard
RecentBlocksCard
EmptyTrafficState
```

### Empty state

When no runs exist:

```txt
No agent traffic yet.

Point your SDK or tool at Kave:

OPENAI_BASE_URL=http://127.0.0.1:18080/v1/openai
ANTHROPIC_BASE_URL=http://127.0.0.1:18080/v1/anthropic

Then run your agent. Kave will show runs, spans, cost, and policy decisions here.
```

Buttons:

```txt
Copy OpenAI URL
Copy Anthropic URL
View setup guide
```

---

## 4.2 Monitor — “Live traffic”

Route:

```txt
/monitor
```

Purpose: replace the old “live spans table” with a real operator screen.

This is the magic page for `kave watch`.

Layout:

```txt
Header
  Live Monitor
  Watching runs, spans, policy decisions, cost events

Left rail filters
  Agent
  Provider
  Model
  Status
  Event type
  Errors only
  Blocked only

Main stream
  Timeline of events:
    run.started
    span.opened
    provider.request
    policy.allowed
    policy.denied
    budget.charged
    span.closed
    run.completed
    run.failed

Right side detail panel
  Selected event details
  JSON payload
  Related run
  Related trace
  Policy decision
  Cost snapshot
```

This page should feel alive. It should show streaming RPC data, not polling or SSE.

### Event row design

Each row:

```txt
[time] [event type badge] [agent] [provider/model] [duration] [cost] [status]
```

Examples:

```txt
14:22:18  span.closed     research-agent   openai/gpt-4.1-mini   832ms   $0.0021   ok
14:22:19  policy.denied   scraper-agent    github/repos.write    —       —         blocked
14:22:21  run.failed      invoice-agent    anthropic/claude      4.2s    $0.031    error
```

### Components

```txt
LiveEventStream
EventTypeBadge
LiveFilterRail
EventDetailPanel
JsonInspector
CopyableId
LiveConnectionBadge
```

---

## 4.3 Runs

Route:

```txt
/runs
/runs/:runId
```

Purpose: inspect agent executions.

The current RunsView is okay structurally, but it needs to become the primary debugging object. A run is the user’s main unit of work.

## Runs list

Columns:

```txt
Status
Run
Agent
Policy
Started
Duration
Spend
Tokens
Spans
Error
```

Filters:

```txt
Time range
Status
Agent
Policy
Provider
Model
Min cost
Has error
Blocked
```

Search:

```txt
run id
agent id/name
trace id
model
error text
```

Row click opens detail page or split drawer.

## Run detail

Top summary:

```txt
Run name / id
Status
Agent
Policy
Started / ended
Duration
Spend
Budget cap
Trace id
```

Tabs:

```txt
Timeline
Spans
Cost
Policy
Input/Output
Metadata
Raw
```

### Timeline tab

A chronological view:

```txt
run.started
policy.evaluated
span.opened
provider.request
span.closed
budget.charged
run.completed
```

This gives users the “movie” of the run.

### Spans tab

Table + nested tree:

```txt
root run
  llm.chat.completions
    tool.github.search
    llm.embeddings
```

Each span shows:

```txt
duration
model
provider
input tokens
output tokens
cache tokens
cost
error
```

### Cost tab

Break down:

```txt
Total spend
By provider
By model
By span
Input/output/cache split
FX snapshot if converted
Price snapshot version
```

### Policy tab

Show:

```txt
Policy attached
Decision result
Allowed connectors
Allowed methods
Budget behavior
Trace input/output flags
Retention
Casbin document if present
```

### Components

```txt
RunsTable
RunStatusBadge
RunDetailHeader
RunTimeline
SpanTree
RunCostBreakdown
PolicyDecisionPanel
RawRecordViewer
```

---

## 4.4 Traces

Route:

```txt
/traces
/traces/:traceId
```

Purpose: deep debugging and performance analysis.

This should not just be a spans table. It should be a trace explorer.

## Traces list

Columns:

```txt
Trace
Root run
Agent
Status
Started
Duration
Spans
Spend
Errors
```

## Trace detail

Main layout:

```txt
Header summary
  trace id
  root run
  agent
  status
  duration
  spend

Main panel
  Waterfall view

Right panel
  Selected span detail
```

Waterfall rows:

```txt
span name
start offset
duration bar
provider/model
tokens
cost
status
```

Span detail panel:

```txt
Identity
  span id
  parent id
  run id
  action id

Timing
  started
  ended
  duration

Model/cost
  provider
  model
  input tokens
  output tokens
  cache read/write
  cost

Payload
  input preview
  output preview
  redaction state

Error
  message
  stack/details

Raw JSON
```

### Components

```txt
TraceWaterfall
TraceMiniMap
SpanDetailPanel
TokenUsagePill
CostPill
DurationBar
SpanErrorBanner
PayloadPreview
```

This page is one of the most important Kave differentiators. It should make agent work visible.

---

## 4.5 Agents

Route:

```txt
/agents
/agents/:agentId
```

Purpose: manage the things that are allowed to act through Kave.

The current AgentsView is too thin. An agent should feel like a real controlled actor.

## Agents list

Cards or table depending on screen width.

Columns:

```txt
Agent
Status
Policy
Monthly budget
Runs
Spend
Last seen
Tokens
```

Actions:

```txt
Create agent
Issue token
Disable
Export
Delete
```

## Agent detail

Header:

```txt
Agent name
Status
Agent id
Project / environment
Policy
Budget
Last seen
```

Tabs:

```txt
Overview
Runs
Tokens
Policy
Budget
Metadata
Raw
```

### Tokens tab

This is critical.

Show:

```txt
Issued tokens
Fingerprint only
Created
Last used
Expires
Permissions
Status
Revoke action
```

Never show full token again after creation.

On issue token:

```txt
Token created. Copy it now. Kave will not show it again.
```

### Agent creation flow

Fields:

```txt
Name
Description
Environment
Policy
Monthly budget
Metadata
```

Then success screen:

```txt
Agent created

1. Issue token
2. Configure your framework
3. Run your first request
```

### Components

```txt
AgentTable
AgentCard
AgentDetailHeader
AgentTokenTable
IssueTokenModal
AgentBudgetPanel
AgentRecentRuns
AgentMetadataEditor
```

---

## 4.6 Policies

Route:

```txt
/policies
/policies/:policyId
```

Purpose: make control understandable.

Policies are where Kave becomes more than observability.

The policy page should not just list policy IDs from agents. It needs a first-class policy builder.

## Policy list

Columns:

```txt
Policy
Mode
Status
Budget cap
Budget behavior
Allowed connectors
Attached agents
Updated
```

Actions:

```txt
Create policy
Validate policy
Export
Delete
```

## Policy detail

Tabs:

```txt
Summary
Rules
Budget
Tracing
Attached agents
Test
Raw
```

### Summary tab

Human-readable explanation:

```txt
This policy allows:
- LLM actions through OpenAI and Anthropic
- Tool actions through GitHub and Postgres
- Up to $25/month
- Input/output tracing enabled
- Blocks when budget is exceeded
```

### Rules tab

Builder sections:

```txt
Allowed action types
Allowed connectors
Allowed methods
Denied methods
Model/provider constraints
Tool constraints
```

### Budget tab

```txt
Budget cap
Budget period
Budget behavior
  warn
  block
  require approval later, cloud
```

### Tracing tab

```txt
Trace input
Trace output
Retention days
Redaction mode
```

### Test tab

A policy simulator:

```txt
Connector: openai
Method: chat.completions
Model: gpt-4.1-mini
Estimated cost: $0.002

[Run test]

Result:
Allowed / Denied
Reason:
Matched rule:
Budget impact:
```

This is a huge UX win. Developers need to understand why something was blocked before production.

### Components

```txt
PolicyTable
PolicyBuilder
PolicySummaryCard
PolicyRuleSection
PolicyBudgetEditor
PolicyTracingEditor
PolicySimulator
PolicyDecisionResult
AttachedAgentsList
```

---

## 4.7 Connectors

Route:

```txt
/connectors
/connectors/:connectorId
```

Purpose: show what Kave can intercept, proxy, or call.

Group connectors exactly like the architecture:

```txt
Frameworks
  Claude Code
  LangChain
  OpenAI Agents
  CrewAI
  AutoGen
  OpenClaw

LLM Providers
  OpenAI
  Anthropic
  Gemini
  Groq
  Ollama

Protocols
  MCP
  A2A
  OTEL

Tools
  GitHub
  Gmail
  Postgres
  S3
  Slack
  Stripe
  Zarinpal
```

Each connector card:

```txt
Name
Kind
Status
Requires auth
Enabled/disabled
Credentials count
Last used
Supported methods
```

Detail page:

```txt
Overview
Setup
Credentials
Methods
Recent traffic
Raw config
```

### Setup tab

This should give copy-paste integration snippets.

For OpenAI-compatible traffic:

```txt
export OPENAI_BASE_URL=http://127.0.0.1:18080/v1/openai
export OPENAI_API_KEY=<kave-agent-token>
```

For Claude Code:

```txt
export ANTHROPIC_BASE_URL=http://127.0.0.1:18080/frameworks/claude-code/anthropic
export ANTHROPIC_API_KEY=<kave-agent-token>
```

For Ollama:

```txt
export OLLAMA_HOST=http://127.0.0.1:18080/frameworks/claude-code/ollama
```

### Components

```txt
ConnectorGrid
ConnectorKindTabs
ConnectorCard
ConnectorSetupSnippet
ConnectorMethodTable
ConnectorCredentialsPanel
ConnectorTrafficList
RequiresAuthBadge
```

---

## 4.8 Credentials

Route:

```txt
/credentials
```

For OSS v1, this can be a Settings section. For cloud/team usage, it deserves its own page.

Purpose: manage provider/tool secrets without exposing them.

Columns:

```txt
Connector
Label
Environment
Status
Created
Last used
Expires
Rotated
```

Actions:

```txt
Create
Rotate
Test
Revoke
Delete
```

Credential detail:

```txt
Connector
Environment
Label
Account hint
Status
Created
Last used
Expires
Rotation metadata
```

Never display secret values after creation.

Components:

```txt
CredentialTable
CreateCredentialModal
RotateCredentialModal
CredentialStatusBadge
SecretCreatedPanel
```

---

## 4.9 Budgets / Cost

Route:

```txt
/budgets
/cost
```

I would name the nav item **Budgets** but the page header can say **Cost & Budgets**.

Purpose: show what agents are spending and where limits apply.

Layout:

```txt
Header
  Cost & Budgets
  Time range
  Currency
  FX status

Top cards
  Total spend
  Forecast
  Blocked by budget
  Highest spending agent

Charts/tables
  Spend over time
  Spend by agent
  Spend by provider
  Spend by model
  Spend by connector
  Budget usage
```

Budget usage row:

```txt
Agent
Policy
Budget cap
Spent
Remaining
Usage %
Behavior
Status
```

Price book section:

```txt
Active price book version
Last refreshed
Providers covered
Missing prices
Edit JSON / import / export
```

FX section:

```txt
Display currency
Available rates
Toman mode: provider-priced / operator-set / missing
Stale rate warning
Refresh
```

### Components

```txt
CostOverviewCards
SpendOverTimeChart
SpendBreakdownTable
BudgetUsageTable
BudgetProgressBar
PriceBookEditor
FXRatePanel
MissingPriceWarning
```

---

## 4.10 Audit

Route:

```txt
/audit
```

Purpose: prove what changed and who/what did it.

For OSS, audit may be simple. For cloud, this becomes enterprise-critical.

Columns:

```txt
Time
Actor
Action
Resource
Result
Reason
IP / source
```

Filters:

```txt
Actor
Action
Resource type
Result
Time range
```

Events:

```txt
agent.created
agent.disabled
token.issued
token.revoked
policy.created
policy.updated
credential.created
credential.rotated
budget.exceeded
guest.invocation
auth.failed
```

Components:

```txt
AuditTable
AuditEventBadge
ActorBadge
ResourceLink
AuditDetailDrawer
```

---

## 4.11 Settings

Route:

```txt
/settings
```

Settings should become cleaner and less mixed.

Sections:

```txt
Workspace
  Project id
  Environment id
  Trust mode
  Export config

Daemon
  Server URL
  Version
  Storage backend
  RPC endpoint
  Gateway routes
  Health

Preferences
  Theme
  Language
  Currency
  Time format

Proxy URLs
  OpenAI
  Anthropic
  Gemini
  Ollama
  Claude Code framework routes

Data
  Retention
  Export
  Import
  Reset local dashboard state

Advanced
  Price book JSON
  Raw config
  Debug info
```

Current SettingsView has useful pieces, but pricing JSON should move into either **Budgets** or **Advanced**. Proxy URLs deserve a beautiful setup section, not a long util list.

---

# 5. Key reusable components

## Base UI components

```txt
KavePage
KavePageHeader
KaveSection
KaveCard
KaveEmptyState
KaveErrorState
KaveSkeleton
KaveCopyButton
KaveCodeBlock
KaveJsonViewer
KaveId
KaveTimestamp
KaveMoney
KaveDuration
KaveTokenUsage
```

## Status components

```txt
DaemonStatusBadge
LiveStatusBadge
RunStatusBadge
AgentStatusBadge
PolicyStatusBadge
ConnectorStatusBadge
CredentialStatusBadge
TrustModeBadge
BudgetBehaviorBadge
DecisionBadge
```

## Data components

```txt
KaveDataTable
KaveFilterBar
KaveSearchInput
KaveTimeRangeSelect
KaveCurrencySelect
KaveEnvironmentSelect
KaveColumnVisibilityMenu
KaveExportMenu
```

## Product components

```txt
RunTimeline
TraceWaterfall
SpanTree
SpanDetailPanel
LiveEventStream
AgentTokenTable
PolicyBuilder
PolicySimulator
ConnectorSetupSnippet
BudgetUsageTable
PriceBookEditor
FXRatePanel
AuditTable
```

---

# 6. Visual design direction

Kave should not look like a random admin template. It should feel like an infrastructure control plane.

## Style

```txt
Dense but calm
Dark-mode excellent
Light-mode readable
Muted surfaces
Sharp typography
Rounded cards, but not playful
Monospace where IDs/cost/tokens matter
Green only for truly healthy/live
Amber for warnings
Red for blocked/errors
Purple/blue for traces and RPC events
```

## Suggested personality

```txt
Local daemon
Agent observability
Policy control
Budget ledger
Developer trust
```

## Layout density

Use three density levels:

```txt
Comfortable: default for overview/settings
Compact: tables, traces, monitor
Dense: raw JSON, event streams, audit
```

Let the user toggle density later.

---

# 7. Navigation hierarchy I would implement now

For the next dashboard rewrite, I would create this route map:

```txt
/
  Overview

/monitor
  Live traffic

/runs
  Runs list

/runs/:id
  Run detail

/traces
  Traces list

/traces/:id
  Trace waterfall

/agents
  Agents list

/agents/:id
  Agent detail

/policies
  Policies list

/policies/:id
  Policy detail / builder

/connectors
  Connector catalog

/connectors/:id
  Connector detail / setup

/budgets
  Cost & budgets

/audit
  Audit log

/settings
  System settings
```

For v1, details can be drawers instead of full pages, but the route model should assume detail pages eventually.

---

# 8. What to remove from the current dashboard

Remove or replace:

```txt
Static connector health cards
EventSource /api/v1/spans/stream
fetch('/api/v1/settings/pricing')
Any handwritten REST assumptions
Policy list derived only from agents
Hardcoded proxy URLs without connector awareness
Fake “today” metrics if the backend query is not actually time-ranged
Overloaded Settings page
```

Keep and improve:

```txt
Locale support
RTL support
Currency selector
Money formatting
Nuxt UI / Vue Query structure
Generated RPC client direction
Embedded Vite app
PageHeader idea
Status badges
Detail drawer pattern
```

---

# 9. First-run onboarding

The dashboard needs a first-run path.

When Kave has no traffic:

```txt
Welcome to Kave

1. Start the daemon
2. Create or select an agent
3. Issue an agent token
4. Point your SDK/tool to Kave
5. Run your first request
```

Show cards:

```txt
Use with OpenAI SDK
Use with Claude Code
Use with Ollama
Use with LangChain
```

Each card has:

```txt
Copy env vars
View connector
Run test
```

This turns the dashboard into an adoption engine, not just an observability screen.

---

# 10. Cloud-ready extensions

The embedded dashboard should not show cloud-only clutter, but the design should reserve space for it.

## OSS local

```txt
Single workspace
Default project/env
Local daemon
Local credentials
Local policies
Local audit
No billing
No team management
```

## Kave Cloud individual

```txt
Hosted workspaces
Personal account
Cloud auth
Remote agents
Hosted traces
Usage limits
Subscription billing
```

## Kave Cloud teams

```txt
Org switcher
Projects
Environments
Members
Roles
Shared budgets
Team audit
Credential ownership
```

## Kave Cloud enterprise

```txt
SAML/SSO
SCIM
Advanced RBAC
Compliance exports
Retention policies
Approval workflows
Org-wide policies
Org/project/env budgets
```

The same sidebar can evolve:

```txt
Personal/local:
  Overview, Monitor, Runs, Traces, Agents, Policies, Connectors, Budgets, Settings

Team/cloud:
  Add Organization, Members, Roles, Billing, Compliance
```

---

# 11. Recommended build order

Do not build all pages at once. Build the dashboard around the strongest product loop.

## Phase 1 — Magic visibility loop

```txt
App shell
Overview empty state
Monitor live stream
Runs list
Run detail drawer
Trace/span detail drawer
Connector setup snippets
```

Goal: user runs an agent and sees activity instantly.

## Phase 2 — Control loop

```txt
Agents
Issue token modal
Policies list
Policy detail
Policy simulator
Budget cards
```

Goal: user can create controlled agents and understand decisions.

## Phase 3 — Cost loop

```txt
Budgets page
Spend breakdown
Price book editor
FX status
Toman support polished
```

Goal: Kave becomes credible for real usage and Iranian/local providers.

## Phase 4 — Trust loop

```txt
Credentials
Audit log
Trust mode display
Daemon diagnostics
Export flows
```

Goal: teams and companies can trust it.

---

# 12. The dashboard’s core screens in one sentence each

```txt
Overview:
  What is happening and what needs attention?

Monitor:
  What is happening right now?

Runs:
  What did each agent execution do?

Traces:
  Why did it take this long, cost this much, or fail?

Agents:
  Who is allowed to act through Kave?

Policies:
  What is each agent allowed to do?

Connectors:
  What tools, providers, frameworks, and protocols can Kave see or control?

Budgets:
  Where is money going, and what limits are enforced?

Audit:
  Who changed what, and what was blocked or allowed?

Settings:
  How is this daemon/workspace configured?
```

---

# 13. Most important product decision

The dashboard should be centered around **runs and traces**, not around generic stats.

Stats are useful, but the Kave experience becomes valuable when a developer can click one run and answer:

```txt
What agent did this?
What provider/model/tool did it call?
What policy allowed or blocked it?
What did it cost?
Which span was slow?
Which token caused access?
Which credential/provider was used?
Can I export/prove/debug this?
```

That is the Kave dashboard.
