package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"go-task-manager/internal/store"
	"go-task-manager/internal/task"
	"go-task-manager/internal/timefmt"
)

func (a *App) newStartCmd() *cobra.Command {
	var note string
	cmd := &cobra.Command{
		Use:   "start <id>",
		Short: "start tracking time on a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			entry, err := a.store.StartTimer(c.Context(), id, note)
			if err != nil {
				if errors.Is(err, store.ErrTimerAlreadyActive) {
					active, _ := a.store.ActiveTimer(c.Context())
					if active != nil {
						return fmt.Errorf("timer already active on task #%d (started %s ago)",
							active.TaskID, formatDuration(active.Duration(time.Now().UTC())))
					}
				}
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "started timer on #%d\n", entry.TaskID)
			return nil
		},
	}
	cmd.Flags().StringVar(&note, "note", "", "optional note for this entry")
	return cmd
}

func (a *App) newStopCmd() *cobra.Command {
	var at string
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "stop the active timer (optionally backdated with --at)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			var atT *time.Time
			if at != "" {
				t, err := parseTimeFlag(at, time.Now())
				if err != nil {
					return err
				}
				atT = &t
			}
			entry, err := a.store.StopTimer(c.Context(), atT)
			if err != nil {
				return err
			}
			d := entry.Duration(time.Now().UTC())
			fmt.Fprintf(c.OutOrStdout(), "stopped timer on #%d (elapsed %s)\n",
				entry.TaskID, formatDuration(d))
			return nil
		},
	}
	cmd.Flags().StringVar(&at, "at", "",
		"end time (HH:MM, YYYY-MM-DD HH:MM, RFC3339, or relative like -20m)")
	return cmd
}

func (a *App) newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "show active timer",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			active, err := a.store.ActiveTimer(c.Context())
			if err != nil {
				return err
			}
			if active == nil {
				fmt.Fprintln(c.OutOrStdout(), "no active timer")
				return nil
			}
			t, err := a.store.Get(c.Context(), active.TaskID)
			if err != nil {
				return err
			}
			d := active.Duration(time.Now().UTC())
			fmt.Fprintf(c.OutOrStdout(), "#%d %s — %s\n",
				t.ID, formatDuration(d), t.Title)
			return nil
		},
	}
}

func (a *App) newLogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log <task-id>",
		Short: "show time entries for a task (use subcommands to edit/delete)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			entries, err := a.store.TimeEntries(c.Context(), id)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Fprintln(c.OutOrStdout(), "no time entries")
				return nil
			}
			now := time.Now().UTC()
			var total time.Duration
			for _, e := range entries {
				total += e.Duration(now)
				printEntry(c, e, now)
			}
			fmt.Fprintf(c.OutOrStdout(), "total: %s\n", formatDuration(total))
			return nil
		},
	}
	cmd.AddCommand(a.newLogRmCmd(), a.newLogEditCmd(), a.newLogAddCmd())
	return cmd
}

func (a *App) newLogAddCmd() *cobra.Command {
	var start, end, note string
	cmd := &cobra.Command{
		Use:   "add <task-id>",
		Short: "manually add a closed time entry (backfill)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			taskID, err := parseID(args[0])
			if err != nil {
				return err
			}
			if start == "" || end == "" {
				return fmt.Errorf("--start and --end are required")
			}
			now := time.Now()
			startT, err := parseTimeFlag(start, now)
			if err != nil {
				return fmt.Errorf("--start: %w", err)
			}
			endT, err := parseTimeFlag(end, now)
			if err != nil {
				return fmt.Errorf("--end: %w", err)
			}
			entry, err := a.store.CreateTimeEntry(c.Context(), store.CreateTimeEntryInput{
				TaskID: taskID, Start: startT, End: endT, Note: note,
			})
			if err != nil {
				return err
			}
			d := entry.Duration(time.Now().UTC())
			fmt.Fprintf(c.OutOrStdout(), "added entry #%d on #%d (%s)\n",
				entry.ID, entry.TaskID, formatDuration(d))
			return nil
		},
	}
	cmd.Flags().StringVar(&start, "start", "", "start time (HH:MM, YYYY-MM-DD HH:MM, RFC3339, or -1h)")
	cmd.Flags().StringVar(&end, "end", "", "end time (same formats as --start)")
	cmd.Flags().StringVar(&note, "note", "", "optional note")
	return cmd
}

