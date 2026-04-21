package cli

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"go-task-manager/internal/store"
	"go-task-manager/internal/task"
)

func TestEditFlagValidation(t *testing.T) {
	h := newHarness(t)
	added, err := h.store.Add(h.ctx, store.AddInput{
		Title: "x", Priority: task.PriorityNormal,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := idArg(added.ID)

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "ready and draft are mutually exclusive",
			args:    []string{"edit", id, "--ready", "--draft"},
			wantErr: "--ready and --draft are mutually exclusive",
		},
		{
			name:    "empty title rejected",
			args:    []string{"edit", id, "--title", "   "},
			wantErr: "--title cannot be empty",
		},
		{
			name:    "no fields passed",
			args:    []string{"edit", id},
			wantErr: "nothing to update",
		},
		{
			name:    "invalid id",
			args:    []string{"edit", "abc", "--title", "x"},
			wantErr: `invalid id "abc"`,
		},
		{
			name:    "due and clear-due are mutually exclusive",
			args:    []string{"edit", id, "--due", "today", "--clear-due"},
			wantErr: "--due and --clear-due are mutually exclusive",
		},
		{
			name:    "invalid due value",
			args:    []string{"edit", id, "--due", "next-week"},
			wantErr: `invalid due "next-week"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.run(tt.args...)
			if err == nil {
				t.Fatalf("want error, got nil (stdout=%q)", h.stdout.String())
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestEditAppliesChanges(t *testing.T) {
	h := newHarness(t)
	added, err := h.store.Add(h.ctx, store.AddInput{
		Title: "old title", Priority: task.PriorityNormal,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := idArg(added.ID)

	if err := h.run("edit", id,
		"--title", "new title",
		"--body", "fresh body",
		"--ready",
	); err != nil {
		t.Fatalf("run: %v (stderr=%q)", err, h.stderr.String())
	}

	got, err := h.store.Get(h.ctx, added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "new title" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Body != "fresh body" {
		t.Errorf("Body = %q", got.Body)
	}
	if got.Draft {
		t.Errorf("Draft = true, want false after --ready")
	}

	if !strings.Contains(h.stdout.String(), "new title") {
		t.Errorf("stdout should confirm new title, got %q", h.stdout.String())
	}
}

func idArg(id int64) string {
	return strconv.FormatInt(id, 10)
}

func TestEditSetAndClearDue(t *testing.T) {
	h := newHarness(t)
	added, err := h.store.Add(h.ctx, store.AddInput{
		Title: "x", Priority: task.PriorityNormal,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := idArg(added.ID)

	if err := h.run("edit", id, "--due", "tomorrow"); err != nil {
		t.Fatalf("set due: %v (stderr=%q)", err, h.stderr.String())
	}
	got, _ := h.store.Get(h.ctx, added.ID)
	if got.DueAt == nil {
		t.Fatalf("DueAt nil after --due tomorrow")
	}
	tomorrow := time.Now().AddDate(0, 0, 1)
	if got.DueAt.Year() != tomorrow.Year() || got.DueAt.YearDay() != tomorrow.YearDay() {
		t.Errorf("DueAt = %v, want tomorrow's date", got.DueAt.Local())
	}

	if err := h.run("edit", id, "--clear-due"); err != nil {
		t.Fatalf("clear due: %v", err)
	}
	got, _ = h.store.Get(h.ctx, added.ID)
	if got.DueAt != nil {
		t.Errorf("DueAt = %v, want nil after --clear-due", got.DueAt)
	}
}
