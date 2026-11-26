package sqlite

import (
	"bytes"
	"crypto/sha256"
	"path/filepath"
	"testing"
	"time"

	"github.com/ebe542/go-screen-time/internal/policy"
	"github.com/ebe542/go-screen-time/internal/usage"
)

func TestSQLiteLedgerRestoresGrantsChargesAndCodes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.db")
	at := sqliteTestTime()
	expiry := at.Add(time.Hour)
	ledger := mustOpenSQLiteLedger(t, path, time.UTC)

	if err := ledger.GrantBonus("child-a", "", at, 2*time.Minute); err != nil {
		t.Fatalf("grant shared bonus: %v", err)
	}
	usedCode := mustIssueSQLiteBonusCode(t, ledger, "child-a", "app-a", at, expiry, 5*time.Minute)
	unusedCode := mustIssueSQLiteBonusCode(t, ledger, "child-a", "app-b", at, expiry, time.Minute)
	mustRedeemSQLiteBonusCode(t, ledger, "child-a", usedCode, at)

	access := sqliteTestAccessPolicy(t)
	usedThrough := at.Add(4 * time.Minute)
	if err := ledger.RecordAppUsageWithGrants("child-a", "app-a", at, usedThrough, access); err != nil {
		t.Fatalf("record usage: %v", err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatalf("close before restart: %v", err)
	}

	ledger = mustOpenSQLiteLedger(t, path, time.UTC)
	defer ledger.Close()

	bonuses := mustReadSQLiteBonuses(t, ledger, "child-a", "app-a", usedThrough)
	if bonuses.Shared.Granted != 2*time.Minute || bonuses.App.Granted != 5*time.Minute {
		t.Errorf("restored grants = %+v, want 2m shared and 5m app", bonuses)
	}

	want := usage.BudgetUsage{
		SharedRegular: 2 * time.Minute,
		AppRegular:    2 * time.Minute,
		AppBonus:      2 * time.Minute,
	}
	got, err := ledger.BudgetTotals("child-a", "app-a", usedThrough)
	if err != nil {
		t.Fatalf("read restored charges: %v", err)
	}
	if got != want {
		t.Errorf("restored charges = %+v, want %+v", got, want)
	}

	mustCheckSQLiteAccess(t, ledger, usedThrough, access, true)
	exhaustedAt := usedThrough.Add(3 * time.Minute)
	if err := ledger.RecordAppUsageWithGrants(
		"child-a", "app-a", usedThrough, exhaustedAt, access,
	); err != nil {
		t.Fatalf("record remaining bonus usage: %v", err)
	}
	mustCheckSQLiteAccess(t, ledger, exhaustedAt, access, false)

	if err := ledger.RedeemBonusCode("child-a", usedCode, exhaustedAt); err == nil {
		t.Error("redeemed code worked again after restart")
	}
	mustRedeemSQLiteBonusCode(t, ledger, "child-a", unusedCode, exhaustedAt)
	bonuses = mustReadSQLiteBonuses(t, ledger, "child-a", "app-b", exhaustedAt)
	if bonuses.App.Granted != time.Minute {
		t.Errorf("app B bonus = %s, want 1m", bonuses.App.Granted)
	}
}

func TestSQLiteLedgerRollsBackRedemptionWhenGrantWriteFails(t *testing.T) {
	ledger := mustOpenSQLiteLedger(t, filepath.Join(t.TempDir(), "usage.db"), time.UTC)
	defer ledger.Close()

	at := sqliteTestTime()
	code := mustIssueSQLiteBonusCode(t, ledger, "child-a", "", at, at.Add(time.Hour), time.Minute)

	_, err := ledger.db.Exec(`
		CREATE TRIGGER reject_test_grant
		BEFORE INSERT ON bonus_grants
		BEGIN
			SELECT RAISE(ABORT, 'test grant write failure');
		END
	`)
	if err != nil {
		t.Fatalf("install failure trigger: %v", err)
	}
	if err := ledger.RedeemBonusCode("child-a", code, at); err == nil {
		t.Error("redemption succeeded despite failed grant write")
	}
	if got := mustReadSQLiteBonuses(t, ledger, "child-a", "app-a", at).Shared.Granted; got != 0 {
		t.Errorf("failed redemption granted %s, want 0s", got)
	}

	if _, err := ledger.db.Exec("DROP TRIGGER reject_test_grant"); err != nil {
		t.Fatalf("remove failure trigger: %v", err)
	}
	mustRedeemSQLiteBonusCode(t, ledger, "child-a", code, at)
	if got := mustReadSQLiteBonuses(t, ledger, "child-a", "app-a", at).Shared.Granted; got != time.Minute {
		t.Errorf("retried redemption granted %s, want 1m", got)
	}
}

