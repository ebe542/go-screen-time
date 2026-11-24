package usage

import (
	"testing"
	"time"
	_ "time/tzdata"
)

func TestLedgerCountsDailyUsagePerApp(t *testing.T) {
	ledger := NewLedger(time.UTC)
	start := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	switchAt := start.Add(20 * time.Minute)
	end := switchAt.Add(25 * time.Minute)

	ledger.Record("child-a", "app-a", start, switchAt)
	ledger.Record("child-a", "app-b", switchAt, end)

	tests := []struct {
		appID string
		want  time.Duration
	}{
		{"app-a", 20 * time.Minute},
		{"app-b", 25 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.appID, func(t *testing.T) {
			sharedUsed, appUsed := ledger.Totals("child-a", tt.appID, end)
			if sharedUsed != 45*time.Minute || appUsed != tt.want {
				t.Errorf("Totals() = (%s, %s), want (45m, %s)",
					sharedUsed, appUsed, tt.want)
			}
		})
	}
}

func TestLedgerKeepsChildTotalsSeparate(t *testing.T) {
	ledger := NewLedger(time.UTC)
	start := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)

	const appID = "shared-game"
	ledger.Record("child-a", appID, start, start.Add(20*time.Minute))
	ledger.Record("child-b", appID, start, start.Add(35*time.Minute))

	tests := []struct {
		childID string
		want    time.Duration
	}{
		{"child-a", 20 * time.Minute},
		{"child-b", 35 * time.Minute},
		{"child-without-usage", 0},
	}

	for _, tt := range tests {
		t.Run(tt.childID, func(t *testing.T) {
			sharedUsed, appUsed := ledger.Totals(tt.childID, appID, start)
			if sharedUsed != tt.want || appUsed != tt.want {
				t.Errorf("Totals() = (%s, %s), want (%s, %s)",
					sharedUsed, appUsed, tt.want, tt.want)
			}
		})
	}
}

func TestLedgerSplitsUsageAtLocalMidnight(t *testing.T) {
	local := time.FixedZone("UTC+1", 60*60)
	ledger := NewLedger(local)

	midnight := time.Date(2026, time.November, 28, 0, 0, 0, 0, local)
	start := midnight.Add(-10 * time.Minute).UTC()
	end := midnight.Add(20 * time.Minute).UTC()

	ledger.Record("child-a", "app-a", start, end)

	tests := []struct {
		name string
		at   time.Time
		want time.Duration
	}{
		{"before midnight", start, 10 * time.Minute},
		{"at midnight", midnight.UTC(), 20 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sharedUsed, appUsed := ledger.Totals("child-a", "app-a", tt.at)
			if sharedUsed != tt.want || appUsed != tt.want {
				t.Errorf("Totals() = (%s, %s), want (%s, %s)",
					sharedUsed, appUsed, tt.want, tt.want)
			}
		})
	}
}

func TestLedgerCountsElapsedTimeAcrossDST(t *testing.T) {
	local, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("load policy time zone: %v", err)
	}

	tests := []struct {
		name  string
		month time.Month
		day   int
		want  time.Duration
	}{
		{"spring forward", time.March, 29, 23 * time.Hour},
		{"fall back", time.October, 25, 25 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ledger := NewLedger(local)
			start := time.Date(2026, tt.month, tt.day, 0, 0, 0, 0, local)
			end := start.AddDate(0, 0, 1)

			ledger.Record("child-a", "app-a", start, end)

			sharedUsed, appUsed := ledger.Totals("child-a", "app-a", start)
			if sharedUsed != tt.want || appUsed != tt.want {
				t.Errorf("Totals() = (%s, %s), want (%s, %s)",
					sharedUsed, appUsed, tt.want, tt.want)
			}

			sharedUsed, appUsed = ledger.Totals("child-a", "app-a", end)
			if sharedUsed != 0 || appUsed != 0 {
				t.Errorf("next day totals = (%s, %s), want (0s, 0s)",
					sharedUsed, appUsed)
			}
		})
	}
}

