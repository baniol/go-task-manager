package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"go-task-manager/internal/store"
	"go-task-manager/internal/task"
)

func (a *App) newAddCmd() *cobra.Command {
	var (
		prio  string
		body  string
		tags  []string
		ready bool
		due   string
	)
	cmd := &cobra.Command{
		Use:   "add <title...>",
		Short: "add a new task (default: draft)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			priority, err := task.ParsePriority(prio)
			if err != nil {
				return err
			}
			in := store.AddInput{
				Title:    strings.Join(args, " "),
				Body:     body,
				Priority: priority,
				Tags:     tags,
				Ready:    ready,
			}
			if c.Flags().Changed("due") {
				t, err := parseDueInput(due)
				if err != nil {
					return err
				}
				in.Due = &t
			}
			t, err := a.store.Add(c.Context(), in)
			if err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "added #%d %s\n", t.ID, formatTaskShort(t))
			return nil
		},
	}
	cmd.Flags().StringVar(&prio, "prio", string(task.PriorityNormal), "priority: low|normal|high")
	cmd.Flags().StringVar(&body, "body", "", "task body / description")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "tag (repeatable; created on first use)")
	cmd.Flags().BoolVar(&ready, "ready", false, "create as ready (non-draft)")
	cmd.Flags().StringVar(&due, "due", "", "deadline: today|tomorrow|+Nd|YYYY-MM-DD")
	return cmd
}

// formatTaskShort renders a short task description for confirmation messages.
// Format: [draft|prio] title #epic +tag1 +tag2 @YYYY-MM-DD
func formatTaskShort(t task.Task) string {
	var b strings.Builder
	b.WriteString("[")
	if t.Draft {
		b.WriteString("draft|")
	}
	b.WriteString(string(t.Priority))
	b.WriteString("] ")
	b.WriteString(t.Title)
	for _, tag := range t.Tags {
		b.WriteString(" +")
		b.WriteString(tag)
	}
	if t.DueAt != nil {
		b.WriteString(" @")
		b.WriteString(t.DueAt.Local().Format("2006-01-02"))
	}
	return b.String()
}
