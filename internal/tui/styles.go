package tui

import "charm.land/lipgloss/v2"

// styles groups every lipgloss style so View stays declarative.
type styles struct {
	title    lipgloss.Style
	subtitle lipgloss.Style
	selected lipgloss.Style
	normal   lipgloss.Style
	dim      lipgloss.Style
	ok       lipgloss.Style
	err      lipgloss.Style
	box      lipgloss.Style
	modal    lipgloss.Style
	status   lipgloss.Style
}

func defaultStyles() styles {
	return styles{
		title:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")),
		subtitle: lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		selected: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")),
		normal:   lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		dim:      lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		ok:       lipgloss.NewStyle().Foreground(lipgloss.Color("42")),
		err:      lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		box: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(0, 1),
		modal: lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("212")).
			Padding(1, 2),
		status: lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
	}
}
