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

func (a *App) newListCmd() *cobra.Command {
	var (
		statusFlag string
		prioFlag   string
		tags       []string
		draftOnly  bool
		readyOnly  bool
		overdue    bool
		noDue      bool
		sortFlag   string
	)
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "list tasks (drafts marked with ~)",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if draftOnly && readyOnly {
				return fmt.Errorf("--draft and --ready are mutually exclusive")
			}
			if overdue && noDue {
				return fmt.Errorf("--overdue and --no-due are mutually exclusive")
			}
			var filter store.ListFilter
			if statusFlag != "" {
				st, err := task.ParseStatus(statusFlag)
				if err != nil {
					return err
				}
				filter.Status = st
			}
			if prioFlag != "" {
				p, err := task.ParsePriority(prioFlag)
				if err != nil {
					return err
				}
				filter.Priority = p
			}
			filter.Tags = tags
			switch {
			case draftOnly:
				filter.DraftMode = store.DraftOnly
			case readyOnly:
				filter.DraftMode = store.DraftHide
			}
			filter.Overdue = overdue
			filter.NoDue = noDue
			switch sortFlag {
			case "", "default":
				filter.Sort = store.SortDefault
			case "due":
				filter.Sort = store.SortDue
			default:
				return fmt.Errorf("invalid --sort %q (want: default|due)", sortFlag)
			}
			tasks, err := a.store.List(c.Context(), filter)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			if len(tasks) == 0 {
				fmt.Fprintln(out, "no tasks match — add one with `tm add <title>`")
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
	cmd.Flags().StringVar(&prioFlag, "prio", "", "filter by priority: low|normal|high")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "filter by tag (repeatable, AND)")
	cmd.Flags().BoolVar(&draftOnly, "draft", false, "only drafts")
	cmd.Flags().BoolVar(&readyOnly, "ready", false, "only ready (non-draft)")
	cmd.Flags().BoolVar(&overdue, "overdue", false, "only overdue (past due, not done)")
	cmd.Flags().BoolVar(&noDue, "no-due", false, "only tasks without a deadline")
	cmd.Flags().StringVar(&sortFlag, "sort", "", "sort order: default|due")
	return cmd
}
