package policy

import (
	"testing"
	"time"
)

func TestAppAccessPolicyAllows(t *testing.T) {
	window, err := NewUsageWindow(
		mustTimeOfDay(t, 14, 0),
		mustTimeOfDay(t, 19, 0),
	)
	if err != nil {
		t.Fatalf("create usage window: %v", err)
	}

	policy := AppAccessPolicy{
		Schedule: UsageSchedule{
			WeekdayWindows: []UsageWindow{window},
		},
		SharedBudget: DailyBudget{WeekdayLimit: 2 * time.Hour},
	}

	friday := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	oneHour := &AppDailyLimit{Limit: time.Hour}

	tests := []struct {
		name       string
		at         time.Time
		sharedUsed time.Duration
		appUsed    time.Duration
		appLimit   *AppDailyLimit
		want       bool
	}{
		{"all rules allow use", friday, time.Hour, 30 * time.Minute, oneHour, true},
		{"window closed", friday.Add(4 * time.Hour), time.Hour, 30 * time.Minute, oneHour, false},
		{"shared budget exhausted", friday, 2 * time.Hour, 30 * time.Minute, oneHour, false},
		{"app limit exhausted", friday, time.Hour, time.Hour, oneHour, false},
		{"another app can use remaining shared time", friday, time.Hour, 0, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appPolicy := policy
			appPolicy.AppLimit = tt.appLimit

			if got := appPolicy.Allows(tt.at, tt.sharedUsed, tt.appUsed, time.Time{}); got != tt.want {
				t.Errorf("Allows() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestAppAccessPolicyBlocksDuringBreak(t *testing.T) {
	window, err := NewUsageWindow(
		mustTimeOfDay(t, 14, 0),
		mustTimeOfDay(t, 19, 0),
	)
	if err != nil {
		t.Fatalf("create usage window: %v", err)
	}

	policy := AppAccessPolicy{
		Schedule:     UsageSchedule{WeekdayWindows: []UsageWindow{window}},
		SharedBudget: DailyBudget{WeekdayLimit: 2 * time.Hour},
		BreakRule:    BreakRule{After: 45 * time.Minute, Duration: 10 * time.Minute},
	}
	startedAt := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)

	if policy.Allows(startedAt, 45*time.Minute, 20*time.Minute, startedAt) {
		t.Error("app is allowed during a required break")
	}

	endedAt := startedAt.Add(10 * time.Minute)
	if !policy.Allows(endedAt, 45*time.Minute, 20*time.Minute, startedAt) {
		t.Error("app is blocked after the required break ends")
	}
}

func TestAppAccessPolicyAllowsWithSharedBonus(t *testing.T) {
	window, err := NewUsageWindow(
		mustTimeOfDay(t, 14, 0),
		mustTimeOfDay(t, 19, 0),
	)
	if err != nil {
		t.Fatalf("create usage window: %v", err)
	}

	policy := AppAccessPolicy{
		Schedule:     UsageSchedule{WeekdayWindows: []UsageWindow{window}},
		SharedBudget: DailyBudget{WeekdayLimit: 2 * time.Hour},
		AppLimit:     &AppDailyLimit{Limit: time.Hour},
	}

	friday := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	bonus := DailyBonus{
		Shared:    15 * time.Minute,
		ExpiresAt: time.Date(2026, time.November, 28, 0, 0, 0, 0, time.UTC),
	}

	tests := []struct {
		name       string
		at         time.Time
		sharedUsed time.Duration
		appUsed    time.Duration
		want       bool
	}{
		{"bonus extends shared budget", friday, 2 * time.Hour, 30 * time.Minute, true},
		{"bonus exhausted", friday, 2*time.Hour + 15*time.Minute, 30 * time.Minute, false},
		{"window closed", friday.Add(4 * time.Hour), 2 * time.Hour, 30 * time.Minute, false},
		{"app limit reached", friday, 2 * time.Hour, time.Hour, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := policy.AllowsWithBonus(tt.at, tt.sharedUsed, tt.appUsed, time.Time{}, bonus)
			if got != tt.want {
				t.Errorf("AllowsWithBonus() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestAppAccessPolicyAllowsWithAppBonus(t *testing.T) {
	window, err := NewUsageWindow(
		mustTimeOfDay(t, 14, 0),
		mustTimeOfDay(t, 19, 0),
	)
	if err != nil {
		t.Fatalf("create usage window: %v", err)
	}

	policy := AppAccessPolicy{
		Schedule:     UsageSchedule{WeekdayWindows: []UsageWindow{window}},
		SharedBudget: DailyBudget{WeekdayLimit: 2 * time.Hour},
		AppLimit:     &AppDailyLimit{Limit: time.Hour},
		BreakRule:    BreakRule{After: 45 * time.Minute, Duration: 10 * time.Minute},
	}

	friday := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, time.November, 28, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		at             time.Time
		sharedUsed     time.Duration
		appUsed        time.Duration
		appRemaining   time.Duration
		breakStartedAt time.Time
		want           bool
	}{
		{"shared budget reached", friday, 2 * time.Hour, 30 * time.Minute, 15 * time.Minute, time.Time{}, true},
		{"app limit reached", friday, time.Hour, time.Hour, 15 * time.Minute, time.Time{}, true},
		{"both limits reached", friday, 2 * time.Hour, time.Hour, 15 * time.Minute, time.Time{}, true},
		{"no bonus for this app", friday, 2 * time.Hour, 30 * time.Minute, 0, time.Time{}, false},
		{"window closed", friday.Add(4 * time.Hour), 2 * time.Hour, time.Hour, 15 * time.Minute, time.Time{}, false},
		{"required break active", friday, 2 * time.Hour, time.Hour, 15 * time.Minute, friday, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bonus := DailyBonus{
				AppRemaining: tt.appRemaining,
				ExpiresAt:    expiresAt,
			}
			got := policy.AllowsWithBonus(tt.at, tt.sharedUsed, tt.appUsed, tt.breakStartedAt, bonus)
			if got != tt.want {
				t.Errorf("AllowsWithBonus() = %t, want %t", got, tt.want)
			}
		})
	}
}
