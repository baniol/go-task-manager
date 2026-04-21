package cli

import (
	"strings"
	"testing"
	"time"
)

func TestTimerStartStopStatusLog(t *testing.T) {
	h := newHarness(t)

	if err := h.run("add", "kod", "--ready"); err != nil {
		t.Fatalf("add: %v (%s)", err, h.stderr.String())
	}

	if err := h.run("status"); err != nil {
		t.Fatalf("status: %v", err)
	}
	if got := h.stdout.String(); !strings.Contains(got, "no active timer") {
		t.Fatalf("status before start = %q", got)
	}

	if err := h.run("start", "1"); err != nil {
		t.Fatalf("start: %v (%s)", err, h.stderr.String())
	}
	if got := h.stdout.String(); !strings.Contains(got, "started timer on #1") {
		t.Fatalf("start output = %q", got)
	}

	if err := h.run("status"); err != nil {
		t.Fatalf("status: %v", err)
	}
	if got := h.stdout.String(); !strings.Contains(got, "#1") || !strings.Contains(got, "kod") {
		t.Fatalf("status active = %q", got)
	}

	if err := h.run("stop"); err != nil {
		t.Fatalf("stop: %v (%s)", err, h.stderr.String())
	}
	if got := h.stdout.String(); !strings.Contains(got, "stopped timer on #1") {
		t.Fatalf("stop output = %q", got)
	}

	if err := h.run("log", "1"); err != nil {
		t.Fatalf("log: %v", err)
	}
	got := h.stdout.String()
	if !strings.Contains(got, "total:") {
		t.Fatalf("log output missing total: %q", got)
	}
}

func TestTimerStartRejectsWhenActive(t *testing.T) {
	h := newHarness(t)
	mustRun(t, h, "add", "A", "--ready")
	mustRun(t, h, "add", "B", "--ready")
	mustRun(t, h, "start", "1")

	if err := h.run("start", "2"); err == nil {
		t.Fatalf("want error starting second timer, got stdout=%q", h.stdout.String())
	}
}

func TestLogEmpty(t *testing.T) {
	h := newHarness(t)
	mustRun(t, h, "add", "x", "--ready")
	if err := h.run("log", "1"); err != nil {
		t.Fatalf("log: %v", err)
	}
	if got := h.stdout.String(); !strings.Contains(got, "no time entries") {
		t.Fatalf("log empty = %q", got)
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0s"},
		{45 * time.Second, "45s"},
		{90 * time.Second, "1m30s"},
		{3*time.Hour + 5*time.Minute + 12*time.Second, "3h5m"},
		{-time.Second, "0s"},
	}
	for _, c := range cases {
		if got := formatDuration(c.in); got != c.want {
			t.Errorf("formatDuration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func mustRun(t *testing.T, h *testHarness, args ...string) {
	t.Helper()
	if err := h.run(args...); err != nil {
		t.Fatalf("run %v: %v (%s)", args, err, h.stderr.String())
	}
}
