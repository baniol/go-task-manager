package cli

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"go-task-manager/internal/config"
	"go-task-manager/internal/tui"
)

func (a *App) newUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ui",
		Short: "interactive TUI (bubbletea)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cfg, _ := config.Load()
			m := tui.New(a.store, cfg)
			p := tea.NewProgram(m, tea.WithAltScreen())
			_, err := p.Run()
			return err
		},
	}
}
