package watchui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kave-io/kave/cli/internal/watch"
)

type connectionState string

const (
	stateConnecting   connectionState = "connecting"
	stateLive         connectionState = "live"
	stateDisconnected connectionState = "disconnected"
)

type Options struct {
	NoColor bool
}

type model struct {
	ctx      context.Context
	cancel   context.CancelFunc
	client   watch.Client
	filter   watch.Filter
	opts     Options
	styles   styles
	state    connectionState
	errMsg   string
	events   []watch.Event
	stats    watch.Stats
	selected int
	expanded map[int]bool
	help     bool

	eventCh <-chan watch.Event
	errCh   <-chan error
}

type streamStartedMsg struct {
	events <-chan watch.Event
	errs   <-chan error
}
type streamEventMsg struct{ event watch.Event }
type streamErrMsg struct{ err error }
type streamClosedMsg struct{}

func newModel(ctx context.Context, client watch.Client, filter watch.Filter, opts Options) model {
	wctx, cancel := context.WithCancel(ctx)
	return model{
		ctx:      wctx,
		cancel:   cancel,
		client:   client,
		filter:   filter,
		opts:     opts,
		styles:   newStyles(opts.NoColor),
		state:    stateConnecting,
		expanded: map[int]bool{},
	}
}

func (m model) Init() tea.Cmd { return m.startStreamCmd() }

func (m model) startStreamCmd() tea.Cmd {
	return func() tea.Msg {
		events, errs := m.client.Stream(m.ctx, m.filter)
		return streamStartedMsg{events: events, errs: errs}
	}
}

func (m model) waitStreamCmd() tea.Cmd {
	return func() tea.Msg {
		if m.eventCh == nil && m.errCh == nil {
			return streamClosedMsg{}
		}
		select {
		case <-m.ctx.Done():
			return streamClosedMsg{}
		case err, ok := <-m.errCh:
			if ok && err != nil {
				return streamErrMsg{err: err}
			}
			m.errCh = nil
			if m.eventCh == nil {
				return streamClosedMsg{}
			}
			return streamClosedMsg{}
		case ev, ok := <-m.eventCh:
			if !ok {
				return streamClosedMsg{}
			}
			return streamEventMsg{event: ev}
		}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.cancel()
			return m, tea.Quit
		case "j", "down":
			if m.selected < len(m.events)-1 {
				m.selected++
			}
		case "k", "up":
			if m.selected > 0 {
				m.selected--
			}
		case "enter":
			if len(m.events) > 0 {
				m.expanded[m.selected] = !m.expanded[m.selected]
			}
		case "?":
			m.help = !m.help
		case "r":
			m.state = stateConnecting
			m.errMsg = ""
			return m, m.startStreamCmd()
		}
		return m, nil
	case streamStartedMsg:
		m.eventCh = msg.events
		m.errCh = msg.errs
		m.state = stateLive
		return m, m.waitStreamCmd()
	case streamEventMsg:
		m.state = stateLive
		m.events = append(m.events, msg.event)
		if m.filter.Limit > 0 && len(m.events) > m.filter.Limit {
			m.events = m.events[len(m.events)-m.filter.Limit:]
			if m.selected > len(m.events)-1 {
				m.selected = len(m.events) - 1
			}
		}
		m.stats = watch.ApplyStats(m.stats, msg.event)
		return m, m.waitStreamCmd()
	case streamErrMsg:
		m.state = stateDisconnected
		m.errMsg = msg.err.Error()
		return m, nil
	case streamClosedMsg:
		if m.state != stateDisconnected {
			m.state = stateDisconnected
		}
		return m, nil
	default:
		return m, nil
	}
}