func TestLedgerRecordsActualUsageAndBudgetCharges(t *testing.T) {
	ledger := NewLedger(time.UTC)
	at := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	allocation := Allocate(8*time.Minute, 5*time.Minute, 10*time.Minute)

	ledger.RecordAllocation("child-a", "app-a", at, allocation)

	sharedActual, appActual := ledger.Totals("child-a", "app-a", at)
	if sharedActual != 8*time.Minute || appActual != 8*time.Minute {
		t.Errorf("actual usage = (%s, %s), want (8m, 8m)", sharedActual, appActual)
	}

	wantA := BudgetUsage{
		SharedRegular: 5 * time.Minute,
		AppRegular:    5 * time.Minute,
		AppBonus:      3 * time.Minute,
	}
	if got := ledger.BudgetTotals("child-a", "app-a", at); got != wantA {
		t.Errorf("App A budget usage = %+v, want %+v", got, wantA)
	}

	wantB := BudgetUsage{SharedRegular: 5 * time.Minute}
	if got := ledger.BudgetTotals("child-a", "app-b", at); got != wantB {
		t.Errorf("App B budget usage = %+v, want %+v", got, wantB)
	}
}
func TestLedgerRecordWithBudgetUsesPreviousCharges(t *testing.T) {
	ledger := NewLedger(time.UTC)
	start := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	firstEnd := start.Add(8 * time.Minute)
	secondEnd := firstEnd.Add(9 * time.Minute)

	limits := DailyLimits{Shared: 5 * time.Minute}
	bonus := AppBonusAllowance{
		AppID:     "app-a",
		Granted:   10 * time.Minute,
		ExpiresAt: time.Date(2026, time.November, 28, 0, 0, 0, 0, time.UTC),
	}

	first := ledger.RecordWithBudget("child-a", "app-a", start, firstEnd, limits, bonus)
	wantFirst := Allocation{
		Actual:   8 * time.Minute,
		Regular:  5 * time.Minute,
		AppBonus: 3 * time.Minute,
	}
	if first != wantFirst {
		t.Errorf("first allocation = %+v, want %+v", first, wantFirst)
	}

	second := ledger.RecordWithBudget("child-a", "app-a", firstEnd, secondEnd, limits, bonus)
	wantSecond := Allocation{
		Actual:   9 * time.Minute,
		AppBonus: 7 * time.Minute,
	}
	if second != wantSecond {
		t.Errorf("second allocation = %+v, want %+v", second, wantSecond)
	}

	sharedActual, appActual := ledger.Totals("child-a", "app-a", start)
	if sharedActual != 17*time.Minute || appActual != 17*time.Minute {
		t.Errorf("actual usage = (%s, %s), want (17m, 17m)", sharedActual, appActual)
	}

	wantCharges := BudgetUsage{
		SharedRegular: 5 * time.Minute,
		AppRegular:    5 * time.Minute,
		AppBonus:      10 * time.Minute,
	}
	if got := ledger.BudgetTotals("child-a", "app-a", start); got != wantCharges {
		t.Errorf("budget usage = %+v, want %+v", got, wantCharges)
	}
}
func TestLedgerRecordWithBudgetKeepsAppAndDayLimitsSeparate(t *testing.T) {
	local := time.FixedZone("UTC+1", 60*60)
	ledger := NewLedger(local)
	midnight := time.Date(2026, time.November, 28, 0, 0, 0, 0, local)
	start := midnight.Add(-30 * time.Minute).UTC()

	appLimit := 2 * time.Minute
	limitsA := DailyLimits{Shared: 10 * time.Minute, App: &appLimit}
	limitsB := DailyLimits{Shared: 10 * time.Minute}
	bonusA := AppBonusAllowance{
		AppID:     "app-a",
		Granted:   5 * time.Minute,
		ExpiresAt: midnight,
	}

	tests := []struct {
		name   string
		appID  string
		start  time.Time
		length time.Duration
		limits DailyLimits
		want   Allocation
	}{
		{
			name:   "app A uses bonus after its own limit",
			appID:  "app-a",
			start:  start,
			length: 5 * time.Minute,
			limits: limitsA,
			want:   Allocation{Actual: 5 * time.Minute, Regular: 2 * time.Minute, AppBonus: 3 * time.Minute},
		},
		{
			name:   "app B uses only the remaining shared time",
			appID:  "app-b",
			start:  start.Add(5 * time.Minute),
			length: 9 * time.Minute,
			limits: limitsB,
			want:   Allocation{Actual: 9 * time.Minute, Regular: 8 * time.Minute},
		},
		{
			name:   "next local day has fresh limits and no old bonus",
			appID:  "app-a",
			start:  midnight.UTC(),
			length: 4 * time.Minute,
			limits: limitsA,
			want:   Allocation{Actual: 4 * time.Minute, Regular: 2 * time.Minute},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ledger.RecordWithBudget(
				"child-a", tt.appID, tt.start, tt.start.Add(tt.length), tt.limits, bonusA,
			)
			if got != tt.want {
				t.Errorf("RecordWithBudget = %+v, want %+v", got, tt.want)
			}
		})
	}
}
