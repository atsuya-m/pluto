package app

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

func Run(ctx context.Context, deps Dependencies) error {
	program := tea.NewProgram(NewModel(ctx, deps), tea.WithContext(ctx))
	_, err := program.Run()
	return err
}
