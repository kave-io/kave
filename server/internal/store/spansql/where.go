package spansql

import (
	"strconv"
	"strings"
	"time"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
)

// Dialect describes backend-specific SQL literal and placeholder behavior.
type Dialect struct {
	placeholder func(int) string
	timeValue   func(int64) any
}

// Postgres uses $1-style placeholders and timestamps for span time filters.
var Postgres = Dialect{
	placeholder: func(n int) string { return "$" + strconv.Itoa(n) },
	timeValue:   func(ms int64) any { return time.UnixMilli(ms) },
}

// DuckDB uses the same placeholder style as Postgres in this codebase, but
// keeps its own value conversion hook so the shared builder stays explicit.
var DuckDB = Dialect{
	placeholder: func(n int) string { return "$" + strconv.Itoa(n) },
	timeValue:   func(ms int64) any { return ms },
}

// ClickHouse uses ? placeholders and DateTime64 values for span time filters.
var ClickHouse = Dialect{
	placeholder: func(int) string { return "?" },
	timeValue:   func(ms int64) any { return time.UnixMilli(ms) },
}

// Question uses ? placeholders for SQLite-style callers.
var Question = Dialect{
	placeholder: func(int) string { return "?" },
	timeValue:   func(ms int64) any { return ms },
}

// BuildWhere returns an SQL AND-clause fragment plus args for SpanFilter.
func BuildWhere(filter *runtimemodel.SpanFilter, d Dialect) (string, []any) {
	if filter == nil {
		return "", nil
	}
	if d.placeholder == nil {
		d = Postgres
	}
	if d.timeValue == nil {
		d.timeValue = func(ms int64) any { return ms }
	}

	var b strings.Builder
	args := make([]any, 0, 12)
	argNum := 1

	appendEq := func(column, value string) {
		if value == "" {
			return
		}
		b.WriteString(" AND ")
		b.WriteString(column)
		b.WriteString(" = ")
		b.WriteString(d.placeholder(argNum))
		args = append(args, value)
		argNum++
	}

	appendEq("id", filter.ID)
	appendEq("run_id", filter.RunID)
	appendEq("action_id", filter.ActionID)
	appendEq("trace_id", filter.TraceID)
	appendEq("project_id", filter.ProjectID)
	appendEq("env_id", filter.EnvID)
	appendEq("agent_id", filter.AgentID)
	appendEq("connector", filter.Connector)
	appendEq("model", filter.Model)
	appendEq("kind", filter.Kind)

	if filter.NamePrefix != "" {
		b.WriteString(" AND name LIKE ")
		b.WriteString(d.placeholder(argNum))
		args = append(args, filter.NamePrefix+"%")
		argNum++
	}
	if filter.FromMs != nil {
		b.WriteString(" AND started_at >= ")
		b.WriteString(d.placeholder(argNum))
		args = append(args, d.timeValue(*filter.FromMs))
		argNum++
	}
	if filter.ToMs != nil {
		b.WriteString(" AND started_at <= ")
		b.WriteString(d.placeholder(argNum))
		args = append(args, d.timeValue(*filter.ToMs))
		argNum++
	}
	if filter.HasError != nil {
		if *filter.HasError {
			b.WriteString(" AND error IS NOT NULL")
		} else {
			b.WriteString(" AND error IS NULL")
		}
	}
	if filter.MinCostMicro != nil {
		b.WriteString(" AND COALESCE(cost_amount_nanos, 0) >= ")
		b.WriteString(d.placeholder(argNum))
		args = append(args, *filter.MinCostMicro*int64(money.MicroDollar))
	}

	return b.String(), args
}
