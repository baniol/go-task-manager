package cli

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"go-task-manager/internal/store"
)

func (a *App) newResetCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "wipe the entire database (tasks, tags, time entries)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if !force {
				fmt.Fprint(c.ErrOrStderr(),
					"This will WIPE the entire database. Type 'yes' to confirm: ")
				line, _ := bufio.NewReader(c.InOrStdin()).ReadString('\n')
				if strings.TrimSpace(line) != "yes" {
					return fmt.Errorf("aborted")
				}
			}
			if err := a.store.ImportReplace(c.Context(), store.ImportPayload{}); err != nil {
				return err
			}
			fmt.Fprintln(c.OutOrStdout(), "database wiped")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip the confirmation prompt")
	return cmd
}
