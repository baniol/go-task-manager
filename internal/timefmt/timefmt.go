// Package timefmt provides shared helpers for parsing and formatting time.
package timefmt

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ParseFlag recognizes:
//   - "HH:MM"               → today at that time (local)
//   - "YYYY-MM-DD HH:MM"    → specific local datetime
//   - RFC3339               → exact
//   - "-20m" / "-1h30m"     → relative to now
func ParseFlag(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("empty time")
	}
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		d, err := time.ParseDuration(s)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		return now.Add(d), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return t, nil // date-only → midnight
	}
	if t, err := time.ParseInLocation("15:04", s, time.Local); err == nil {
		y, m, d := now.Date()
		return time.Date(y, m, d, t.Hour(), t.Minute(), 0, 0, time.Local), nil
	}
	return time.Time{}, fmt.Errorf("cannot parse time %q (want HH:MM, YYYY-MM-DD, YYYY-MM-DD HH:MM, RFC3339, or -1h30m)", s)
}

// Clock renders a duration as HH:MM:SS (or MM:SS when under 1h).
func Clock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	s := int((d % time.Minute) / time.Second)
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// Short renders a duration compactly, e.g. "2h15m", "45m30s", "30s".
func Short(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	s := int((d % time.Minute) / time.Second)
	switch {
	case h > 0:
		return fmt.Sprintf("%dh%dm", h, m)
	case m > 0:
		return fmt.Sprintf("%dm%ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}