func TestSQLiteLedgerRedeemsOnlyOnceAcrossConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.db")
	first := mustOpenSQLiteLedger(t, path, time.UTC)
	defer first.Close()
	second := mustOpenSQLiteLedger(t, path, time.UTC)
	defer second.Close()

	at := sqliteTestTime()
	code := mustIssueSQLiteBonusCode(t, first, "child-a", "", at, at.Add(time.Hour), time.Minute)
	start := make(chan struct{})
	results := make(chan error, 2)

	for _, ledger := range []*Ledger{first, second} {
		go func() {
			<-start
			results <- ledger.RedeemBonusCode("child-a", code, at)
		}()
	}
	close(start)

	successes := 0
	for range 2 {
		if err := <-results; err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Errorf("successful redemptions = %d, want 1", successes)
	}
	if got := mustReadSQLiteBonuses(t, second, "child-a", "app-a", at).Shared.Granted; got != time.Minute {
		t.Errorf("stored bonus = %s, want 1m", got)
	}
}

func TestSQLiteLedgerPreservesExpiryAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.db")
	local := time.FixedZone("UTC+1", 60*60)
	midnight := time.Date(2026, time.November, 28, 0, 0, 0, 0, local)
	at := midnight.Add(-time.Hour).UTC()
	codeExpiry := midnight.Add(-time.Minute)

	ledger := mustOpenSQLiteLedger(t, path, local)
	code := mustIssueSQLiteBonusCode(t, ledger, "child-a", "", at, codeExpiry, time.Minute)
	if err := ledger.GrantBonus("child-a", "app-a", at, 2*time.Minute); err != nil {
		t.Fatalf("grant app bonus: %v", err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatalf("close before restart: %v", err)
	}

	ledger = mustOpenSQLiteLedger(t, path, local)
	defer ledger.Close()

	if err := ledger.RedeemBonusCode("child-a", code, codeExpiry.UTC()); err == nil {
		t.Error("code was accepted at its expiry after restart")
	}
	before := mustReadSQLiteBonuses(t, ledger, "child-a", "app-a", midnight.Add(-time.Second).UTC())
	if before.App.Granted != 2*time.Minute || !before.App.ExpiresAt.Equal(midnight) {
		t.Errorf("bonus before midnight = %+v, want 2m expiring at midnight", before.App)
	}
	if got := mustReadSQLiteBonuses(t, ledger, "child-a", "app-a", midnight.UTC()); got.App.Granted != 0 {
		t.Errorf("expired app bonus = %s, want 0s", got.App.Granted)
	}
}

func TestSQLiteLedgerKeepsRedemptionLimitPerChildAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.db")
	at := sqliteTestTime()
	expiry := at.Add(time.Hour)

	ledger := mustOpenSQLiteLedger(t, path, time.UTC)
	codeA := mustIssueSQLiteBonusCode(t, ledger, "child-a", "", at, expiry, time.Minute)
	codeB := mustIssueSQLiteBonusCode(t, ledger, "child-b", "", at, expiry, time.Minute)

	for range 5 {
		if err := ledger.RedeemBonusCode("child-a", "unknown-code", at); err == nil {
			t.Fatal("unknown code was accepted")
		}
	}
	if err := ledger.Close(); err != nil {
		t.Fatalf("close before restart: %v", err)
	}

	ledger = mustOpenSQLiteLedger(t, path, time.UTC)
	defer ledger.Close()

	if err := ledger.RedeemBonusCode("child-a", codeA, at.Add(30*time.Second)); err == nil {
		t.Error("child A redeemed a code during the blocked minute")
	}
	mustRedeemSQLiteBonusCode(t, ledger, "child-b", codeB, at.Add(30*time.Second))
	mustRedeemSQLiteBonusCode(t, ledger, "child-a", codeA, at.Add(time.Minute))
}

func TestSQLiteLedgerStoresCodeDigest(t *testing.T) {
	ledger := mustOpenSQLiteLedger(t, filepath.Join(t.TempDir(), "usage.db"), time.UTC)
	defer ledger.Close()

	at := sqliteTestTime()
	code := mustIssueSQLiteBonusCode(t, ledger, "child-a", "", at, at.Add(time.Hour), time.Minute)

	var stored []byte
	if err := ledger.db.QueryRow("SELECT digest FROM bonus_codes").Scan(&stored); err != nil {
		t.Fatalf("read stored digest: %v", err)
	}
	want := sha256.Sum256([]byte(code))
	if !bytes.Equal(stored, want[:]) {
		t.Error("stored verifier is not the SHA-256 digest of the issued code")
	}
}

