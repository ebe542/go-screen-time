package policy

import (
	"testing"
	"time"
)

func TestUsageScheduleSelectsDayPlan(t *testing.T) {
	weekdayWindow, err := NewUsageWindow(mustTimeOfDay(t, 14, 0), mustTimeOfDay(t, 19, 0))
	if err != nil {
		t.Fatalf("create weekday window: %v", err)
	}

	weekendWindow, err := NewUsageWindow(mustTimeOfDay(t, 10, 0), mustTimeOfDay(t, 19, 0))
	if err != nil {
		t.Fatalf("create weekend window: %v", err)
	}

	schedule := UsageSchedule{
		WeekdayWindows: []UsageWindow{weekdayWindow},
		WeekendWindows: []UsageWindow{weekendWindow},
	}

	local := time.FixedZone("UTC+1", 60*60)
	tests := []struct {
		name string
		at   time.Time
		want bool
	}{
		{"Friday at noon", time.Date(2026, time.November, 27, 12, 0, 0, 0, local), false},
		{"Saturday at noon", time.Date(2026, time.November, 28, 12, 0, 0, 0, local), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := schedule.Allows(tt.at); got != tt.want {
				t.Errorf("Allows(%s) = %t, want %t", tt.at.Format(time.RFC3339), got, tt.want)
			}
		})
	}
}

func TestUsageScheduleAcrossMidnight(t *testing.T) {
	newWindow := func(start, end TimeOfDay) UsageWindow {
		t.Helper()
		window, err := NewUsageWindow(start, end)
		if err != nil {
			t.Fatalf("NewUsageWindow(%d, %d): %v", start, end, err)
		}
		return window
	}

	schedule := UsageSchedule{
		WeekdayWindows: []UsageWindow{
			newWindow(mustTimeOfDay(t, 14, 0), mustTimeOfDay(t, 17, 0)),
			newWindow(mustTimeOfDay(t, 22, 0), mustTimeOfDay(t, 24, 0)),
		},
		WeekendWindows: []UsageWindow{
			newWindow(mustTimeOfDay(t, 0, 0), mustTimeOfDay(t, 1, 0)),
		},
	}

	tests := []struct {
		name   string
		day    int
		hour   int
		minute int
		want   bool
	}{
		{"Friday in first window", 27, 15, 0, true},
		{"Friday between windows", 27, 18, 0, false},
		{"Friday in second window", 27, 22, 0, true},
		{"Friday just before midnight", 27, 23, 59, true},
		{"Saturday at midnight", 28, 0, 0, true},
		{"Saturday when the window closes", 28, 1, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := time.Date(2026, time.November, tt.day, tt.hour, tt.minute, 0, 0, time.UTC)
			if got := schedule.Allows(at); got != tt.want {
				t.Errorf("Allows(%s) = %t, want %t", at.Format(time.RFC3339), got, tt.want)
			}
		})
	}
}

func TestUsageScheduleRejectsUseOnUnscheduledDay(t *testing.T) {
	window, err := NewUsageWindow(
		mustTimeOfDay(t, 14, 0),
		mustTimeOfDay(t, 19, 0))
	if err != nil {
		t.Fatalf("create weekday window: %v", err)
	}

	schedule := UsageSchedule{WeekdayWindows: []UsageWindow{window}}
	saturday := time.Date(2026, time.November, 28, 15, 0, 0, 0, time.UTC)

	if schedule.Allows(saturday) {
		t.Error("Allows returned true for Saturday with an empty weekend plan")
	}
}
