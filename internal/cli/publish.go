package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (a *App) newPublishCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "publish <id>",
		Short: "flip a draft task to ready",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if err := a.store.Publish(c.Context(), id); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "task #%d → ready\n", id)
			return nil
		},
	}
}
