package watchui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kave-io/kave/cli/internal/watch"
)

func Run(ctx context.Context, client watch.Client, filter watch.Filter, opts Options) error {
	m := newModel(ctx, client, filter, opts)
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