func (m model) View() string {
	header := m.styles.header.Render("Kave Watch") + "\n" +
		fmt.Sprintf("server: %s | filters: %s | status: %s", displayServer(m.client.Endpoint()), m.filterText(), m.state)

	summary := m.styles.summary.Render(fmt.Sprintf(
		"active:%d  completed:%d  blocked/denied:%d  errors:%d  cost:%s",
		m.stats.ActiveRuns, m.stats.CompletedRuns, m.stats.BlockedOrDenied, m.stats.Errors,
		watch.CostLabel(m.stats.TotalCost, m.stats.Currency),
	))

	rows := make([]string, 0, len(m.events)+1)
	if len(m.events) == 0 {
		rows = append(rows, m.styles.muted.Render("waiting for events..."))
	}
	for i, ev := range m.events {
		line := m.renderRow(ev)
		if i == m.selected {
			line = m.styles.selected.Render(line)
		} else {
			line = m.styles.event.Render(line)
		}
		rows = append(rows, line)
		if m.expanded[i] {
			rows = append(rows, m.styles.muted.Render(m.renderExpanded(ev)))
		}
	}

	footer := m.styles.footer.Render("q quit | j/k or arrows move | enter expand | r reconnect | ? help")
	parts := []string{header, summary, strings.Join(rows, "\n")}
	if m.errMsg != "" {
		parts = append(parts, m.styles.error.Render("error: "+m.errMsg))
	}
	if m.help {
		parts = append(parts, m.styles.help.Render("watch is TUI-only. For scripts use: kave events tail, kave span tail, kave trace export"))
	}
	parts = append(parts, footer)
	return strings.Join(parts, "\n\n")
}

func (m model) renderRow(ev watch.Event) string {
	agent := watch.ShortID(ev.AgentID)
	if agent == "" {
		agent = watch.ShortID(ev.AgentName)
	}
	tool := firstNonEmpty(ev.Tool, ev.Connector, ev.Model)
	if m.filter.Compact {
		return fmt.Sprintf("%s  run:%-8s  %-16s %-10s %s",
			watch.TimeLabel(ev.At),
			watch.ShortID(ev.RunID),
			watch.CompactText(ev.Kind, 16),
			watch.CompactText(ev.Status, 10),
			watch.CompactText(firstNonEmpty(ev.Message, ev.Error), 72),
		)
	}
	return fmt.Sprintf("%s  run:%-8s  agent:%-8s  %-18s %-10s %-14s %s %s",
		watch.TimeLabel(ev.At),
		watch.ShortID(ev.RunID),
		agent,
		watch.CompactText(ev.Kind, 18),
		watch.CompactText(ev.Status, 10),
		watch.CompactText(tool, 14),
		watch.CostLabel(ev.Cost, ev.Currency),
		watch.CompactText(firstNonEmpty(ev.Message, ev.Error), 56),
	)
}

func (m model) renderExpanded(ev watch.Event) string {
	return fmt.Sprintf("run:%s trace:%s span:%s policy:%s tokens:%d/%d error:%s metadata:%s",
		ev.RunID,
		ev.TraceID,
		ev.SpanID,
		ev.PolicyDecision,
		ev.InputTokens,
		ev.OutputTokens,
		watch.CompactText(ev.Error, 80),
		watch.CompactText(fmt.Sprintf("%v", ev.Metadata), 160),
	)
}

func (m model) filterText() string {
	parts := []string{}
	if m.filter.Agent != "" {
		parts = append(parts, "agent="+m.filter.Agent)
	}
	if m.filter.RunID != "" {
		parts = append(parts, "run="+m.filter.RunID)
	}
	if m.filter.TraceID != "" {
		parts = append(parts, "trace="+m.filter.TraceID)
	}
	if m.filter.Status != "" {
		parts = append(parts, "status="+m.filter.Status)
	}
	if m.filter.Type != "" {
		parts = append(parts, "type="+m.filter.Type)
	}
	if len(parts) == 0 {
		return "all"
	}
	return strings.Join(parts, ",")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return "-"
}

func displayServer(v string) string {
	if strings.TrimSpace(v) == "" {
		return "default"
	}
	return v
}