func sqliteTestTime() time.Time {
	return time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
}

func mustOpenSQLiteLedger(t *testing.T, path string, location *time.Location) *Ledger {
	t.Helper()
	ledger, err := Open(path, location)
	if err != nil {
		t.Fatalf("open SQLite ledger: %v", err)
	}
	return ledger
}

func mustIssueSQLiteBonusCode(
	t *testing.T,
	ledger *Ledger,
	childID, appID string,
	at, expiry time.Time,
	duration time.Duration,
) string {
	t.Helper()
	code, err := ledger.IssueBonusCode(childID, appID, at, expiry, duration)
	if err != nil {
		t.Fatalf("issue bonus code: %v", err)
	}
	if code == "" {
		t.Fatal("issued bonus code is empty")
	}
	return code
}

func mustRedeemSQLiteBonusCode(t *testing.T, ledger *Ledger, childID, code string, at time.Time) {
	t.Helper()
	if err := ledger.RedeemBonusCode(childID, code, at); err != nil {
		t.Fatalf("redeem bonus code for %s: %v", childID, err)
	}
}

func mustReadSQLiteBonuses(t *testing.T, ledger *Ledger, childID, appID string, at time.Time) usage.AppBonuses {
	t.Helper()
	bonuses, err := ledger.BonusesFor(childID, appID, at)
	if err != nil {
		t.Fatalf("read stored bonuses: %v", err)
	}
	return bonuses
}

func mustCheckSQLiteAccess(
	t *testing.T,
	ledger *Ledger,
	at time.Time,
	access policy.AppAccessPolicy,
	want bool,
) {
	t.Helper()
	got, err := ledger.AllowsAppWithGrants("child-a", "app-a", at, access, time.Time{})
	if err != nil {
		t.Fatalf("check app access: %v", err)
	}
	if got != want {
		t.Errorf("app access at %v = %t, want %t", at, got, want)
	}
}

func sqliteTestAccessPolicy(t *testing.T) policy.AppAccessPolicy {
	t.Helper()
	endOfDay, err := policy.NewTimeOfDay(24, 0)
	if err != nil {
		t.Fatalf("create end of day: %v", err)
	}
	window, err := policy.NewUsageWindow(policy.TimeOfDay{}, endOfDay)
	if err != nil {
		t.Fatalf("create usage window: %v", err)
	}

	return policy.AppAccessPolicy{
		Schedule: policy.UsageSchedule{WeekdayWindows: []policy.UsageWindow{window}},
	}
}
func TestSQLiteLedgerRedeemsVerifiedBonusOnlyOnceAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.db")
	at := sqliteTestTime()
	grant := usage.VerifiedBonusGrant{
		ID:       "grant-1",
		ChildID:  "child-a",
		AppID:    "app-a",
		Duration: 5 * time.Minute,
	}

	ledger := mustOpenSQLiteLedger(t, path, time.UTC)
	if err := ledger.RedeemVerifiedBonus(grant, at); err != nil {
		_ = ledger.Close()
		t.Fatalf("redeem without pre-issued code: %v", err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatalf("close before restart: %v", err)
	}

	ledger = mustOpenSQLiteLedger(t, path, time.UTC)
	defer ledger.Close()

	// A re-encrypted copy carries the same grant ID.
	if err := ledger.RedeemVerifiedBonus(grant, at.Add(time.Second)); err == nil {
		t.Error("grant was accepted again after restart")
	}
	if got := mustReadSQLiteBonuses(t, ledger, "child-a", "app-a", at).App.Granted; got != 5*time.Minute {
		t.Errorf("app bonus after replay = %s, want 5m", got)
	}

	grant.ID = "grant-2"
	if err := ledger.RedeemVerifiedBonus(grant, at.Add(time.Second)); err != nil {
		t.Fatalf("redeem independent grant with the same terms: %v", err)
	}
	if got := mustReadSQLiteBonuses(t, ledger, "child-a", "app-a", at.Add(time.Second)).App.Granted; got != 10*time.Minute {
		t.Errorf("app bonus after both grants = %s, want 10m", got)
	}
}

