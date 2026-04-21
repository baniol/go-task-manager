package cli

import (
	"strings"
	"testing"
	"time"
)

func TestStopAtBackdates(t *testing.T) {
	h := newHarness(t)
	mustRun(t, h, "add", "T", "--ready")
	mustRun(t, h, "start", "1")

	// Stop 5 minutes into the future (simulates real elapsed time).
	future := time.Now().Add(5 * time.Minute).Format("2006-01-02 15:04")
	if err := h.run("stop", "--at", future); err != nil {
		t.Fatalf("stop --at: %v (%s)", err, h.stderr.String())
	}

	// Log should show a duration of ~5min (tolerance for minute rounding).
	if err := h.run("log", "1"); err != nil {
		t.Fatalf("log: %v", err)
	}
	got := h.stdout.String()
	if !strings.Contains(got, "total:") {
		t.Fatalf("missing total: %q", got)
	}
	// Accept 4m.. or 5m.. — depends on when rounding happens.
	if !strings.Contains(got, "4m") && !strings.Contains(got, "5m") {
		t.Fatalf("expected ~5m total, got %q", got)
	}
}

func TestStopAtRelative(t *testing.T) {
	h := newHarness(t)
	mustRun(t, h, "add", "T", "--ready")
	mustRun(t, h, "start", "1")

	// --at -0s = now, should work (end >= start).
	if err := h.run("stop", "--at", "-0s"); err != nil {
		t.Fatalf("stop --at -0s: %v (%s)", err, h.stderr.String())
	}
}

func TestLogRm(t *testing.T) {
	h := newHarness(t)
	mustRun(t, h, "add", "T", "--ready")
	mustRun(t, h, "start", "1")
	mustRun(t, h, "stop")

	// Find the entry ID in the log.
	mustRun(t, h, "log", "1")
	if !strings.Contains(h.stdout.String(), "#1") {
		t.Fatalf("log missing entry id: %q", h.stdout.String())
	}

	if err := h.run("log", "rm", "1"); err != nil {
		t.Fatalf("log rm: %v (%s)", err, h.stderr.String())
	}
	if !strings.Contains(h.stdout.String(), "deleted entry 1") {
		t.Fatalf("unexpected output: %q", h.stdout.String())
	}

	mustRun(t, h, "log", "1")
	if !strings.Contains(h.stdout.String(), "no time entries") {
		t.Fatalf("entry still present: %q", h.stdout.String())
	}
}

func TestLogEditEnd(t *testing.T) {
	h := newHarness(t)
	mustRun(t, h, "add", "T", "--ready")
	mustRun(t, h, "start", "1")
	mustRun(t, h, "stop")

	// Move end one hour into the future from now; use the full format to avoid
	// ambiguity when parsing HH:MM near midnight.
	future := time.Now().Add(time.Hour).Format("2006-01-02 15:04")
	if err := h.run("log", "edit", "1", "--end", future); err != nil {
		t.Fatalf("log edit: %v (%s)", err, h.stderr.String())
	}

	mustRun(t, h, "log", "1")
	got := h.stdout.String()
	// ~1h — may render as "1h0m" or "59m..." depending on HH:MM parser rounding.
	if !strings.Contains(got, "1h") && !strings.Contains(got, "59m") {
		t.Fatalf("expected ~1h total, got %q", got)
	}
}

func TestLogEditValidation(t *testing.T) {
	h := newHarness(t)
	mustRun(t, h, "add", "T", "--ready")
	mustRun(t, h, "start", "1")
	mustRun(t, h, "stop")

	// End before start → error.
	past := time.Now().Add(-24 * time.Hour).Format("2006-01-02 15:04")
	if err := h.run("log", "edit", "1", "--end", past); err == nil {
		t.Fatalf("want error for end before start")
	}
}

func TestParseTimeFlag(t *testing.T) {
	now := time.Date(2026, 4, 16, 14, 30, 0, 0, time.Local)
	cases := []struct {
		in      string
		want    time.Time
		wantErr bool
	}{
		{"15:30", time.Date(2026, 4, 16, 15, 30, 0, 0, time.Local), false},
		{"2026-04-15 09:00", time.Date(2026, 4, 15, 9, 0, 0, 0, time.Local), false},
		{"-20m", now.Add(-20 * time.Minute), false},
		{"-1h30m", now.Add(-90 * time.Minute), false},
		{"garbage", time.Time{}, true},
		{"", time.Time{}, true},
	}
	for _, c := range cases {
		got, err := parseTimeFlag(c.in, now)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseTimeFlag(%q): want error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseTimeFlag(%q): %v", c.in, err)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("parseTimeFlag(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
