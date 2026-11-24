package policy

import "testing"

func TestNewTimeOfDay(t *testing.T) {
	tests := []struct {
		name    string
		hour    int
		minute  int
		wantErr bool
	}{
		{"midnight", 0, 0, false},
		{"afternoon", 14, 30, false},
		{"last minute of the day", 23, 59, false},
		{"end of day", 24, 0, false},
		{"negative hour", -1, 0, true},
		{"hour above 24", 25, 0, true},
		{"negative minute", 14, -1, true},
		{"minute above 59", 14, 60, true},
		{"after end of day", 24, 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTimeOfDay(tt.hour, tt.minute)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTimeOfDay(%d, %d) error = %v, want error: %t",
					tt.hour, tt.minute, err, tt.wantErr)
			}
		})
	}
}