func TestSQLiteLedgerRollsBackVerifiedBonusWhenGrantWriteFails(t *testing.T) {
	ledger := mustOpenSQLiteLedger(t, filepath.Join(t.TempDir(), "usage.db"), time.UTC)
	defer ledger.Close()

	at := sqliteTestTime()
	grant := usage.VerifiedBonusGrant{
		ID:       "grant-1",
		ChildID:  "child-a",
		Duration: time.Minute,
	}

	if _, err := ledger.db.Exec(`
		CREATE TRIGGER reject_verified_grant
		BEFORE INSERT ON bonus_grants
		BEGIN
			SELECT RAISE(ABORT, 'test grant write failure');
		END
	`); err != nil {
		t.Fatalf("install failure trigger: %v", err)
	}

	if err := ledger.RedeemVerifiedBonus(grant, at); err == nil {
		t.Error("redemption succeeded despite the failed grant write")
	}
	if got := mustReadSQLiteBonuses(t, ledger, "child-a", "app-a", at).Shared.Granted; got != 0 {
		t.Errorf("shared bonus after failed write = %s, want 0s", got)
	}

	if _, err := ledger.db.Exec("DROP TRIGGER reject_verified_grant"); err != nil {
		t.Fatalf("remove failure trigger: %v", err)
	}
	if err := ledger.RedeemVerifiedBonus(grant, at); err != nil {
		t.Fatalf("retry the same grant ID: %v", err)
	}
	if got := mustReadSQLiteBonuses(t, ledger, "child-a", "app-a", at).Shared.Granted; got != time.Minute {
		t.Errorf("shared bonus after retry = %s, want 1m", got)
	}
}

func TestSQLiteLedgerRedeemsVerifiedBonusOnceAcrossConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.db")
	first := mustOpenSQLiteLedger(t, path, time.UTC)
	defer first.Close()
	second := mustOpenSQLiteLedger(t, path, time.UTC)
	defer second.Close()

	at := sqliteTestTime()
	grant := usage.VerifiedBonusGrant{
		ID:       "grant-1",
		ChildID:  "child-a",
		Duration: time.Minute,
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, ledger := range []*Ledger{first, second} {
		go func() {
			<-start
			results <- ledger.RedeemVerifiedBonus(grant, at)
		}()
	}
	close(start)

	successes := 0
	for range 2 {
		if err := <-results; err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Errorf("successful redemptions = %d, want 1", successes)
	}
	if got := mustReadSQLiteBonuses(t, second, "child-a", "app-a", at).Shared.Granted; got != time.Minute {
		t.Errorf("shared bonus = %s, want 1m", got)
	}
}
func TestSQLiteLedgerRollsBackUsageWhenBreakStateWriteFails(t *testing.T) {
	store := mustOpenSQLiteLedger(t, filepath.Join(t.TempDir(), "usage.db"), time.UTC)
	defer store.Close()

	const childID = "child-a"
	const appID = "app-a"
	start := sqliteTestTime()
	end := start.Add(15 * time.Minute)

	access := sqliteTestAccessPolicy(t)
	access.SharedBudget.WeekdayLimit = time.Hour
	state := policy.BreakState{
		Used:      15 * time.Minute,
		StartedAt: end,
	}

	if _, err := store.db.Exec(`
		CREATE TRIGGER reject_break_state_write
		BEFORE INSERT ON break_states
		BEGIN
			SELECT RAISE(ABORT, 'test break state write failure');
		END
	`); err != nil {
		t.Fatalf("install break state failure trigger: %v", err)
	}

	err := store.RecordAppUsageAndBreakState(childID, appID, start, end, access, state)
	if err == nil {
		t.Error("booking succeeded despite a failed break state write")
	}
	charges, err := store.BudgetTotals(childID, appID, start)
	if err != nil {
		t.Fatalf("read budget totals: %v", err)
	}
	if charges != (usage.BudgetUsage{}) {
		t.Errorf("charges after failed write = %+v, want zero charges", charges)
	}
	saved, err := store.LoadBreakState(childID)
	if err != nil || saved != (policy.BreakState{}) {
		t.Errorf("state after failed write = (%+v, %v), want zero state", saved, err)
	}

	if _, err := store.db.Exec("DROP TRIGGER reject_break_state_write"); err != nil {
		t.Fatalf("remove failure trigger: %v", err)
	}
	if err := store.RecordAppUsageAndBreakState(childID, appID, start, end, access, state); err != nil {
		t.Fatalf("retry the same booking: %v", err)
	}

	charges, err = store.BudgetTotals(childID, appID, start)
	if err != nil {
		t.Fatalf("read retried budget totals: %v", err)
	}
	want := usage.BudgetUsage{
		SharedRegular: 15 * time.Minute,
		AppRegular:    15 * time.Minute,
	}
	if charges != want {
		t.Errorf("charges after retry = %+v, want %+v", charges, want)
	}
	saved, err = store.LoadBreakState(childID)
	if err != nil || saved.Used != state.Used || !saved.StartedAt.Equal(state.StartedAt) {
		t.Errorf("state after retry = (%+v, %v), want %+v", saved, err, state)
	}
}
