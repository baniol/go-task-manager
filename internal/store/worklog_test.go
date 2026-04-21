package store

import (
	"context"
	"testing"
	"time"

	"go-task-manager/internal/task"
)

// insertEntry inserts a closed entry with the given timestamps.
func insertEntry(t *testing.T, s *SQLite, taskID int64, start, end time.Time, note string) int64 {
	t.Helper()
	res, err := s.db.ExecContext(context.Background(),
		`INSERT INTO time_entries (task_id, started_at, ended_at, note) VALUES (?, ?, ?, ?)`,
		taskID,
		start.UTC().Format(time.RFC3339),
		end.UTC().Format(time.RFC3339),
		note)
	if err != nil {
		t.Fatalf("insert entry: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last id: %v", err)
	}
	return id
}

func TestAllTimeEntriesFromToFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tk := mustAdd(t, s, AddInput{Title: "T"})

	base := time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)
	insertEntry(t, s, tk.ID, base.Add(-48*time.Hour), base.Add(-47*time.Hour), "old")
	insertEntry(t, s, tk.ID, base, base.Add(30*time.Minute), "in-range")
	insertEntry(t, s, tk.ID, base.Add(48*time.Hour), base.Add(49*time.Hour), "future")

	from := base.Add(-1 * time.Hour)
	to := base.Add(24 * time.Hour)
	got, err := s.AllTimeEntries(ctx, TimeEntryFilter{From: &from, To: &to})
	if err != nil {
		t.Fatalf("AllTimeEntries: %v", err)
	}
	if len(got) != 1 || got[0].Note != "in-range" {
		t.Fatalf("got %+v, want single in-range", got)
	}
	if got[0].TaskTitle != "T" {
		t.Errorf("TaskTitle = %q, want T", got[0].TaskTitle)
	}
}

func TestAllTimeEntriesTaskAndSearchFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a := mustAdd(t, s, AddInput{Title: "A"})
	b := mustAdd(t, s, AddInput{Title: "B"})

	base := time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)
	insertEntry(t, s, a.ID, base, base.Add(time.Hour), "refactor AUTH middleware")
	insertEntry(t, s, a.ID, base.Add(2*time.Hour), base.Add(3*time.Hour), "lunch")
	insertEntry(t, s, b.ID, base.Add(4*time.Hour), base.Add(5*time.Hour), "auth tests")

	// Task filter
	got, err := s.AllTimeEntries(ctx, TimeEntryFilter{TaskID: &a.ID})
	if err != nil {
		t.Fatalf("AllTimeEntries task: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("task filter: got %d, want 2", len(got))
	}

	// Search — case-insensitive, cross-task
	got, err = s.AllTimeEntries(ctx, TimeEntryFilter{Search: "auth"})
	if err != nil {
		t.Fatalf("AllTimeEntries search: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("search: got %d, want 2", len(got))
	}
}

func TestAllTimeEntriesTagFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	backend := mustAdd(t, s, AddInput{Title: "be", Tags: []string{"backend", "api"}})
	frontend := mustAdd(t, s, AddInput{Title: "fe", Tags: []string{"frontend"}})

	base := time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)
	insertEntry(t, s, backend.ID, base, base.Add(time.Hour), "")
	insertEntry(t, s, frontend.ID, base, base.Add(time.Hour), "")

	got, err := s.AllTimeEntries(ctx, TimeEntryFilter{Tags: []string{"backend"}})
	if err != nil {
		t.Fatalf("AllTimeEntries tag: %v", err)
	}
	if len(got) != 1 || got[0].TaskID != backend.ID {
		t.Errorf("tag filter: got %+v, want only backend task", got)
	}

	// AND semantics — backend + api → only the task with both
	got, err = s.AllTimeEntries(ctx, TimeEntryFilter{Tags: []string{"backend", "api"}})
	if err != nil {
		t.Fatalf("AllTimeEntries tag AND: %v", err)
	}
	if len(got) != 1 || got[0].TaskID != backend.ID {
		t.Errorf("tag AND: got %+v", got)
	}

	// Nonexistent combination
	got, err = s.AllTimeEntries(ctx, TimeEntryFilter{Tags: []string{"backend", "frontend"}})
	if err != nil {
		t.Fatalf("AllTimeEntries tag impossible: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("impossible tag combo: got %d, want 0", len(got))
	}
}

