package watch

import "time"

type Filter struct {
	Agent   string
	RunID   string
	TraceID string
	Status  string
	Type    string
	Since   time.Duration
	Limit   int
	Compact bool
}

type Event struct {
	At             time.Time
	Kind           string
	Status         string
	RunID          string
	TraceID        string
	SpanID         string
	AgentID        string
	AgentName      string
	Connector      string
	Tool           string
	Model          string
	Message        string
	Error          string
	PolicyDecision string
	Cost           float64
	Currency       string
	InputTokens    int
	OutputTokens   int
	Metadata       map[string]any
	Raw            map[string]any
}

type Stats struct {
	ActiveRuns      int
	CompletedRuns   int
	BlockedOrDenied int
	Errors          int
	TotalCost       float64
	Currency        string
}
