package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"go-task-manager/internal/store"
)

func TestResolveRangeDefault(t *testing.T) {
	// No flags → current ISO week. 2026-04-17 is a Friday.
	now := time.Date(2026, 4, 17, 15, 30, 0, 0, time.Local)
	from, to, err := resolveRange(worklogRangeFlags{}, now)
	if err != nil {
		t.Fatal(err)
	}
	wantFrom := time.Date(2026, 4, 13, 0, 0, 0, 0, time.Local)
	wantTo := time.Date(2026, 4, 20, 0, 0, 0, 0, time.Local)
	if !from.Equal(wantFrom) || !to.Equal(wantTo) {
		t.Errorf("default: [%v, %v), want [%v, %v)", from, to, wantFrom, wantTo)
	}

	// Sunday → 6 days back to Monday.
	sunday := time.Date(2026, 4, 19, 12, 0, 0, 0, time.Local)
	from, to, err = resolveRange(worklogRangeFlags{}, sunday)
	if err != nil {
		t.Fatal(err)
	}
	if !from.Equal(wantFrom) || !to.Equal(wantTo) {
		t.Errorf("sunday: [%v, %v), want [%v, %v)", from, to, wantFrom, wantTo)
	}
}

func TestResolveRangeExplicit(t *testing.T) {
	now := time.Date(2026, 4, 17, 15, 30, 0, 0, time.Local)
	from, to, err := resolveRange(worklogRangeFlags{from: "2026-04-14", to: "2026-04-18"}, now)
	if err != nil {
		t.Fatal(err)
	}
	wantFrom := time.Date(2026, 4, 14, 0, 0, 0, 0, time.Local)
	wantTo := time.Date(2026, 4, 18, 0, 0, 0, 0, time.Local)
	if !from.Equal(wantFrom) || !to.Equal(wantTo) {
		t.Errorf("explicit: [%v, %v), want [%v, %v)", from, to, wantFrom, wantTo)
	}
}

// seedEntry inserts an entry directly into the store.
func seedEntry(t *testing.T, h *testHarness, taskID int64, start, end time.Time, note string) {
	t.Helper()
	_, err := h.store.CreateTimeEntry(context.Background(), store.CreateTimeEntryInput{
		TaskID: taskID, Start: start, End: end, Note: note,
	})
	if err != nil {
		t.Fatalf("seed entry: %v", err)
	}
}

func TestLogAddCreatesEntry(t *testing.T) {
	h := newHarness(t)
	if err := h.run("add", "backfill task"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := h.run("log", "add", "1",
		"--start", "2026-04-17 09:00",
		"--end", "2026-04-17 10:30",
		"--note", "refactor auth"); err != nil {
		t.Fatalf("log add: %v", err)
	}
	out := h.stdout.String()
	if !strings.Contains(out, "added entry") || !strings.Contains(out, "#1") {
		t.Errorf("unexpected output: %q", out)
	}

	// The entry should be visible in `tm log 1`
	if err := h.run("log", "1"); err != nil {
		t.Fatalf("log 1: %v", err)
	}
	if !strings.Contains(h.stdout.String(), "refactor auth") {
		t.Errorf("log output missing note: %q", h.stdout.String())
	}
}

func TestLogAddRequiresStartAndEnd(t *testing.T) {
	h := newHarness(t)
	if err := h.run("add", "t"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := h.run("log", "add", "1", "--start", "09:00"); err == nil {
		t.Error("want error when --end missing")
	}
	if err := h.run("log", "add", "1", "--end", "10:00"); err == nil {
		t.Error("want error when --start missing")
	}
}

func TestLogAddRejectsEndBeforeStart(t *testing.T) {
	h := newHarness(t)
	if err := h.run("add", "t"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := h.run("log", "add", "1",
		"--start", "2026-04-17 10:00",
		"--end", "2026-04-17 09:00"); err == nil {
		t.Error("want error when end before start")
	}
}

func TestWorklogListsAllTasks(t *testing.T) {
	h := newHarness(t)
	if err := h.run("add", "task A"); err != nil {
		t.Fatal(err)
	}
	if err := h.run("add", "task B"); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)
	seedEntry(t, h, 1, base, base.Add(30*time.Minute), "aaa")
	seedEntry(t, h, 2, base.Add(time.Hour), base.Add(2*time.Hour), "bbb")

	if err := h.run("worklog", "--from", "2026-04-17", "--to", "2026-04-18"); err != nil {
		t.Fatalf("worklog: %v", err)
	}
	out := h.stdout.String()
	if !strings.Contains(out, "task A") || !strings.Contains(out, "task B") {
		t.Errorf("missing titles: %q", out)
	}
	if !strings.Contains(out, "total:") {
		t.Errorf("missing total: %q", out)
	}
}

func TestWorklogEmptyMessage(t *testing.T) {
	h := newHarness(t)
	if err := h.run("worklog"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.stdout.String(), "no time entries") {
		t.Errorf("output: %q", h.stdout.String())
	}
}

func TestWorklogTaskAndSearchFilter(t *testing.T) {
	h := newHarness(t)
	if err := h.run("add", "A"); err != nil {
		t.Fatal(err)
	}
	if err := h.run("add", "B"); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)
	seedEntry(t, h, 1, base, base.Add(time.Hour), "auth work")
	seedEntry(t, h, 2, base.Add(2*time.Hour), base.Add(3*time.Hour), "lunch")

	if err := h.run("worklog", "--from", "2026-04-17", "--to", "2026-04-18", "--task", "1"); err != nil {
		t.Fatalf("worklog --task: %v", err)
	}
	if !strings.Contains(h.stdout.String(), "A") ||
		strings.Contains(h.stdout.String(), "B") {
		t.Errorf("--task filter failed: %q", h.stdout.String())
	}

	if err := h.run("worklog", "--from", "2026-04-17", "--to", "2026-04-18", "--search", "AUTH"); err != nil {
		t.Fatalf("worklog --search: %v", err)
	}
	if !strings.Contains(h.stdout.String(), "A") ||
		strings.Contains(h.stdout.String(), "B") {
		t.Errorf("--search filter failed: %q", h.stdout.String())
	}
}

func TestWorklogSummaryByTask(t *testing.T) {
	h := newHarness(t)
	if err := h.run("add", "Alpha"); err != nil {
		t.Fatal(err)
	}
	if err := h.run("add", "Beta"); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC)
	seedEntry(t, h, 1, base, base.Add(2*time.Hour), "")
	seedEntry(t, h, 2, base, base.Add(30*time.Minute), "")

	if err := h.run("worklog", "summary", "--from", "2026-04-17", "--to", "2026-04-18", "--group-by", "task"); err != nil {
		t.Fatalf("worklog summary: %v", err)
	}
	out := h.stdout.String()
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Beta") {
		t.Errorf("missing labels: %q", out)
	}
	if !strings.Contains(out, "2h0m") {
		t.Errorf("missing Alpha duration: %q", out)
	}
	if !strings.Contains(out, "total:") {
		t.Errorf("missing total: %q", out)
	}
}

func TestWorklogSummaryInvalidGroupBy(t *testing.T) {
	h := newHarness(t)
	if err := h.run("worklog", "summary", "--group-by", "weekday"); err == nil {
		t.Error("want error for invalid group-by")
	}
}

func TestWorklogSummaryEmpty(t *testing.T) {
	h := newHarness(t)
	if err := h.run("worklog", "summary", "--group-by", "day"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.stdout.String(), "no time entries") {
		t.Errorf("output: %q", h.stdout.String())
	}
}
