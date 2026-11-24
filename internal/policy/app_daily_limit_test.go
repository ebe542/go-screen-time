package policy

import (
	"testing"
	"time"
)

func TestAppDailyLimitAllows(t *testing.T) {
	oneHour := &AppDailyLimit{Limit: time.Hour}
	zero := &AppDailyLimit{Limit: 0}

	tests := []struct {
		name  string
		limit *AppDailyLimit
		used  time.Duration
		want  bool
	}{
		{"no app limit", nil, 2 * time.Hour, true},
		{"below limit", oneHour, time.Hour - time.Second, true},
		{"at limit", oneHour, time.Hour, false},
		{"above limit", oneHour, time.Hour + time.Second, false},
		{"zero limit", zero, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.limit.Allows(tt.used); got != tt.want {
				t.Errorf("Allows(%s) = %t, want %t", tt.used, got, tt.want)
			}
		})
	}
}