func TestAllTimeEntriesLimitAndOrder(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tk := mustAdd(t, s, AddInput{Title: "T"})

	base := time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		start := base.Add(time.Duration(i) * time.Hour)
		insertEntry(t, s, tk.ID, start, start.Add(30*time.Minute), "")
	}

	got, err := s.AllTimeEntries(ctx, TimeEntryFilter{Limit: 3})
	if err != nil {
		t.Fatalf("AllTimeEntries: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("limit: got %d, want 3", len(got))
	}
	// DESC — newest first
	if !got[0].StartedAt.After(got[1].StartedAt) || !got[1].StartedAt.After(got[2].StartedAt) {
		t.Errorf("order not DESC: %+v", got)
	}
}

func TestAllTimeEntriesRunningTimer(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tk := mustAdd(t, s, AddInput{Title: "T"})

	if _, err := s.StartTimer(ctx, tk.ID, "running"); err != nil {
		t.Fatalf("StartTimer: %v", err)
	}
	got, err := s.AllTimeEntries(ctx, TimeEntryFilter{})
	if err != nil {
		t.Fatalf("AllTimeEntries: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].EndedAt != nil {
		t.Errorf("EndedAt = %v, want nil for running entry", got[0].EndedAt)
	}
}

func TestCreateTimeEntryBasic(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tk := mustAdd(t, s, AddInput{Title: "T"})

	start := time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)
	end := start.Add(90 * time.Minute)
	entry, err := s.CreateTimeEntry(ctx, CreateTimeEntryInput{
		TaskID: tk.ID, Start: start, End: end, Note: "backfill",
	})
	if err != nil {
		t.Fatalf("CreateTimeEntry: %v", err)
	}
	if entry.TaskID != tk.ID || entry.Active() {
		t.Errorf("entry = %+v, want closed on task %d", entry, tk.ID)
	}
	if entry.Duration(time.Now()) != 90*time.Minute {
		t.Errorf("duration = %v, want 90m", entry.Duration(time.Now()))
	}

	entries, err := s.TimeEntries(ctx, tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Note != "backfill" {
		t.Errorf("TimeEntries = %+v", entries)
	}
}

func TestCreateTimeEntryRejectsEndBeforeStart(t *testing.T) {
	s := newTestStore(t)
	tk := mustAdd(t, s, AddInput{Title: "T"})

	start := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	end := start.Add(-time.Hour)
	if _, err := s.CreateTimeEntry(context.Background(), CreateTimeEntryInput{
		TaskID: tk.ID, Start: start, End: end,
	}); err == nil {
		t.Error("want error for end before start")
	}
}

func TestCreateTimeEntryRejectsMissingTask(t *testing.T) {
	s := newTestStore(t)
	start := time.Now()
	end := start.Add(time.Hour)
	if _, err := s.CreateTimeEntry(context.Background(), CreateTimeEntryInput{
		TaskID: 9999, Start: start, End: end,
	}); err == nil {
		t.Error("want error for missing task")
	}
}

func TestCreateTimeEntryCoexistsWithActiveTimer(t *testing.T) {
	// Unique index `idx_time_entries_one_active` only covers ended_at IS NULL.
	// Manual add always has ended_at != NULL, so there's no collision.
	s := newTestStore(t)
	ctx := context.Background()
	tk := mustAdd(t, s, AddInput{Title: "T"})

	if _, err := s.StartTimer(ctx, tk.ID, ""); err != nil {
		t.Fatalf("StartTimer: %v", err)
	}
	start := time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	if _, err := s.CreateTimeEntry(ctx, CreateTimeEntryInput{
		TaskID: tk.ID, Start: start, End: end,
	}); err != nil {
		t.Fatalf("CreateTimeEntry while timer active: %v", err)
	}
}

func TestSummarizeByTask(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a := mustAdd(t, s, AddInput{Title: "Alpha"})
	b := mustAdd(t, s, AddInput{Title: "Beta"})

	base := time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)
	insertEntry(t, s, a.ID, base, base.Add(time.Hour), "")
	insertEntry(t, s, a.ID, base.Add(2*time.Hour), base.Add(3*time.Hour), "")
	insertEntry(t, s, b.ID, base, base.Add(30*time.Minute), "")

	got, err := s.SummarizeTimeEntries(ctx, TimeEntryFilter{}, GroupByTask)
	if err != nil {
		t.Fatalf("SummarizeTimeEntries: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// ORDER BY secs DESC — Alpha (2h) before Beta (30m)
	if got[0].Label != "Alpha" || got[0].Duration != 2*time.Hour || got[0].Count != 2 {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].Label != "Beta" || got[1].Duration != 30*time.Minute || got[1].Count != 1 {
		t.Errorf("got[1] = %+v", got[1])
	}
}

func TestSummarizeByDay(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tk := mustAdd(t, s, AddInput{Title: "T"})

	// Two entries on the same local day — 9:00 and 14:00,
	// plus one on the previous day. Use local TZ so that
	// SQLite's date(..., 'localtime') returns a consistent date.
	day1 := time.Date(2026, 4, 17, 9, 0, 0, 0, time.Local)
	day2 := time.Date(2026, 4, 18, 11, 0, 0, 0, time.Local)
	insertEntry(t, s, tk.ID, day1, day1.Add(time.Hour), "")
	insertEntry(t, s, tk.ID, day1.Add(5*time.Hour), day1.Add(6*time.Hour), "")
	insertEntry(t, s, tk.ID, day2, day2.Add(30*time.Minute), "")

	got, err := s.SummarizeTimeEntries(ctx, TimeEntryFilter{}, GroupByDay)
	if err != nil {
		t.Fatalf("SummarizeTimeEntries: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (rows: %+v)", len(got), got)
	}
	// Sort: ORDER BY k ASC
	if got[0].Key != "2026-04-17" || got[0].Count != 2 || got[0].Duration != 2*time.Hour {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].Key != "2026-04-18" || got[1].Count != 1 || got[1].Duration != 30*time.Minute {
		t.Errorf("got[1] = %+v", got[1])
	}
}

func TestSummarizeByTagIncludesNone(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tagged := mustAdd(t, s, AddInput{Title: "tagged", Tags: []string{"backend"}})
	untagged := mustAdd(t, s, AddInput{Title: "untagged"})

	base := time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)
	insertEntry(t, s, tagged.ID, base, base.Add(time.Hour), "")
	insertEntry(t, s, untagged.ID, base, base.Add(2*time.Hour), "")

	got, err := s.SummarizeTimeEntries(ctx, TimeEntryFilter{}, GroupByTag)
	if err != nil {
		t.Fatalf("SummarizeTimeEntries: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (rows: %+v)", len(got), got)
	}
	// ORDER BY secs DESC — untagged (2h) > backend (1h)
	if got[0].Key != "(none)" || got[0].Duration != 2*time.Hour {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].Key != "backend" || got[1].Duration != time.Hour {
		t.Errorf("got[1] = %+v", got[1])
	}
}

func TestSummarizeRespectsFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a := mustAdd(t, s, AddInput{Title: "A"})
	b := mustAdd(t, s, AddInput{Title: "B"})

	base := time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)
	insertEntry(t, s, a.ID, base, base.Add(time.Hour), "")
	insertEntry(t, s, b.ID, base, base.Add(2*time.Hour), "")

	got, err := s.SummarizeTimeEntries(ctx, TimeEntryFilter{TaskID: &a.ID}, GroupByTask)
	if err != nil {
		t.Fatalf("SummarizeTimeEntries: %v", err)
	}
	if len(got) != 1 || got[0].Label != "A" {
		t.Errorf("filter task = %+v, want only A", got)
	}
}

func TestSummarizeInvalidGroupBy(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.SummarizeTimeEntries(context.Background(), TimeEntryFilter{}, "bogus"); err == nil {
		t.Error("want error for invalid group-by")
	}
}

func TestSummarizeTagFilterWithGroupByTagRejected(t *testing.T) {
	s := newTestStore(t)
	_, err := s.SummarizeTimeEntries(context.Background(),
		TimeEntryFilter{Tags: []string{"x"}}, GroupByTag)
	if err == nil {
		t.Error("want error for --tag + group-by=tag combo")
	}
}

// sanity — checks that WorklogEntry embeds task.TimeEntry (Note field etc.)
func TestWorklogEntryEmbedsTimeEntry(t *testing.T) {
	var e WorklogEntry = WorklogEntry{
		TimeEntry: task.TimeEntry{ID: 7, Note: "x"},
		TaskTitle: "Title",
	}
	if e.ID != 7 || e.Note != "x" || e.TaskTitle != "Title" {
		t.Errorf("embed broken: %+v", e)
	}
}
