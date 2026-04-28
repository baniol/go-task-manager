package cli

import (
	"time"

	"go-task-manager/internal/task"
)

// Shared JSON dump format used by `tm export` and `tm import`.

type dumpTimeEntry struct {
	ID        int64      `json:"id"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	Note      string     `json:"note,omitempty"`
}

type dumpTask struct {
	ID          int64           `json:"id"`
	UUID        string          `json:"uuid,omitempty"`
	Title       string          `json:"title"`
	Body        string          `json:"body,omitempty"`
	Status      task.Status     `json:"status"`
	Priority    task.Priority   `json:"priority"`
	Tags        []string        `json:"tags"`
	Draft       bool            `json:"draft"`
	DueAt       *time.Time      `json:"due_at,omitempty"`
	Position    int             `json:"position"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	DeletedAt   *time.Time      `json:"deleted_at,omitempty"`
	TimeEntries []dumpTimeEntry `json:"time_entries"`
}

type dump struct {
	ExportedAt time.Time  `json:"exported_at"`
	Tasks      []dumpTask `json:"tasks"`
	Tags       []string   `json:"tags"`
}
