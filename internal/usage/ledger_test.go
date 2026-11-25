package usage

import (
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/ebe542/go-screen-time/internal/policy"
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

func TestLedgerRecordAcrossDaysWithBudget(t *testing.T) {
	local := time.FixedZone("UTC+1", 60*60)
	ledger := NewLedger(local)
	midnight := time.Date(2026, time.November, 28, 0, 0, 0, 0, local)
	start := midnight.Add(-5 * time.Minute).UTC()
	end := midnight.Add(5 * time.Minute).UTC()

	ledger.RecordAcrossDaysWithBudget(
		"child-a", "app-a", start, end,
		func(at time.Time) (DailyLimits, AppBonusAllowance) {
			if at.Weekday() == time.Friday {
				return DailyLimits{Shared: 2 * time.Minute}, AppBonusAllowance{
					AppID:     "app-a",
					Granted:   3 * time.Minute,
					ExpiresAt: midnight,
				}
			}
			return DailyLimits{Shared: 5 * time.Minute}, AppBonusAllowance{}
		},
	)

	tests := []struct {
		name string
		at   time.Time
		want BudgetUsage
	}{
		{
			name: "Friday",
			at:   start,
			want: BudgetUsage{
				SharedRegular: 2 * time.Minute,
				AppRegular:    2 * time.Minute,
				AppBonus:      3 * time.Minute,
			},
		},
		{
			name: "Saturday",
			at:   midnight.UTC(),
			want: BudgetUsage{
				SharedRegular: 5 * time.Minute,
				AppRegular:    5 * time.Minute,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shared, app := ledger.Totals("child-a", "app-a", tt.at)
			if shared != 5*time.Minute || app != 5*time.Minute {
				t.Errorf("Totals() = (%s, %s), want (5m, 5m)", shared, app)
			}
			if got := ledger.BudgetTotals("child-a", "app-a", tt.at); got != tt.want {
				t.Errorf("BudgetTotals() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLedgerRecordAcrossDaysStopsBonusAtMidnight(t *testing.T) {
	local := time.FixedZone("UTC+1", 60*60)
	midnight := time.Date(2026, time.November, 28, 0, 0, 0, 0, local)
	start := midnight.Add(-5 * time.Minute).UTC()
	bonus := AppBonusAllowance{
		AppID:     "app-a",
		Granted:   10 * time.Minute,
		ExpiresAt: midnight,
	}

	tests := []struct {
		name         string
		saturdayTime time.Duration
		wantLookups  int
	}{
		{"ends at midnight", 0, 1},
		{"continues after midnight", 5 * time.Minute, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ledger := NewLedger(local)
			lookups := 0

			ledger.RecordAcrossDaysWithBudget(
				"child-a", "app-a", start, midnight.Add(tt.saturdayTime).UTC(),
				func(at time.Time) (DailyLimits, AppBonusAllowance) {
					lookups++
					return DailyLimits{}, bonus
				},
			)

			if lookups != tt.wantLookups {
				t.Errorf("budget lookups = %d, want %d", lookups, tt.wantLookups)
			}
			if got := ledger.BudgetTotals("child-a", "app-a", start).AppBonus; got != 5*time.Minute {
				t.Errorf("Friday bonus used = %s, want 5m", got)
			}
			if got := ledger.BudgetTotals("child-a", "app-a", midnight); got != (BudgetUsage{}) {
				t.Errorf("Saturday budget usage = %+v, want zero charges", got)
			}
			shared, app := ledger.Totals("child-a", "app-a", midnight)
			if shared != tt.saturdayTime || app != tt.saturdayTime {
				t.Errorf("Saturday actual usage = (%s, %s), want (%s, %s)",
					shared, app, tt.saturdayTime, tt.saturdayTime)
			}
		})
	}
}

func TestLedgerAllowsAppUsesRecordedBudgetCharges(t *testing.T) {
	ledger := NewLedger(time.UTC)
	start := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Minute)

	endOfDay, err := policy.NewTimeOfDay(24, 0)
	if err != nil {
		t.Fatalf("create end of day: %v", err)
	}
	window, err := policy.NewUsageWindow(policy.TimeOfDay{}, endOfDay)
	if err != nil {
		t.Fatalf("create usage window: %v", err)
	}

	sharedLimit := 10 * time.Minute
	appLimit := 2 * time.Minute
	appA := policy.AppAccessPolicy{
		Schedule: policy.UsageSchedule{
			WeekdayWindows: []policy.UsageWindow{window},
		},
		SharedBudget: policy.DailyBudget{WeekdayLimit: sharedLimit},
		AppLimit:     &policy.AppDailyLimit{Limit: appLimit},
	}
	appB := appA
	appB.AppLimit = nil

	bonusA := AppBonusAllowance{
		AppID:     "app-a",
		Granted:   3 * time.Minute,
		ExpiresAt: time.Date(2026, time.November, 28, 0, 0, 0, 0, time.UTC),
	}
	noBreak := time.Time{}

	if !ledger.AllowsApp("child-a", "app-a", start, appA, noBreak, bonusA) {
		t.Fatal("app A is blocked before any usage")
	}

	ledger.RecordWithBudget(
		"child-a", "app-a", start, end,
		DailyLimits{Shared: sharedLimit, App: &appLimit}, bonusA,
	)

	if ledger.AllowsApp("child-a", "app-a", end, appA, noBreak, bonusA) {
		t.Error("app A is allowed after using its regular time and bonus")
	}
	if !ledger.AllowsApp("child-a", "app-b", end, appB, noBreak, AppBonusAllowance{}) {
		t.Error("app B is blocked despite remaining shared time")
	}
}
func TestLedgerAppliesSharedBonusToRecordingAndAccess(t *testing.T) {
	ledger := NewLedger(time.UTC)
	start := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	afterTen := start.Add(10 * time.Minute)
	afterTwelve := start.Add(12 * time.Minute)
	afterFifteen := start.Add(15 * time.Minute)

	endOfDay, err := policy.NewTimeOfDay(24, 0)
	if err != nil {
		t.Fatalf("create end of day: %v", err)
	}
	window, err := policy.NewUsageWindow(policy.TimeOfDay{}, endOfDay)
	if err != nil {
		t.Fatalf("create usage window: %v", err)
	}

	appA := policy.AppAccessPolicy{
		Schedule: policy.UsageSchedule{
			WeekdayWindows: []policy.UsageWindow{window},
		},
		SharedBudget: policy.DailyBudget{WeekdayLimit: 10 * time.Minute},
		AppLimit:     &policy.AppDailyLimit{Limit: 12 * time.Minute},
	}
	appB := appA
	appB.AppLimit = nil

	bonuses := AppBonuses{
		Shared: SharedBonusAllowance{
			Granted:   5 * time.Minute,
			ExpiresAt: time.Date(2026, time.November, 28, 0, 0, 0, 0, time.UTC),
		},
	}
	noBreak := time.Time{}

	ledger.RecordAppUsage("child-a", "app-a", start, afterTen, appA, bonuses)
	if !ledger.AllowsAppWithBonuses("child-a", "app-a", afterTen, appA, noBreak, bonuses) {
		t.Fatal("app A is blocked before it can use the shared bonus")
	}

	ledger.RecordAppUsage("child-a", "app-a", afterTen, afterTwelve, appA, bonuses)
	if ledger.AllowsAppWithBonuses("child-a", "app-a", afterTwelve, appA, noBreak, bonuses) {
		t.Error("app A is allowed after reaching its individual limit")
	}
	if !ledger.AllowsAppWithBonuses("child-a", "app-b", afterTwelve, appB, noBreak, bonuses) {
		t.Fatal("app B is blocked despite three minutes of shared time remaining")
	}

	ledger.RecordAppUsage("child-a", "app-b", afterTwelve, afterFifteen, appB, bonuses)
	if ledger.AllowsAppWithBonuses("child-a", "app-b", afterFifteen, appB, noBreak, bonuses) {
		t.Error("app B is allowed after the shared budget and bonus are exhausted")
	}

	want := BudgetUsage{SharedRegular: 15 * time.Minute, AppRegular: 3 * time.Minute}
	if got := ledger.BudgetTotals("child-a", "app-b", start); got != want {
		t.Errorf("app B budget usage = %+v, want %+v", got, want)
	}
}
func TestLedgerCombinesSharedAndAppBonuses(t *testing.T) {
	ledger := NewLedger(time.UTC)
	start := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	midnight := time.Date(2026, time.November, 28, 0, 0, 0, 0, time.UTC)

	endOfDay, err := policy.NewTimeOfDay(24, 0)
	if err != nil {
		t.Fatalf("create end of day: %v", err)
	}
	window, err := policy.NewUsageWindow(policy.TimeOfDay{}, endOfDay)
	if err != nil {
		t.Fatalf("create usage window: %v", err)
	}

	appA := policy.AppAccessPolicy{
		Schedule:     policy.UsageSchedule{WeekdayWindows: []policy.UsageWindow{window}},
		SharedBudget: policy.DailyBudget{WeekdayLimit: 10 * time.Minute},
		AppLimit:     &policy.AppDailyLimit{Limit: 12 * time.Minute},
	}
	appB := appA
	appB.AppLimit = nil

	bonuses := AppBonuses{
		Shared: SharedBonusAllowance{Granted: 5 * time.Minute, ExpiresAt: midnight},
		App:    AppBonusAllowance{AppID: "app-a", Granted: 3 * time.Minute, ExpiresAt: midnight},
	}

	afterA := start.Add(13 * time.Minute)
	ledger.RecordAppUsage("child-a", "app-a", start, afterA, appA, bonuses)

	wantA := BudgetUsage{
		SharedRegular: 12 * time.Minute,
		AppRegular:    12 * time.Minute,
		AppBonus:      time.Minute,
	}
	if got := ledger.BudgetTotals("child-a", "app-a", afterA); got != wantA {
		t.Errorf("app A usage = %+v, want %+v", got, wantA)
	}

	afterB := afterA.Add(3 * time.Minute)
	ledger.RecordAppUsage("child-a", "app-b", afterA, afterB, appB, bonuses)

	if ledger.AllowsAppWithBonuses("child-a", "app-b", afterB, appB, time.Time{}, bonuses) {
		t.Error("app B is allowed after the shared time is exhausted")
	}
	if !ledger.AllowsAppWithBonuses("child-a", "app-a", afterB, appA, time.Time{}, bonuses) {
		t.Fatal("app A is blocked despite two minutes of app bonus remaining")
	}

	end := afterB.Add(2 * time.Minute)
	ledger.RecordAppUsage("child-a", "app-a", afterB, end, appA, bonuses)
	if ledger.AllowsAppWithBonuses("child-a", "app-a", end, appA, time.Time{}, bonuses) {
		t.Error("app A is allowed after its app bonus is exhausted")
	}
}
func TestLedgerSharedBonusExpiresAtLocalMidnight(t *testing.T) {
	local := time.FixedZone("UTC+1", 60*60)
	ledger := NewLedger(local)
	midnight := time.Date(2026, time.November, 28, 0, 0, 0, 0, local)
	start := midnight.Add(-2 * time.Minute).UTC()
	end := midnight.Add(2 * time.Minute).UTC()

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
			WeekendWindows: []policy.UsageWindow{window},
		},
	}
	bonuses := AppBonuses{
		Shared: SharedBonusAllowance{
			Granted:   5 * time.Minute,
			ExpiresAt: midnight,
		},
	}

	ledger.RecordAppUsage("child-a", "app-a", start, midnight.UTC(), access, bonuses)

	beforeMidnight := midnight.Add(-time.Second).UTC()
	if !ledger.AllowsAppWithBonuses("child-a", "app-a", beforeMidnight, access, time.Time{}, bonuses) {
		t.Error("app is blocked while Friday's shared bonus is still available")
	}
	if ledger.AllowsAppWithBonuses("child-a", "app-a", midnight.UTC(), access, time.Time{}, bonuses) {
		t.Error("app is allowed at the shared bonus expiry")
	}

	ledger.RecordAppUsage("child-a", "app-a", midnight.UTC(), end, access, bonuses)

	if got := ledger.BudgetTotals("child-a", "app-a", start).SharedRegular; got != 2*time.Minute {
		t.Errorf("Friday shared usage = %s, want 2m", got)
	}
	if got := ledger.BudgetTotals("child-a", "app-a", end); got != (BudgetUsage{}) {
		t.Errorf("Saturday budget usage = %+v, want zero charges", got)
	}
	shared, app := ledger.Totals("child-a", "app-a", end)
	if shared != 2*time.Minute || app != 2*time.Minute {
		t.Errorf("Saturday actual usage = (%s, %s), want (2m, 2m)", shared, app)
	}
}
