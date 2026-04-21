package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStartStopTimer(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tk := mustAdd(t, s, AddInput{Title: "T"})

	entry, err := s.StartTimer(ctx, tk.ID, "focus")
	if err != nil {
		t.Fatalf("StartTimer: %v", err)
	}
	if entry.TaskID != tk.ID || !entry.Active() || entry.Note != "focus" {
		t.Fatalf("unexpected entry: %+v", entry)
	}

	active, err := s.ActiveTimer(ctx)
	if err != nil {
		t.Fatalf("ActiveTimer: %v", err)
	}
	if active == nil || active.ID != entry.ID {
		t.Fatalf("ActiveTimer = %+v, want id %d", active, entry.ID)
	}

	stopped, err := s.StopTimer(ctx, nil)
	if err != nil {
		t.Fatalf("StopTimer: %v", err)
	}
	if stopped.Active() {
		t.Fatal("stopped entry still active")
	}
	if stopped.Duration(time.Now().UTC()) < 0 {
		t.Fatalf("negative duration: %v", stopped.Duration(time.Now().UTC()))
	}

	active, err = s.ActiveTimer(ctx)
	if err != nil {
		t.Fatalf("ActiveTimer post-stop: %v", err)
	}
	if active != nil {
		t.Fatalf("expected no active timer, got %+v", active)
	}
}

func TestStartTimerRejectsConcurrent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a := mustAdd(t, s, AddInput{Title: "A"})
	b := mustAdd(t, s, AddInput{Title: "B"})

	if _, err := s.StartTimer(ctx, a.ID, ""); err != nil {
		t.Fatalf("first StartTimer: %v", err)
	}
	if _, err := s.StartTimer(ctx, a.ID, ""); !errors.Is(err, ErrTimerAlreadyActive) {
		t.Fatalf("want ErrTimerAlreadyActive on same task, got %v", err)
	}
	if _, err := s.StartTimer(ctx, b.ID, ""); !errors.Is(err, ErrTimerAlreadyActive) {
		t.Fatalf("want ErrTimerAlreadyActive on other task, got %v", err)
	}

	if _, err := s.StopTimer(ctx, nil); err != nil {
		t.Fatalf("StopTimer: %v", err)
	}
	if _, err := s.StartTimer(ctx, b.ID, ""); err != nil {
		t.Fatalf("StartTimer after stop: %v", err)
	}
}

func TestStopTimerWithoutActive(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.StopTimer(context.Background(), nil); !errors.Is(err, ErrNoActiveTimer) {
		t.Fatalf("want ErrNoActiveTimer, got %v", err)
	}
}

func TestStartTimerRejectsMissingTask(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.StartTimer(context.Background(), 999, ""); err == nil {
		t.Fatal("want error for missing task")
	}
}

func TestTaskTotalDurationSumsEntries(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tk := mustAdd(t, s, AddInput{Title: "T"})

	// Entry closed manually (via raw SQL so we can set known values).
	start1 := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	end1 := time.Now().UTC().Add(-90 * time.Minute).Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO time_entries (task_id, started_at, ended_at) VALUES (?, ?, ?)`,
		tk.ID, start1, end1); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Active entry started 10 minutes ago.
	start2 := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO time_entries (task_id, started_at) VALUES (?, ?)`,
		tk.ID, start2); err != nil {
		t.Fatalf("insert active: %v", err)
	}

	total, err := s.TaskTotalDuration(ctx, tk.ID)
	if err != nil {
		t.Fatalf("TaskTotalDuration: %v", err)
	}
	// Closed = 30min, active ~= 10min → together ~40min. Allow 1min slack for latency.
	min, max := 39*time.Minute, 41*time.Minute
	if total < min || total > max {
		t.Fatalf("total = %v, want ~40m", total)
	}
}

func TestDeleteTaskCascadesTimeEntries(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tk := mustAdd(t, s, AddInput{Title: "T"})

	if _, err := s.StartTimer(ctx, tk.ID, ""); err != nil {
		t.Fatalf("StartTimer: %v", err)
	}
	if err := s.Delete(ctx, tk.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	active, err := s.ActiveTimer(ctx)
	if err != nil {
		t.Fatalf("ActiveTimer: %v", err)
	}
	if active != nil {
		t.Fatalf("cascade failed: active timer still present: %+v", active)
	}
}
