package cli

import (
	"testing"
	"time"
)

func TestParseDueInput(t *testing.T) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, time.Local)

	tests := []struct {
		name    string
		in      string
		want    time.Time
		wantErr bool
	}{
		{name: "today", in: "today", want: today},
		{name: "tomorrow", in: "tomorrow", want: today.AddDate(0, 0, 1)},
		{name: "+0d equals today", in: "+0d", want: today},
		{name: "+3d", in: "+3d", want: today.AddDate(0, 0, 3)},
		{name: "explicit date", in: "2026-12-31",
			want: time.Date(2026, 12, 31, 23, 59, 59, 0, time.Local)},
		{name: "case insensitive", in: "TODAY", want: today},
		{name: "empty", in: "", wantErr: true},
		{name: "garbage", in: "next-week", wantErr: true},
		{name: "negative relative", in: "+-1d", wantErr: true},
		{name: "bad date format", in: "2026/12/31", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDueInput(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Errorf("want error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatDueShort(t *testing.T) {
	d := time.Date(2026, 4, 20, 23, 59, 59, 0, time.Local)
	if got := formatDueShort(nil, false); got != "-" {
		t.Errorf("nil: got %q, want -", got)
	}
	if got := formatDueShort(&d, false); got != "2026-04-20" {
		t.Errorf("not overdue: got %q", got)
	}
	if got := formatDueShort(&d, true); got != "!2026-04-20" {
		t.Errorf("overdue: got %q", got)
	}
}
