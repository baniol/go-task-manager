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

// buildTimeEntryWhere builds WHERE + args from TimeEntryFilter.
// Aliases: `te` = time_entries, `t` = tasks.
func buildTimeEntryWhere(filter TimeEntryFilter) (string, []any) {
	var (
		where []string
		args  []any
	)
	if filter.From != nil {
		where = append(where, "te.started_at >= ?")
		args = append(args, filter.From.UTC().Format(time.RFC3339))
	}
	if filter.To != nil {
		where = append(where, "te.started_at < ?")
		args = append(args, filter.To.UTC().Format(time.RFC3339))
	}
	if filter.TaskID != nil {
		where = append(where, "te.task_id = ?")
		args = append(args, *filter.TaskID)
	}
	if s := strings.TrimSpace(filter.Search); s != "" {
		where = append(where, "te.note LIKE ? COLLATE NOCASE")
		args = append(args, "%"+s+"%")
	}
	tags := dedupeStrings(filter.Tags)
	if len(tags) > 0 {
		placeholders := strings.Repeat("?,", len(tags))
		placeholders = placeholders[:len(placeholders)-1]
		where = append(where,
			`te.task_id IN (
				SELECT tt.task_id FROM task_tags tt
				JOIN tags tg ON tg.id = tt.tag_id
				WHERE tg.name IN (`+placeholders+`)
				GROUP BY tt.task_id
				HAVING COUNT(DISTINCT tg.id) = ?
			)`)
		for _, name := range tags {
			args = append(args, name)
		}
		args = append(args, len(tags))
	}
	cond := ""
	if len(where) > 0 {
		cond = "WHERE " + strings.Join(where, " AND ")
	}
	return cond, args
}

func (s *SQLite) AllTimeEntries(ctx context.Context, filter TimeEntryFilter) ([]WorklogEntry, error) {
	cond, args := buildTimeEntryWhere(filter)

	query := `
		SELECT te.id, te.task_id, te.started_at, te.ended_at, te.note, t.title
		FROM time_entries te
		JOIN tasks t ON t.id = te.task_id
		` + cond + `
		ORDER BY te.started_at DESC, te.id DESC`
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var (
		out []WorklogEntry
		ids []int64
	)
	for rows.Next() {
		var (
			e         WorklogEntry
			startedAt string
			endedAt   sql.NullString
		)
		if err := rows.Scan(&e.ID, &e.TaskID, &startedAt, &endedAt, &e.Note, &e.TaskTitle); err != nil {
			return nil, err
		}
		started, err := time.Parse(time.RFC3339, startedAt)
		if err != nil {
			return nil, fmt.Errorf("parse started_at for entry %d: %w", e.ID, err)
		}
		e.StartedAt = started
		if endedAt.Valid {
			ended, err := time.Parse(time.RFC3339, endedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse ended_at for entry %d: %w", e.ID, err)
			}
			e.EndedAt = &ended
		}
		out = append(out, e)
		ids = append(ids, e.TaskID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(ids) == 0 {
		return out, nil
	}
	tagsByTask, err := loadTagsForTasks(ctx, s.db, dedupeInt64(ids))
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].TaskTags = tagsByTask[out[i].TaskID]
	}
	return out, nil
}

