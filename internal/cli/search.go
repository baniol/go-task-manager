package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"go-task-manager/internal/store"
	"go-task-manager/internal/task"
)

func (a *App) newSearchCmd() *cobra.Command {
	var (
		statusFlag string
		tags       []string
	)
	cmd := &cobra.Command{
		Use:   "search <query...>",
		Short: "full-text search across task titles and bodies",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			var filter store.SearchFilter
			if statusFlag != "" {
				st, err := task.ParseStatus(statusFlag)
				if err != nil {
					return err
				}
				filter.Status = st
			}
			filter.Tags = tags

			query := strings.Join(args, " ")
			tasks, err := a.store.Search(c.Context(), query, filter)
			if err != nil {
				return err
			}

			out := c.OutOrStdout()
			if len(tasks) == 0 {
				fmt.Fprintln(out, "no tasks match")
				return nil
			}

			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tSTATUS\tPRIO\tTITLE\tTAGS\tDUE\tCREATED")
			now := time.Now().UTC()
			for _, t := range tasks {
				title := t.Title
				if t.Draft {
					title = "~ " + title
				}
				tagsCol := "-"
				if len(t.Tags) > 0 {
					tagsCol = strings.Join(t.Tags, ",")
				}
				isOverdue := t.DueAt != nil && t.DueAt.Before(now) && t.Status != task.StatusDone
				dueCol := formatDueShort(t.DueAt, isOverdue)
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
					t.ID, t.Status, t.Priority, title, tagsCol, dueCol,
					t.CreatedAt.Local().Format("2006-01-02 15:04"))
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&statusFlag, "status", "", "filter by status: todo|doing|action|done")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "filter by tag (repeatable, AND)")
	return cmd
}
