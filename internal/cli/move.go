package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"go-task-manager/internal/task"
)

func (a *App) newMoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "move <id> <status>",
		Aliases: []string{"mv"},
		Short:   "change task status (todo|doing|action|done)",
		Args:    cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			status, err := task.ParseStatus(args[1])
			if err != nil {
				return err
			}
			if err := a.store.Move(c.Context(), id, status); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "task #%d → %s\n", id, status)
			return nil
		},
	}
}
