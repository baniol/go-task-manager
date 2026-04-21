package store

import (
	"context"
	"testing"
	"time"
)

func TestDeleteTimeEntry(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tk := mustAdd(t, s, AddInput{Title: "T"})
	e, err := s.StartTimer(ctx, tk.ID, "")
	if err != nil {
		t.Fatalf("StartTimer: %v", err)
	}
	if _, err := s.StopTimer(ctx, nil); err != nil {
		t.Fatalf("StopTimer: %v", err)
	}
	if err := s.DeleteTimeEntry(ctx, e.ID); err != nil {
		t.Fatalf("DeleteTimeEntry: %v", err)
	}
	entries, _ := s.TimeEntries(ctx, tk.ID)
	if len(entries) != 0 {
		t.Fatalf("want 0 entries, got %d", len(entries))
	}
	if err := s.DeleteTimeEntry(ctx, 999); err == nil {
		t.Fatal("want error for missing entry")
	}
}

func TestUpdateTimeEntry(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tk := mustAdd(t, s, AddInput{Title: "T"})
	e, _ := s.StartTimer(ctx, tk.ID, "old")
	s.StopTimer(ctx, nil)

	newStart := time.Now().Add(-2 * time.Hour)
	newEnd := time.Now().Add(-1 * time.Hour)
	newNote := "fixed"
	if err := s.UpdateTimeEntry(ctx, e.ID, UpdateTimeEntryInput{
		Start: &newStart, End: &newEnd, Note: &newNote,
	}); err != nil {
		t.Fatalf("UpdateTimeEntry: %v", err)
	}

	entries, _ := s.TimeEntries(ctx, tk.ID)
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	got := entries[0]
	if got.Note != "fixed" {
		t.Errorf("note = %q", got.Note)
	}
	d := got.Duration(time.Now())
	if d < 59*time.Minute || d > 61*time.Minute {
		t.Errorf("duration = %v, want ~1h", d)
	}
}

func TestUpdateTimeEntryValidates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tk := mustAdd(t, s, AddInput{Title: "T"})
	e, _ := s.StartTimer(ctx, tk.ID, "")
	s.StopTimer(ctx, nil)

	// End before start → error.
	past := time.Now().Add(-24 * time.Hour)
	if err := s.UpdateTimeEntry(ctx, e.ID, UpdateTimeEntryInput{End: &past}); err == nil {
		t.Fatal("want error for end before start")
	}

	// Nothing to change → error.
	if err := s.UpdateTimeEntry(ctx, e.ID, UpdateTimeEntryInput{}); err == nil {
		t.Fatal("want error for empty update")
	}

	// Missing entry.
	now := time.Now()
	if err := s.UpdateTimeEntry(ctx, 999, UpdateTimeEntryInput{End: &now}); err == nil {
		t.Fatal("want error for missing entry")
	}
}

func TestStopTimerWithAtBackdates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tk := mustAdd(t, s, AddInput{Title: "T"})
	e, _ := s.StartTimer(ctx, tk.ID, "")

	// Start is "now", set end 30min ahead.
	at := time.Now().Add(30 * time.Minute)
	stopped, err := s.StopTimer(ctx, &at)
	if err != nil {
		t.Fatalf("StopTimer: %v", err)
	}
	if stopped.EndedAt == nil {
		t.Fatal("EndedAt is nil")
	}
	d := stopped.Duration(time.Now())
	if d < 29*time.Minute || d > 31*time.Minute {
		t.Errorf("duration = %v, want ~30m", d)
	}
	_ = e
}

func TestStopTimerAtBeforeStartFails(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tk := mustAdd(t, s, AddInput{Title: "T"})
	s.StartTimer(ctx, tk.ID, "")

	past := time.Now().Add(-time.Hour)
	if _, err := s.StopTimer(ctx, &past); err == nil {
		t.Fatal("want error for at before start")
	}
}
