package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (a *App) newTagCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "tag",
		Aliases: []string{"tags"},
		Short:   "manage tags",
	}
	cmd.AddCommand(
		a.newTagListCmd(),
		a.newTagAddCmd(),
		a.newTagRmCmd(),
		a.newTagDeleteCmd(),
	)
	return cmd
}

func (a *App) newTagListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "list all tag names",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			names, err := a.store.Tags(c.Context())
			if err != nil {
				return err
			}
			if len(names) == 0 {
				fmt.Fprintln(c.OutOrStdout(), "no tags yet")
				return nil
			}
			out := c.OutOrStdout()
			for _, n := range names {
				fmt.Fprintln(out, n)
			}
			return nil
		},
	}
}

func (a *App) newTagAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <task-id> <tag>...",
		Short: "add tags to a task",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if err := a.store.AddTaskTags(c.Context(), id, args[1:]); err != nil {
				return err
			}
			if err := a.printTask(c, id); err != nil {
				return err
			}
			return nil
		},
	}
}

func (a *App) newTagRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <task-id> <tag>...",
		Short: "remove tags from a task",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if err := a.store.RemoveTaskTags(c.Context(), id, args[1:]); err != nil {
				return err
			}
			if err := a.printTask(c, id); err != nil {
				return err
			}
			return nil
		},
	}
}

func (a *App) newTagDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "delete a tag from the system (unlinks from all tasks)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			n, err := a.store.DeleteTag(c.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "deleted tag %q, unlinked from %d task(s)\n", args[0], n)
			return nil
		},
	}
}
