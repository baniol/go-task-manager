package cli

import (
	"regexp"
	"strings"
	"testing"

	"go-task-manager/internal/store"
	"go-task-manager/internal/task"
)

func TestListFlagValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "draft and ready are mutually exclusive",
			args:    []string{"list", "--draft", "--ready"},
			wantErr: "--draft and --ready are mutually exclusive",
		},
		{
			name:    "invalid status",
			args:    []string{"list", "--status", "archived"},
			wantErr: `invalid status "archived"`,
		},
		{
			name:    "invalid priority",
			args:    []string{"list", "--prio", "urgent"},
			wantErr: `invalid priority "urgent"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
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

func TestListEmptyHintWhenNoTasks(t *testing.T) {
	h := newHarness(t)
	if err := h.run("list"); err != nil {
		t.Fatalf("run: %v (stderr=%q)", err, h.stderr.String())
	}
	if !strings.Contains(h.stdout.String(), "no tasks match") {
		t.Errorf("stdout = %q, want hint about no tasks", h.stdout.String())
	}
}

func TestListOutputFormat(t *testing.T) {
	h := newHarness(t)

	// Seed three tasks covering: draft (~), ready, tagged, empty tags.
	if _, err := h.store.Add(h.ctx, store.AddInput{
		Title: "ship release", Priority: task.PriorityHigh,
		Tags: []string{"backend"}, Ready: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Add(h.ctx, store.AddInput{
		Title: "draft idea", Priority: task.PriorityNormal,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Add(h.ctx, store.AddInput{
		Title: "low prio cleanup", Priority: task.PriorityLow,
		Tags: []string{"chore"}, Ready: true,
	}); err != nil {
		t.Fatal(err)
	}

	if err := h.run("list"); err != nil {
		t.Fatalf("run: %v (stderr=%q)", err, h.stderr.String())
	}

	// Normalize the timestamp; CreatedAt is set by the DB/clock.
	tsRE := regexp.MustCompile(`\d{4}-\d{2}-\d{2} \d{2}:\d{2}`)
	got := tsRE.ReplaceAllString(h.stdout.String(), "YYYY-MM-DD HH:MM")

	want := strings.Join([]string{
		"ID  STATUS  PRIO    TITLE             TAGS     DUE  CREATED",
		"1   todo    high    ship release      backend  -    YYYY-MM-DD HH:MM",
		"2   todo    normal  ~ draft idea      -        -    YYYY-MM-DD HH:MM",
		"3   todo    low     low prio cleanup  chore    -    YYYY-MM-DD HH:MM",
		"",
	}, "\n")

	if got != want {
		t.Errorf("output mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}
