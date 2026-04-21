package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"go-task-manager/internal/task"
)

func (s *SQLite) StartTimer(ctx context.Context, taskID int64, note string) (task.TimeEntry, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return task.TimeEntry{}, err
	}
	defer tx.Rollback()

	if err := assertTaskExists(ctx, tx, taskID); err != nil {
		return task.TimeEntry{}, err
	}

	now := time.Now().UTC()
	startedAt := now.Format(time.RFC3339)
	res, err := tx.ExecContext(ctx,
		`INSERT INTO time_entries (task_id, started_at, note) VALUES (?, ?, ?)`,
		taskID, startedAt, note)
	if err != nil {
		if isUniqueViolation(err) {
			return task.TimeEntry{}, ErrTimerAlreadyActive
		}
		return task.TimeEntry{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return task.TimeEntry{}, err
	}
	if err := tx.Commit(); err != nil {
		return task.TimeEntry{}, err
	}
	return task.TimeEntry{
		ID:        id,
		TaskID:    taskID,
		StartedAt: now,
		Note:      note,
	}, nil
}

// StopTimer stops the active timer. If at != nil, uses the given time instead of
// time.Now — useful when you forget to hit stop and want to correct it.
// Validates that at >= started_at.
func (s *SQLite) StopTimer(ctx context.Context, at *time.Time) (task.TimeEntry, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return task.TimeEntry{}, err
	}
	defer tx.Rollback()

	entry, err := activeTimerTx(ctx, tx)
	if err != nil {
		return task.TimeEntry{}, err
	}
	if entry == nil {
		return task.TimeEntry{}, ErrNoActiveTimer
	}

	end := time.Now().UTC()
	if at != nil {
		end = at.UTC()
	}
	if end.Before(entry.StartedAt) {
		return task.TimeEntry{}, fmt.Errorf("end %s before start %s",
			end.Format(time.RFC3339), entry.StartedAt.Format(time.RFC3339))
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE time_entries SET ended_at = ? WHERE id = ?`,
		end.Format(time.RFC3339), entry.ID); err != nil {
		return task.TimeEntry{}, err
	}
	if err := tx.Commit(); err != nil {
		return task.TimeEntry{}, err
	}
	entry.EndedAt = &end
	return *entry, nil
}

func (s *SQLite) TasksWithTimeEntries(ctx context.Context) (map[int64]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT task_id FROM time_entries`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (s *SQLite) GetTimeEntry(ctx context.Context, id int64) (task.TimeEntry, error) {
	var (
		e         task.TimeEntry
		startedAt string
		endedAt   sql.NullString
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, task_id, started_at, ended_at, note FROM time_entries WHERE id = ?`, id,
	).Scan(&e.ID, &e.TaskID, &startedAt, &endedAt, &e.Note)
	if err == sql.ErrNoRows {
		return task.TimeEntry{}, fmt.Errorf("time entry %d not found", id)
	}
	if err != nil {
		return task.TimeEntry{}, err
	}
	started, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return task.TimeEntry{}, fmt.Errorf("parse started_at: %w", err)
	}
	e.StartedAt = started
	if endedAt.Valid {
		ended, err := time.Parse(time.RFC3339, endedAt.String)
		if err != nil {
			return task.TimeEntry{}, fmt.Errorf("parse ended_at: %w", err)
		}
		e.EndedAt = &ended
	}
	return e, nil
}

func (s *SQLite) DeleteTimeEntry(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM time_entries WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("time entry %d not found", id)
	}
	return nil
}

func (s *SQLite) UpdateTimeEntry(ctx context.Context, id int64, in UpdateTimeEntryInput) error {
	if in.Start == nil && in.End == nil && in.Note == nil {
		return errors.New("nothing to update")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Fetch current values so we can validate start<=end consistency.
	var (
		startedAt string
		endedAt   sql.NullString
	)
	if err := tx.QueryRowContext(ctx,
		`SELECT started_at, ended_at FROM time_entries WHERE id = ?`, id).
		Scan(&startedAt, &endedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("time entry %d not found", id)
		}
		return err
	}
	curStart, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return err
	}
	var curEnd *time.Time
	if endedAt.Valid {
		t, err := time.Parse(time.RFC3339, endedAt.String)
		if err != nil {
			return err
		}
		curEnd = &t
	}

	newStart := curStart
	if in.Start != nil {
		newStart = in.Start.UTC()
	}
	newEnd := curEnd
	if in.End != nil {
		v := in.End.UTC()
		newEnd = &v
	}
	if newEnd != nil && newEnd.Before(newStart) {
		return fmt.Errorf("end %s before start %s",
			newEnd.Format(time.RFC3339), newStart.Format(time.RFC3339))
	}

	var sets []string
	var args []any
	if in.Start != nil {
		sets = append(sets, "started_at = ?")
		args = append(args, newStart.Format(time.RFC3339))
	}
	if in.End != nil {
		sets = append(sets, "ended_at = ?")
		args = append(args, newEnd.Format(time.RFC3339))
	}
	if in.Note != nil {
		sets = append(sets, "note = ?")
		args = append(args, *in.Note)
	}
	args = append(args, id)
	if _, err := tx.ExecContext(ctx,
		`UPDATE time_entries SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLite) ActiveTimer(ctx context.Context) (*task.TimeEntry, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	return activeTimerTx(ctx, tx)
}

func activeTimerTx(ctx context.Context, tx *sql.Tx) (*task.TimeEntry, error) {
	row := tx.QueryRowContext(ctx,
		`SELECT id, task_id, started_at, ended_at, note
		 FROM time_entries WHERE ended_at IS NULL LIMIT 1`)
	entry, err := scanTimeEntry(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

func (s *SQLite) TimeEntries(ctx context.Context, taskID int64) ([]task.TimeEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, task_id, started_at, ended_at, note
		 FROM time_entries WHERE task_id = ? ORDER BY started_at`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []task.TimeEntry
	for rows.Next() {
		entry, err := scanTimeEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (s *SQLite) TaskTotalDuration(ctx context.Context, taskID int64) (time.Duration, error) {
	entries, err := s.TimeEntries(ctx, taskID)
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	var total time.Duration
	for _, e := range entries {
		total += e.Duration(now)
	}
	return total, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTimeEntry(r rowScanner) (task.TimeEntry, error) {
	var (
		e         task.TimeEntry
		startedAt string
		endedAt   sql.NullString
	)
	if err := r.Scan(&e.ID, &e.TaskID, &startedAt, &endedAt, &e.Note); err != nil {
		return task.TimeEntry{}, err
	}
	started, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return task.TimeEntry{}, fmt.Errorf("parse started_at for entry %d: %w", e.ID, err)
	}
	e.StartedAt = started
	if endedAt.Valid {
		ended, err := time.Parse(time.RFC3339, endedAt.String)
		if err != nil {
			return task.TimeEntry{}, fmt.Errorf("parse ended_at for entry %d: %w", e.ID, err)
		}
		e.EndedAt = &ended
	}
	return e, nil
}

// isUniqueViolation detects a UNIQUE violation from modernc/sqlite (code 2067 = SQLITE_CONSTRAINT_UNIQUE).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "constraint failed: UNIQUE")
}
