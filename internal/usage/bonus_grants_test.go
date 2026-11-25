package usage

import (
	"testing"
	"time"

	"github.com/ebe542/go-screen-time/internal/policy"
)

func TestLedgerAddsSharedBonusGrantsForOneChild(t *testing.T) {
	ledger := NewLedger(time.UTC)
	first := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	second := first.Add(time.Hour)

	if err := ledger.GrantBonus("child-a", "", first, 5*time.Minute); err != nil {
		t.Fatalf("grant first bonus: %v", err)
	}
	if err := ledger.GrantBonus("child-a", "", second, 7*time.Minute); err != nil {
		t.Fatalf("grant second bonus: %v", err)
	}

	if got := ledger.BonusesFor("child-a", "app-a", second).Shared.Granted; got != 12*time.Minute {
		t.Errorf("child A shared bonus = %s, want 12m", got)
	}
	if got := ledger.BonusesFor("child-b", "app-a", second).Shared.Granted; got != 0 {
		t.Errorf("child B shared bonus = %s, want 0s", got)
	}
}
func TestLedgerAppBonusAppliesOnlyToItsAppAndPolicyDay(t *testing.T) {
	local := time.FixedZone("UTC+1", 60*60)
	ledger := NewLedger(local)
	midnight := time.Date(2026, time.November, 28, 0, 0, 0, 0, local)
	grantedAt := midnight.Add(-10 * time.Minute).UTC()

	if err := ledger.GrantBonus("child-a", "app-a", grantedAt, 8*time.Minute); err != nil {
		t.Fatalf("grant app bonus: %v", err)
	}

	tests := []struct {
		name    string
		childID string
		appID   string
		at      time.Time
		want    time.Duration
	}{
		{"before grant", "child-a", "app-a", grantedAt.Add(-time.Second), 0},
		{"at grant", "child-a", "app-a", grantedAt, 8 * time.Minute},
		{"other app", "child-a", "app-b", grantedAt, 0},
		{"other child", "child-b", "app-a", grantedAt, 0},
		{"before midnight", "child-a", "app-a", midnight.Add(-time.Second).UTC(), 8 * time.Minute},
		{"at local midnight", "child-a", "app-a", midnight.UTC(), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bonuses := ledger.BonusesFor(tt.childID, tt.appID, tt.at)
			if bonuses.Shared.Granted != 0 || bonuses.App.Granted != tt.want {
				t.Errorf("granted = (shared %s, app %s), want (shared 0s, app %s)",
					bonuses.Shared.Granted, bonuses.App.Granted, tt.want)
			}
			if tt.want > 0 &&
				(bonuses.App.AppID != tt.appID || !bonuses.App.ExpiresAt.Equal(midnight)) {
				t.Errorf("app bonus = %+v, want app %q with expiry %v",
					bonuses.App, tt.appID, midnight)
			}
		})
	}
}
func TestLedgerUsesDirectGrantsFromTheirStartTime(t *testing.T) {
	ledger := NewLedger(time.UTC)
	start := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	grantedAt := start.Add(2 * time.Minute)
	end := start.Add(5 * time.Minute)

	endOfDay, err := policy.NewTimeOfDay(24, 0)
	if err != nil {
		t.Fatalf("create end of day: %v", err)
	}
	window, err := policy.NewUsageWindow(policy.TimeOfDay{}, endOfDay)
	if err != nil {
		t.Fatalf("create usage window: %v", err)
	}
	access := policy.AppAccessPolicy{
		Schedule: policy.UsageSchedule{
			WeekdayWindows: []policy.UsageWindow{window},
		},
	}

	if err := ledger.GrantBonus("child-a", "", grantedAt, time.Minute); err != nil {
		t.Fatalf("grant shared bonus: %v", err)
	}
	if err := ledger.GrantBonus("child-a", "app-a", grantedAt, 2*time.Minute); err != nil {
		t.Fatalf("grant app bonus: %v", err)
	}

	noBreak := time.Time{}
	if ledger.AllowsAppWithGrants("child-a", "app-a", start, access, noBreak) {
		t.Error("app is allowed before the grants take effect")
	}
	if !ledger.AllowsAppWithGrants("child-a", "app-a", grantedAt, access, noBreak) {
		t.Fatal("app is blocked when the grants take effect")
	}

	ledger.RecordAppUsageWithGrants("child-a", "app-a", start, end, access)

	shared, app := ledger.Totals("child-a", "app-a", end)
	if shared != 5*time.Minute || app != 5*time.Minute {
		t.Errorf("actual usage = (%s, %s), want (5m, 5m)", shared, app)
	}
	want := BudgetUsage{
		SharedRegular: time.Minute,
		AppRegular:    time.Minute,
		AppBonus:      2 * time.Minute,
	}
	if got := ledger.BudgetTotals("child-a", "app-a", end); got != want {
		t.Errorf("budget usage = %+v, want %+v", got, want)
	}
	if ledger.AllowsAppWithGrants("child-a", "app-a", end, access, noBreak) {
		t.Error("app is allowed after both grants are exhausted")
	}
}
