package task

import (
	"fmt"
	"time"
)

type Status string

const (
	StatusTodo   Status = "todo"
	StatusDoing  Status = "doing"
	StatusAction Status = "action"
	StatusDone   Status = "done"
)

func ParseStatus(s string) (Status, error) {
	switch Status(s) {
	case StatusTodo, StatusDoing, StatusAction, StatusDone:
		return Status(s), nil
	default:
		return "", fmt.Errorf("invalid status %q (want: todo|doing|action|done)", s)
	}
}

type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
)

func ParsePriority(s string) (Priority, error) {
	switch Priority(s) {
	case PriorityLow, PriorityNormal, PriorityHigh:
		return Priority(s), nil
	default:
		return "", fmt.Errorf("invalid priority %q (want: low|normal|high)", s)
	}
}

type Task struct {
	ID        int64
	UUID      string // stable cross-device identity for sync
	Title     string
	Body      string
	Status    Status
	Priority  Priority
	Tags      []string
	Draft     bool
	DueAt     *time.Time // nil = no deadline
	Position  int        // 0 = no manual position
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time // nil = alive; non-nil = tombstone
}