func (s *SQLite) CreateTimeEntry(ctx context.Context, in CreateTimeEntryInput) (task.TimeEntry, error) {
	if in.End.Before(in.Start) {
		return task.TimeEntry{}, fmt.Errorf("end %s before start %s",
			in.End.Format(time.RFC3339), in.Start.Format(time.RFC3339))
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return task.TimeEntry{}, err
	}
	defer tx.Rollback()

	if err := assertTaskExists(ctx, tx, in.TaskID); err != nil {
		return task.TimeEntry{}, err
	}

	start := in.Start.UTC()
	end := in.End.UTC()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO time_entries (task_id, started_at, ended_at, note) VALUES (?, ?, ?, ?)`,
		in.TaskID, start.Format(time.RFC3339), end.Format(time.RFC3339), in.Note)
	if err != nil {
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
		TaskID:    in.TaskID,
		StartedAt: start,
		EndedAt:   &end,
		Note:      in.Note,
	}, nil
}

func (s *SQLite) SummarizeTimeEntries(ctx context.Context, filter TimeEntryFilter, group WorklogGroupBy) ([]WorklogSummaryRow, error) {
	cond, args := buildTimeEntryWhere(filter)

	// Duration in seconds: COALESCE ended_at → now (for running timers).
	// strftime('%s', ...) returns epoch as INTEGER.
	durExpr := `CAST(strftime('%s', COALESCE(te.ended_at, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))) AS INTEGER)
	            - CAST(strftime('%s', te.started_at) AS INTEGER)`

	var (
		query   string
		scanRow func(*sql.Rows) (WorklogSummaryRow, error)
	)
	switch group {
	case GroupByDay:
		// Group by LOCAL date — assumption: started_at in UTC,
		// convert to local time via SQLite's 'localtime' modifier.
		query = `
			SELECT date(te.started_at, 'localtime') AS k,
			       COUNT(*) AS cnt,
			       SUM(` + durExpr + `) AS secs
			FROM time_entries te
			` + cond + `
			GROUP BY k
			ORDER BY k`
		scanRow = func(r *sql.Rows) (WorklogSummaryRow, error) {
			var (
				key  string
				cnt  int
				secs int64
			)
			if err := r.Scan(&key, &cnt, &secs); err != nil {
				return WorklogSummaryRow{}, err
			}
			return WorklogSummaryRow{
				Key:      key,
				Label:    key,
				Count:    cnt,
				Duration: time.Duration(secs) * time.Second,
			}, nil
		}
	case GroupByTask:
		query = `
			SELECT te.task_id, t.title, COUNT(*) AS cnt, SUM(` + durExpr + `) AS secs
			FROM time_entries te
			JOIN tasks t ON t.id = te.task_id
			` + cond + `
			GROUP BY te.task_id, t.title
			ORDER BY secs DESC`
		scanRow = func(r *sql.Rows) (WorklogSummaryRow, error) {
			var (
				taskID int64
				title  string
				cnt    int
				secs   int64
			)
			if err := r.Scan(&taskID, &title, &cnt, &secs); err != nil {
				return WorklogSummaryRow{}, err
			}
			return WorklogSummaryRow{
				Key:      fmt.Sprintf("%d", taskID),
				Label:    title,
				Count:    cnt,
				Duration: time.Duration(secs) * time.Second,
			}, nil
		}
	case GroupByTag:
		// LEFT JOIN task_tags — entries without tags land in the `(none)` bucket.
		// COUNT(DISTINCT te.id) because LEFT JOIN multiplies rows per tag.
		query = `
			SELECT COALESCE(tg.name, '(none)') AS k,
			       COUNT(DISTINCT te.id) AS cnt,
			       SUM(` + durExpr + `) AS secs
			FROM time_entries te
			LEFT JOIN task_tags tt ON tt.task_id = te.task_id
			LEFT JOIN tags tg ON tg.id = tt.tag_id
			` + cond + `
			GROUP BY k
			ORDER BY secs DESC`
		scanRow = func(r *sql.Rows) (WorklogSummaryRow, error) {
			var (
				key  string
				cnt  int
				secs int64
			)
			if err := r.Scan(&key, &cnt, &secs); err != nil {
				return WorklogSummaryRow{}, err
			}
			return WorklogSummaryRow{
				Key:      key,
				Label:    key,
				Count:    cnt,
				Duration: time.Duration(secs) * time.Second,
			}, nil
		}
	default:
		return nil, errors.New("invalid group-by")
	}

	// GroupByTag + tag filter: the LEFT JOIN inflates SUM because a single
	// entry would be counted once per assigned tag. For simplicity we
	// return an error — tag filter and group-by-tag together are nonsensical.
	if group == GroupByTag && len(dedupeStrings(filter.Tags)) > 0 {
		return nil, errors.New("cannot combine --tag filter with group-by=tag")
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WorklogSummaryRow
	for rows.Next() {
		row, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func dedupeInt64(in []int64) []int64 {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(in))
	out := make([]int64, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
