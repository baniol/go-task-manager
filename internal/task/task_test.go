package task

import "testing"

func TestParseStatus(t *testing.T) {
	tests := []struct {
		in      string
		want    Status
		wantErr bool
	}{
		{"todo", StatusTodo, false},
		{"doing", StatusDoing, false},
		{"action", StatusAction, false},
		{"done", StatusDone, false},
		{"", "", true},
		{"TODO", "", true},
		{"archived", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseStatus(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParsePriority(t *testing.T) {
	tests := []struct {
		in      string
		want    Priority
		wantErr bool
	}{
		{"low", PriorityLow, false},
		{"normal", PriorityNormal, false},
		{"high", PriorityHigh, false},
		{"", "", true},
		{"HIGH", "", true},
		{"urgent", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParsePriority(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
