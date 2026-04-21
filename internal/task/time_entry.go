package task

import "time"

// TimeEntry represents a single period of work on a task.
// EndedAt == nil → timer still active.
type TimeEntry struct {
	ID        int64
	TaskID    int64
	StartedAt time.Time
	EndedAt   *time.Time
	Note      string
}

// Duration returns the entry length. For an active entry uses now.
func (e TimeEntry) Duration(now time.Time) time.Duration {
	end := now
	if e.EndedAt != nil {
		end = *e.EndedAt
	}
	return end.Sub(e.StartedAt)
}

// Active reports whether this is an active (not yet ended) entry.
func (e TimeEntry) Active() bool { return e.EndedAt == nil }
