package policy

import (
	"testing"
	"time"
)

func mustTimeOfDay(t *testing.T, hour, minute int) TimeOfDay {
	t.Helper()

	at, err := NewTimeOfDay(hour, minute)
	if err != nil {
		t.Fatalf("NewTimeOfDay(%d, %d): %v", hour, minute, err)
	}
	return at
}

func TestUsageWindowAllows(t *testing.T) {
	window, err := NewUsageWindow(
		mustTimeOfDay(t, 14, 0),
		mustTimeOfDay(t, 19, 0),
	)
	if err != nil {
		t.Fatalf("NewUsageWindow: %v", err)
	}

	tests := []struct {
		name   string
		hour   int
		minute int
		want   bool
	}{
		{"before opening", 13, 59, false},
		{"at opening", 14, 0, true},
		{"before closing", 18, 59, true},
		{"at closing", 19, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := time.Date(2026, time.November, 24, tt.hour, tt.minute, 0, 0, time.UTC)
			if got := window.Allows(at); got != tt.want {
				t.Errorf("Allows(%s) = %t, want %t", at.Format("15:04"), got, tt.want)
			}
		})
	}
}

func TestNewUsageWindowRejectsInvalidBoundaries(t *testing.T) {
	opening := mustTimeOfDay(t, 14, 0)
	closing := mustTimeOfDay(t, 19, 0)

	tests := []struct {
		name  string
		start TimeOfDay
		end   TimeOfDay
	}{
		{"equal boundaries", opening, opening},
		{"end before start", closing, opening},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewUsageWindow(tt.start, tt.end); err == nil {
				t.Error("NewUsageWindow returned no error for invalid boundaries")
			}
		})
	}
}
