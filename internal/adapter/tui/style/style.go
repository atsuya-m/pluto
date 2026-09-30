package style

import "github.com/charmbracelet/lipgloss"

var (
	Title    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	Subtle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	Cursor   = lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true)
	Selected = lipgloss.NewStyle().Bold(true)
	Type     = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	Value    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	Unset    = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
	Default  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	Error    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	Success  = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	Failure  = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	Help     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	Prompt   = lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true)
)
