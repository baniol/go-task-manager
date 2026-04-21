package store

import (
	"context"
	"errors"
	"time"

	"go-task-manager/internal/task"
)

type Store interface {
	Add(ctx context.Context, in AddInput) (task.Task, error)
	List(ctx context.Context, filter ListFilter) ([]task.Task, error)
	Search(ctx context.Context, query string, filter SearchFilter) ([]task.Task, error)
	Get(ctx context.Context, id int64) (task.Task, error)
	Move(ctx context.Context, id int64, status task.Status) error
	Update(ctx context.Context, id int64, in EditInput) error
	Delete(ctx context.Context, id int64) error
	Publish(ctx context.Context, id int64) error

	// per-task tag operations
	AddTaskTags(ctx context.Context, taskID int64, tags []string) error
	RemoveTaskTags(ctx context.Context, taskID int64, tags []string) error

	// system-wide delete (returns number of unlinked tasks)
	DeleteTag(ctx context.Context, name string) (int64, error)

	// reorder — sets task positions in the given order (1-based).
	SetPositions(ctx context.Context, ids []int64) error

	Tags(ctx context.Context) ([]string, error)

	// time tracking
	StartTimer(ctx context.Context, taskID int64, note string) (task.TimeEntry, error)
	StopTimer(ctx context.Context, at *time.Time) (task.TimeEntry, error)
	ActiveTimer(ctx context.Context) (*task.TimeEntry, error)
	TimeEntries(ctx context.Context, taskID int64) ([]task.TimeEntry, error)
	TaskTotalDuration(ctx context.Context, taskID int64) (time.Duration, error)
	GetTimeEntry(ctx context.Context, id int64) (task.TimeEntry, error)
	DeleteTimeEntry(ctx context.Context, id int64) error
	UpdateTimeEntry(ctx context.Context, id int64, in UpdateTimeEntryInput) error
	// TasksWithTimeEntries returns the set of task IDs that have at least one time entry.
	TasksWithTimeEntries(ctx context.Context) (map[int64]bool, error)

	// worklog — cross-task
	AllTimeEntries(ctx context.Context, filter TimeEntryFilter) ([]WorklogEntry, error)
	CreateTimeEntry(ctx context.Context, in CreateTimeEntryInput) (task.TimeEntry, error)
	SummarizeTimeEntries(ctx context.Context, filter TimeEntryFilter, group WorklogGroupBy) ([]WorklogSummaryRow, error)

	// ImportReplace wipes the whole database and inserts the dump, preserving task and entry IDs.
	ImportReplace(ctx context.Context, payload ImportPayload) error

	// Path returns the database file path (empty when :memory: or unknown).
	Path() string
	// Backup takes a consistent snapshot of the database to dst (VACUUM INTO).
	Backup(ctx context.Context, dst string) error

	Close() error
}

// ImportTask is a task plus its time entries, used during import.
type ImportTask struct {
	task.Task
	TimeEntries []task.TimeEntry
}

// ImportPayload is the full dump consumed by ImportReplace.
type ImportPayload struct {
	Tasks []ImportTask
	Tags  []string // extra tags not used by any task (optional)
}

// ErrTimerAlreadyActive is returned by StartTimer when another entry is active.
var ErrTimerAlreadyActive = errors.New("another timer is already active")

// ErrNoActiveTimer is returned by StopTimer when no entry is active.
var ErrNoActiveTimer = errors.New("no active timer")

// UpdateTimeEntryInput — nil fields mean "do not change".
type UpdateTimeEntryInput struct {
	Start *time.Time
	End   *time.Time
	Note  *string
}

// TimeEntryFilter — empty field means no filter. Half-open range [From, To).
type TimeEntryFilter struct {
	From   *time.Time
	To     *time.Time
	TaskID *int64
	Tags   []string // AND — task must have all of them
	Search string   // LIKE %q% over note (case-insensitive)
	Limit  int      // 0 = no limit
}

// CreateTimeEntryInput — manual backfill. End is required (complete entry, not a timer).
type CreateTimeEntryInput struct {
	TaskID int64
	Start  time.Time
	End    time.Time
	Note   string
}

// WorklogEntry — time entry enriched with task data (cross-task listing).
type WorklogEntry struct {
	task.TimeEntry
	TaskTitle string
	TaskTags  []string
}

type WorklogGroupBy string

const (
	GroupByDay  WorklogGroupBy = "day"
	GroupByTask WorklogGroupBy = "task"
	GroupByTag  WorklogGroupBy = "tag"
)

// WorklogSummaryRow is one aggregated row.
type WorklogSummaryRow struct {
	Key      string        // e.g. "2026-04-17", "42", "backend"
	Label    string        // human-readable (task title for GroupByTask)
	Count    int           // number of entries
	Duration time.Duration // total
}

// AddInput is the input for Store.Add. Optional fields have zero-value defaults.
type AddInput struct {
	Title    string
	Body     string
	Priority task.Priority
	Tags     []string
	Ready    bool       // true → task created as non-draft
	Due      *time.Time // nil = no deadline
}

// EditInput is the input for Store.Update. Nil = don't change.
// Tag and epic operations live on dedicated methods.
type EditInput struct {
	Title    *string
	Body     *string
	Priority *task.Priority
	Draft    *bool
	Due      *time.Time // set due_at; nil + ClearDue=false → leave untouched
	ClearDue bool       // true → clear due_at (mutually exclusive with Due)
}

// SortOrder controls List ordering.
type SortOrder string

const (
	SortDefault  SortOrder = ""         // priority desc, id asc
	SortDue      SortOrder = "due"      // due_at asc (NULLS LAST), then priority, id
	SortPosition SortOrder = "position" // manual order (position > 0 first, then priority/id)
)

// DraftMode controls whether List returns drafts.
type DraftMode string

const (
	DraftAll  DraftMode = ""     // default: both drafts and ready
	DraftOnly DraftMode = "only" // only drafts
	DraftHide DraftMode = "hide" // only ready
)

// SearchFilter constrains Search results. Empty field = no filter.
type SearchFilter struct {
	Status   task.Status
	Statuses []task.Status // alternative to Status: IN (?,…) when len > 0
	Tags     []string      // AND: task must have all given tags
}

// ListFilter constrains List results. Empty field = no filter.
type ListFilter struct {
	Status    task.Status
	Statuses  []task.Status // alternative to Status: IN (?,…) when len > 0
	Priority  task.Priority
	Tags      []string // AND: task must have all given tags
	DraftMode DraftMode
	Overdue   bool      // due_at < now AND status != done (ignores tasks without due)
	NoDue     bool      // only tasks without due_at
	Sort      SortOrder // result ordering
}
