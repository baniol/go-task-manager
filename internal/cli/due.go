package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// parseDueInput parses the --due flag and returns a deadline in local time.
// Supported formats (evaluated in local time):
//   - today, tomorrow            → end of this/next day
//   - +Nd                        → today + N days, end of day
//   - YYYY-MM-DD                 → given day, end of day
//
// "End of day" = 23:59:59 local, so a "due today" task doesn't become
// overdue at creation — overdue kicks in only after midnight.
func parseDueInput(s string) (time.Time, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return time.Time{}, fmt.Errorf("due value is empty")
	}
	now := time.Now()

	switch s {
	case "today":
		return endOfDay(now), nil
	case "tomorrow":
		return endOfDay(now.AddDate(0, 0, 1)), nil
	}

	if strings.HasPrefix(s, "+") && strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(s[1 : len(s)-1])
		if err != nil || n < 0 {
			return time.Time{}, fmt.Errorf("invalid relative due %q (want: +Nd with N >= 0)", s)
		}
		return endOfDay(now.AddDate(0, 0, n)), nil
	}

	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid due %q (want: today|tomorrow|+Nd|YYYY-MM-DD)", s)
	}
	return endOfDay(t), nil
}

// endOfDay returns 23:59:59 local time for the day of t.
func endOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 23, 59, 59, 0, time.Local)
}

// formatDueShort renders due_at as a local YYYY-MM-DD date.
// Prefixes `!` when the task is overdue and not done.
func formatDueShort(due *time.Time, overdue bool) string {
	if due == nil {
		return "-"
	}
	s := due.Local().Format("2006-01-02")
	if overdue {
		return "!" + s
	}
	return s
}
