package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (a *App) newRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "rm <id>",
		Aliases: []string{"delete"},
		Short:   "delete a task",
		Args:    cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if err := a.store.Delete(c.Context(), id); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "deleted #%d\n", id)
			return nil
		},
	}
}
