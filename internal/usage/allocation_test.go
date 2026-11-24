package usage

import (
	"testing"
	"time"
)

func TestAllocateUsesRegularTimeBeforeAppBonus(t *testing.T) {
	got := Allocate(8*time.Minute, 5*time.Minute, 10*time.Minute)

	if got.Actual != 8*time.Minute {
		t.Errorf("actual usage = %s, want 8m", got.Actual)
	}
	if got.Regular != 5*time.Minute {
		t.Errorf("regular usage = %s, want 5m", got.Regular)
	}
	if got.AppBonus != 3*time.Minute {
		t.Errorf("app bonus usage = %s, want 3m", got.AppBonus)
	}
}

func TestAllocateIntervalHonorsAppBonusScopeAndExpiry(t *testing.T) {
	start := time.Date(2026, time.November, 27, 23, 55, 0, 0, time.UTC)
	end := start.Add(8 * time.Minute)
	bonus := AppBonusGrant{
		AppID:     "app-a",
		Remaining: 10 * time.Minute,
		ExpiresAt: start.Add(5 * time.Minute),
	}

	tests := []struct {
		appID     string
		wantBonus time.Duration
	}{
		{"app-a", 3 * time.Minute},
		{"app-b", 0},
	}

	for _, tt := range tests {
		t.Run(tt.appID, func(t *testing.T) {
			got := AllocateInterval(tt.appID, start, end, 2*time.Minute, bonus)
			want := Allocation{
				Actual:   8 * time.Minute,
				Regular:  2 * time.Minute,
				AppBonus: tt.wantBonus,
			}
			if got != want {
				t.Errorf("AllocateInterval = %+v, want %+v", got, want)
			}
		})
	}
}
