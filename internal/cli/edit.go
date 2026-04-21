package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"go-task-manager/internal/store"
)

func (a *App) newEditCmd() *cobra.Command {
	var (
		title    string
		body     string
		ready    bool
		draft    bool
		due      string
		clearDue bool
	)
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "edit a task (only fields you pass are changed)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if ready && draft {
				return fmt.Errorf("--ready and --draft are mutually exclusive")
			}
			if c.Flags().Changed("due") && clearDue {
				return fmt.Errorf("--due and --clear-due are mutually exclusive")
			}

			var in store.EditInput
			if c.Flags().Changed("title") {
				if strings.TrimSpace(title) == "" {
					return fmt.Errorf("--title cannot be empty")
				}
				in.Title = &title
			}
			if c.Flags().Changed("body") {
				in.Body = &body
			}
			switch {
			case ready:
				v := false
				in.Draft = &v
			case draft:
				v := true
				in.Draft = &v
			}
			if c.Flags().Changed("due") {
				t, err := parseDueInput(due)
				if err != nil {
					return err
				}
				in.Due = &t
			}
			if clearDue {
				in.ClearDue = true
			}

			if in.Title == nil && in.Body == nil && in.Draft == nil && in.Due == nil && !in.ClearDue {
				return fmt.Errorf("nothing to update — pass at least one of --title/--body/--ready/--draft/--due/--clear-due")
			}

			if err := a.store.Update(c.Context(), id, in); err != nil {
				return err
			}
			if err := a.printTask(c, id); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "new title")
	cmd.Flags().StringVar(&body, "body", "", "new body")
	cmd.Flags().BoolVar(&ready, "ready", false, "mark as ready (non-draft)")
	cmd.Flags().BoolVar(&draft, "draft", false, "mark as draft")
	cmd.Flags().StringVar(&due, "due", "", "new deadline: today|tomorrow|+Nd|YYYY-MM-DD")
	cmd.Flags().BoolVar(&clearDue, "clear-due", false, "remove deadline")
	return cmd
}
