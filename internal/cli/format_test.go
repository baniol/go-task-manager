package cli

import (
	"testing"

	"go-task-manager/internal/task"
)

func TestFormatTaskShort(t *testing.T) {
	tests := []struct {
		name string
		in   task.Task
		want string
	}{
		{
			name: "ready task minimal",
			in:   task.Task{Title: "write docs", Priority: task.PriorityNormal},
			want: "[normal] write docs",
		},
		{
			name: "draft flag prefixes prio",
			in:   task.Task{Title: "wip", Priority: task.PriorityLow, Draft: true},
			want: "[draft|low] wip",
		},
		{
			name: "tags rendered with plus, order preserved",
			in: task.Task{
				Title: "refactor", Priority: task.PriorityNormal,
				Tags: []string{"backend", "api"},
			},
			want: "[normal] refactor +backend +api",
		},
		{
			name: "tags with due",
			in: task.Task{
				Title: "migrate", Priority: task.PriorityHigh, Draft: true,
				Tags: []string{"db"},
			},
			want: "[draft|high] migrate +db",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatTaskShort(tt.in); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
