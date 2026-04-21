package cli

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"go-task-manager/internal/store"
	"go-task-manager/internal/timefmt"
)

type worklogRangeFlags struct {
	from string
	to   string
}

// resolveRange: no --from and no --to → current ISO week.
// now is a parameter so tests can be deterministic.
func resolveRange(rf worklogRangeFlags, now time.Time) (*time.Time, *time.Time, error) {
	if rf.from == "" && rf.to == "" {
		start := startOfISOWeek(now)
		end := start.AddDate(0, 0, 7)
		return &start, &end, nil
	}
	var from, to *time.Time
	if rf.from != "" {
		t, err := timefmt.ParseFlag(rf.from, now)
		if err != nil {
			return nil, nil, fmt.Errorf("--from: %w", err)
		}
		from = &t
	}
	if rf.to != "" {
		t, err := timefmt.ParseFlag(rf.to, now)
		if err != nil {
			return nil, nil, fmt.Errorf("--to: %w", err)
		}
		to = &t
	}
	return from, to, nil
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// startOfISOWeek returns Monday 00:00 of the ISO week containing t.
func startOfISOWeek(t time.Time) time.Time {
	d := startOfDay(t)
	// time.Weekday: Sunday=0..Saturday=6. ISO: Monday=1..Sunday=7.
	offset := int(d.Weekday()) - 1
	if offset < 0 {
		offset = 6 // Sunday → 6 days back to Monday
	}
	return d.AddDate(0, 0, -offset)
}

func (a *App) newWorklogCmd() *cobra.Command {
	var (
		rf     worklogRangeFlags
		taskID int64
		tags   []string
		search string
		limit  int
	)
	cmd := &cobra.Command{
		Use:   "worklog",
		Short: "show time entries grouped by day and task (default: current week)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			from, to, err := resolveRange(rf, time.Now())
			if err != nil {
				return err
			}
			filter := store.TimeEntryFilter{
				From:   from,
				To:     to,
				Tags:   tags,
				Search: search,
				Limit:  limit,
			}
			if taskID > 0 {
				filter.TaskID = &taskID
			}
			entries, err := a.store.AllTimeEntries(c.Context(), filter)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			if len(entries) == 0 {
				fmt.Fprintln(out, "no time entries match")
				return nil
			}
			printDayTaskWorklog(out, entries, time.Now().UTC(), from, to)
			return nil
		},
	}
	cmd.Flags().StringVar(&rf.from, "from", "", "start of range (YYYY-MM-DD, HH:MM, YYYY-MM-DD HH:MM, RFC3339, -1h)")
	cmd.Flags().StringVar(&rf.to, "to", "", "end of range (same formats)")
	cmd.Flags().Int64Var(&taskID, "task", 0, "filter by task id")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "filter by tag (repeatable, AND)")
	cmd.Flags().StringVar(&search, "search", "", "filter by substring in note (case-insensitive)")
	cmd.Flags().IntVar(&limit, "limit", 200, "max entries to show")
	cmd.AddCommand(a.newWorklogSummaryCmd())
	return cmd
}

type taskAgg struct {
	id       int64
	title    string
	duration time.Duration
	count    int
}

// printDayTaskWorklog prints entries grouped by day, then by task within each day.
func printDayTaskWorklog(out io.Writer, entries []store.WorklogEntry, now time.Time, from, to *time.Time) {
	type dayData struct {
		date  time.Time
		tasks map[int64]*taskAgg
		order []int64
		total time.Duration
		count int
	}

	days := make(map[string]*dayData)
	var dayOrder []string

	for _, e := range entries {
		localDate := e.StartedAt.Local()
		dateKey := localDate.Format("2006-01-02")
		if _, ok := days[dateKey]; !ok {
			y, m, d := localDate.Date()
			days[dateKey] = &dayData{
				date:  time.Date(y, m, d, 0, 0, 0, 0, localDate.Location()),
				tasks: make(map[int64]*taskAgg),
			}
			dayOrder = append(dayOrder, dateKey)
		}
		day := days[dateKey]
		dur := e.Duration(now)
		day.total += dur
		day.count++
		if _, ok := day.tasks[e.TaskID]; !ok {
			day.tasks[e.TaskID] = &taskAgg{id: e.TaskID, title: e.TaskTitle}
			day.order = append(day.order, e.TaskID)
		}
		day.tasks[e.TaskID].duration += dur
		day.tasks[e.TaskID].count++
	}

	sort.Strings(dayOrder)

	if from != nil && to != nil {
		fmt.Fprintf(out, "range: %s – %s\n\n",
			from.Local().Format("2006-01-02"),
			to.Local().AddDate(0, 0, -1).Format("2006-01-02"))
	}

	var rangeTotal time.Duration
	var rangeCount int
	for i, dateKey := range dayOrder {
		if i > 0 {
			fmt.Fprintln(out)
		}
		day := days[dateKey]
		fmt.Fprintf(out, "%s %d %s\n", day.date.Weekday(), day.date.Day(), day.date.Format("Jan"))
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		for _, id := range day.order {
			t := day.tasks[id]
			fmt.Fprintf(w, "  #%d\t%s\t%s\n", t.id, t.title, formatDuration(t.duration))
		}
		w.Flush()
		fmt.Fprintf(out, "  day total: %s\n", formatDuration(day.total))
		rangeTotal += day.total
		rangeCount += day.count
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "total: %s (%d sessions)\n", formatDuration(rangeTotal), rangeCount)
}

func (a *App) newWorklogSummaryCmd() *cobra.Command {
	var (
		rf      worklogRangeFlags
		taskID  int64
		tags    []string
		search  string
		groupBy string
	)
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "aggregate time entries by day, task, or tag",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			var group store.WorklogGroupBy
			switch groupBy {
			case "day":
				group = store.GroupByDay
			case "task":
				group = store.GroupByTask
			case "tag":
				group = store.GroupByTag
			default:
				return fmt.Errorf("invalid --group-by %q (want: day|task|tag)", groupBy)
			}
			from, to, err := resolveRange(rf, time.Now())
			if err != nil {
				return err
			}
			filter := store.TimeEntryFilter{
				From: from, To: to, Tags: tags, Search: search,
			}
			if taskID > 0 {
				filter.TaskID = &taskID
			}
			rows, err := a.store.SummarizeTimeEntries(c.Context(), filter, group)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			if len(rows) == 0 {
				fmt.Fprintln(out, "no time entries match")
				return nil
			}
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "KEY\tLABEL\tCOUNT\tDURATION")
			var (
				total   time.Duration
				entries int
			)
			for _, r := range rows {
				total += r.Duration
				entries += r.Count
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\n",
					r.Key, r.Label, r.Count, formatDuration(r.Duration))
			}
			if err := w.Flush(); err != nil {
				return err
			}
			fmt.Fprintf(out, "total: %s (%d entries)\n", formatDuration(total), entries)
			return nil
		},
	}
	cmd.Flags().StringVar(&groupBy, "group-by", "day", "aggregate by: day|task|tag")
	cmd.Flags().StringVar(&rf.from, "from", "", "start of range")
	cmd.Flags().StringVar(&rf.to, "to", "", "end of range")
	cmd.Flags().Int64Var(&taskID, "task", 0, "filter by task id")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "filter by tag (repeatable, AND)")
	cmd.Flags().StringVar(&search, "search", "", "filter by substring in note")
	return cmd
}