func (a *App) newLogRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <entry-id>",
		Short: "delete a time entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			if err := a.store.DeleteTimeEntry(c.Context(), id); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "deleted entry %d\n", id)
			return nil
		},
	}
}

func (a *App) newLogEditCmd() *cobra.Command {
	var start, end, note, date string
	cmd := &cobra.Command{
		Use:   "edit <entry-id>",
		Short: "edit a time entry (--start/--end/--date/--note, only fields you pass are changed)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			var in store.UpdateTimeEntryInput

			changeStart := c.Flags().Changed("start")
			changeEnd := c.Flags().Changed("end")
			changeDate := c.Flags().Changed("date")

			if changeStart || changeEnd || changeDate {
				entry, err := a.store.GetTimeEntry(c.Context(), id)
				if err != nil {
					return err
				}
				origStart := entry.StartedAt.Local()
				ref := origStart

				if changeDate {
					targetDay, err := time.ParseInLocation("2006-01-02", date, time.Local)
					if err != nil {
						return fmt.Errorf("--date: expected YYYY-MM-DD: %w", err)
					}
					y, m, d := targetDay.Date()
					ref = time.Date(y, m, d, origStart.Hour(), origStart.Minute(), origStart.Second(), 0, time.Local)
				}

				if changeStart {
					t, err := parseTimeFlag(start, ref)
					if err != nil {
						return fmt.Errorf("--start: %w", err)
					}
					in.Start = &t
					ref = t // end parses relative to the new start
				} else if changeDate {
					in.Start = &ref
				}

				if changeEnd {
					t, err := parseTimeFlag(end, ref)
					if err != nil {
						return fmt.Errorf("--end: %w", err)
					}
					in.End = &t
				} else if changeDate && entry.EndedAt != nil {
					orig := entry.EndedAt.Local()
					yr, mo, dy := ref.Date()
					t := time.Date(yr, mo, dy, orig.Hour(), orig.Minute(), orig.Second(), 0, time.Local)
					in.End = &t
				}
			}

			if c.Flags().Changed("note") {
				in.Note = &note
			}
			if err := a.store.UpdateTimeEntry(c.Context(), id, in); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "updated entry %d\n", id)
			return nil
		},
	}
	cmd.Flags().StringVar(&start, "start", "", "new start time (HH:MM, YYYY-MM-DD HH:MM, RFC3339, or -1h)")
	cmd.Flags().StringVar(&end, "end", "", "new end time (same formats as --start)")
	cmd.Flags().StringVar(&date, "date", "", "move entry to a different day (YYYY-MM-DD), preserving times")
	cmd.Flags().StringVar(&note, "note", "", "new note")
	return cmd
}

func parseTimeFlag(s string, now time.Time) (time.Time, error) {
	return timefmt.ParseFlag(s, now)
}

func printEntry(c *cobra.Command, e task.TimeEntry, now time.Time) {
	start := e.StartedAt.Local().Format("2006-01-02 15:04")
	endStr := "running"
	if e.EndedAt != nil {
		endStr = e.EndedAt.Local().Format("15:04")
	}
	line := fmt.Sprintf("#%-4d %s → %s  %s", e.ID, start, endStr, formatDuration(e.Duration(now)))
	if e.Note != "" {
		line += "  " + e.Note
	}
	fmt.Fprintln(c.OutOrStdout(), line)
}

func formatDuration(d time.Duration) string { return timefmt.Short(d) }
