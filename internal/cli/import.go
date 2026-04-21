package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"go-task-manager/internal/store"
	"go-task-manager/internal/task"
)

func (a *App) newImportCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "replace entire DB with JSON dump (use '-' for stdin)",
		Long: "Reads a JSON dump produced by `tm export` and REPLACES the whole database.\n" +
			"All existing tasks, tags and time entries are wiped first.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			var r io.Reader
			if args[0] == "-" {
				r = c.InOrStdin()
			} else {
				f, err := os.Open(args[0])
				if err != nil {
					return err
				}
				defer f.Close()
				r = f
			}

			var d dump
			dec := json.NewDecoder(r)
			if err := dec.Decode(&d); err != nil {
				return fmt.Errorf("parse dump: %w", err)
			}

			if err := validateDump(d); err != nil {
				return err
			}

			if !force {
				fmt.Fprintf(c.ErrOrStderr(),
					"This will WIPE the current database and load %d task(s). Continue? [y/N] ",
					len(d.Tasks))
				reader := bufio.NewReader(c.InOrStdin())
				line, _ := reader.ReadString('\n')
				if s := strings.ToLower(strings.TrimSpace(line)); s != "y" && s != "yes" {
					return fmt.Errorf("aborted")
				}
			}

			payload := store.ImportPayload{
				Tags:  d.Tags,
				Tasks: make([]store.ImportTask, 0, len(d.Tasks)),
			}
			for _, dt := range d.Tasks {
				entries := make([]task.TimeEntry, 0, len(dt.TimeEntries))
				for _, e := range dt.TimeEntries {
					entries = append(entries, task.TimeEntry{
						ID:        e.ID,
						TaskID:    dt.ID,
						StartedAt: e.StartedAt,
						EndedAt:   e.EndedAt,
						Note:      e.Note,
					})
				}
				payload.Tasks = append(payload.Tasks, store.ImportTask{
					Task: task.Task{
						ID:        dt.ID,
						Title:     dt.Title,
						Body:      dt.Body,
						Status:    dt.Status,
						Priority:  dt.Priority,
						Tags:      dt.Tags,
						Draft:     dt.Draft,
						DueAt:     dt.DueAt,
						Position:  dt.Position,
						CreatedAt: dt.CreatedAt,
					},
					TimeEntries: entries,
				})
			}

			if err := a.store.ImportReplace(c.Context(), payload); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "imported %d task(s)\n", len(payload.Tasks))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip the confirmation prompt")
	return cmd
}

func validateDump(d dump) error {
	seenTask := map[int64]bool{}
	seenEntry := map[int64]bool{}
	activeCount := 0
	for _, t := range d.Tasks {
		if t.ID <= 0 {
			return fmt.Errorf("task with invalid id %d", t.ID)
		}
		if seenTask[t.ID] {
			return fmt.Errorf("duplicate task id %d", t.ID)
		}
		seenTask[t.ID] = true
		if t.Title == "" {
			return fmt.Errorf("task %d has empty title", t.ID)
		}
		if _, err := task.ParseStatus(string(t.Status)); err != nil {
			return fmt.Errorf("task %d: %w", t.ID, err)
		}
		if _, err := task.ParsePriority(string(t.Priority)); err != nil {
			return fmt.Errorf("task %d: %w", t.ID, err)
		}
		for _, e := range t.TimeEntries {
			if e.ID <= 0 {
				return fmt.Errorf("task %d has time entry with invalid id %d", t.ID, e.ID)
			}
			if seenEntry[e.ID] {
				return fmt.Errorf("duplicate time entry id %d", e.ID)
			}
			seenEntry[e.ID] = true
			if e.EndedAt == nil {
				activeCount++
			} else if e.EndedAt.Before(e.StartedAt) {
				return fmt.Errorf("time entry %d: end before start", e.ID)
			}
		}
	}
	if activeCount > 1 {
		return fmt.Errorf("dump has %d active timers; at most one is allowed", activeCount)
	}
	return nil
}
