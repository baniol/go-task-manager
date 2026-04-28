package cli

import (
	"encoding/json"
	"time"

	"github.com/spf13/cobra"

	"go-task-manager/internal/store"
)

func (a *App) newExportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "export",
		Short: "export all tasks, tags and time entries as JSON",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()
			tasks, err := a.store.List(ctx, store.ListFilter{})
			if err != nil {
				return err
			}
			tags, err := a.store.Tags(ctx)
			if err != nil {
				return err
			}
			out := dump{
				ExportedAt: time.Now().UTC(),
				Tasks:      make([]dumpTask, 0, len(tasks)),
				Tags:       tags,
			}
			for _, t := range tasks {
				entries, err := a.store.TimeEntries(ctx, t.ID)
				if err != nil {
					return err
				}
				exEntries := make([]dumpTimeEntry, 0, len(entries))
				for _, e := range entries {
					exEntries = append(exEntries, dumpTimeEntry{
						ID:        e.ID,
						StartedAt: e.StartedAt,
						EndedAt:   e.EndedAt,
						Note:      e.Note,
					})
				}
				tagsCol := t.Tags
				if tagsCol == nil {
					tagsCol = []string{}
				}
				out.Tasks = append(out.Tasks, dumpTask{
					ID:          t.ID,
					UUID:        t.UUID,
					Title:       t.Title,
					Body:        t.Body,
					Status:      t.Status,
					Priority:    t.Priority,
					Tags:        tagsCol,
					Draft:       t.Draft,
					DueAt:       t.DueAt,
					Position:    t.Position,
					CreatedAt:   t.CreatedAt,
					UpdatedAt:   t.UpdatedAt,
					DeletedAt:   t.DeletedAt,
					TimeEntries: exEntries,
				})
			}
			enc := json.NewEncoder(c.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		},
	}
}
