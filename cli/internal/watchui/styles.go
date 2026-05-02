package watchui

import "github.com/charmbracelet/lipgloss"

type styles struct {
	header   lipgloss.Style
	summary  lipgloss.Style
	event    lipgloss.Style
	selected lipgloss.Style
	footer   lipgloss.Style
	error    lipgloss.Style
	muted    lipgloss.Style
	help     lipgloss.Style
}

func newStyles(noColor bool) styles {
	if noColor {
		base := lipgloss.NewStyle()
		return styles{
			header:   base.Bold(true),
			summary:  base,
			event:    base,
			selected: base.Bold(true),
			footer:   base,
			error:    base,
			muted:    base,
			help:     base,
		}
	}
	return styles{
		header:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33")),
		summary:  lipgloss.NewStyle().Foreground(lipgloss.Color("250")),
		event:    lipgloss.NewStyle(),
		selected: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229")).Background(lipgloss.Color("24")),
		footer:   lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		error:    lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
		muted:    lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
		help:     lipgloss.NewStyle().Foreground(lipgloss.Color("187")),
	}
}
