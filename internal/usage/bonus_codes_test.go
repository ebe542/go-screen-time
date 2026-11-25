package usage

import (
	"testing"
	"time"
)

func TestBonusCodeRedeemsIntoTheIntendedGrant(t *testing.T) {
	issuedAt := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	expiresAt := issuedAt.Add(time.Hour)
	redeemedAt := issuedAt.Add(time.Minute)
	const duration = 5 * time.Minute

	tests := []struct {
		name       string
		scope      string
		wantShared time.Duration
		wantApp    time.Duration
	}{
		{"shared bonus", "", duration, 0},
		{"app bonus", "app-a", 0, duration},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ledger := NewLedger(time.UTC)
			code := mustIssueBonusCode(t, ledger, tt.scope, issuedAt, expiresAt, duration)

			if err := ledger.RedeemBonusCode("child-a", code, redeemedAt); err != nil {
				t.Fatalf("redeem code: %v", err)
			}

			got := ledger.BonusesFor("child-a", "app-a", redeemedAt)
			if got.Shared.Granted != tt.wantShared || got.App.Granted != tt.wantApp {
				t.Errorf("granted = (shared %s, app %s), want (%s, %s)",
					got.Shared.Granted, got.App.Granted, tt.wantShared, tt.wantApp)
			}

			before := ledger.BonusesFor("child-a", "app-a", redeemedAt.Add(-time.Second))
			if before.Shared.Granted != 0 || before.App.Granted != 0 {
				t.Errorf("grant was effective before redemption: %+v", before)
			}

			otherApp := ledger.BonusesFor("child-a", "app-b", redeemedAt)
			if otherApp.App.Granted != 0 {
				t.Errorf("other app received %s of app bonus", otherApp.App.Granted)
			}

			if err := ledger.RedeemBonusCode("child-a", code, redeemedAt.Add(time.Second)); err == nil {
				t.Error("redeeming the same code twice succeeded")
			}
		})
	}
}

func TestBonusCodeRejectsInvalidRedemption(t *testing.T) {
	issuedAt := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	expiresAt := issuedAt.Add(time.Hour)

	tests := []struct {
		name    string
		childID string
		at      time.Time
		unknown bool
	}{
		{"unknown code", "child-a", issuedAt, true},
		{"wrong child", "child-b", issuedAt, false},
		{"before issuance", "child-a", issuedAt.Add(-time.Second), false},
		{"at expiry", "child-a", expiresAt, false},
		{"after expiry", "child-a", expiresAt.Add(time.Second), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ledger := NewLedger(time.UTC)
			code := mustIssueBonusCode(t, ledger, "", issuedAt, expiresAt, 5*time.Minute)
			if tt.unknown {
				code += "invalid"
			}

			if err := ledger.RedeemBonusCode(tt.childID, code, tt.at); err == nil {
				t.Fatal("invalid redemption succeeded")
			}

			for _, childID := range []string{"child-a", "child-b"} {
				if got := ledger.BonusesFor(childID, "app-a", expiresAt).Shared.Granted; got != 0 {
					t.Errorf("%s received %s from an invalid redemption", childID, got)
				}
			}

			if tt.childID == "child-b" {
				if err := ledger.RedeemBonusCode("child-a", code, issuedAt.Add(time.Minute)); err != nil {
					t.Errorf("wrong-account attempt consumed the code: %v", err)
				}
			}
		})
	}
}

func TestIssueBonusCodeRejectsInvalidTerms(t *testing.T) {
	local := time.FixedZone("UTC+1", 60*60)
	midnight := time.Date(2026, time.November, 28, 0, 0, 0, 0, local)
	issuedAt := midnight.Add(-time.Hour).UTC()

	tests := []struct {
		name      string
		childID   string
		expiresAt time.Time
		duration  time.Duration
	}{
		{"missing child", "", midnight, time.Minute},
		{"zero duration", "child-a", midnight, 0},
		{"negative duration", "child-a", midnight, -time.Minute},
		{"expiry at issuance", "child-a", issuedAt, time.Minute},
		{"expiry after local midnight", "child-a", midnight.Add(time.Second), time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ledger := NewLedger(local)
			if _, err := ledger.IssueBonusCode(
				tt.childID, "", issuedAt, tt.expiresAt, tt.duration,
			); err == nil {
				t.Error("issuing a code with invalid terms succeeded")
			}
		})
	}

	ledger := NewLedger(local)
	mustIssueBonusCode(t, ledger, "", issuedAt, midnight, time.Minute)
}

func TestBonusCodeCanBeRedeemedOnlyOnceConcurrently(t *testing.T) {
	ledger := NewLedger(time.UTC)
	issuedAt := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	redeemedAt := issuedAt.Add(time.Minute)
	code := mustIssueBonusCode(t, ledger, "", issuedAt, issuedAt.Add(time.Hour), 5*time.Minute)

	const attempts = 8
	start := make(chan struct{})
	results := make(chan error, attempts)

	for range attempts {
		go func() {
			<-start
			results <- ledger.RedeemBonusCode("child-a", code, redeemedAt)
		}()
	}
	close(start)

	successes := 0
	for range attempts {
		if err := <-results; err == nil {
			successes++
		}
	}

	if successes != 1 {
		t.Errorf("successful redemptions = %d, want 1", successes)
	}
	if got := ledger.BonusesFor("child-a", "app-a", redeemedAt).Shared.Granted; got != 5*time.Minute {
		t.Errorf("shared bonus = %s, want 5m", got)
	}
}

func mustIssueBonusCode(
	t *testing.T,
	ledger *Ledger,
	appID string,
	issuedAt, expiresAt time.Time,
	duration time.Duration,
) string {
	t.Helper()

	code, err := ledger.IssueBonusCode("child-a", appID, issuedAt, expiresAt, duration)
	if err != nil {
		t.Fatalf("issue code: %v", err)
	}
	if code == "" {
		t.Fatal("issued code is empty")
	}
	return code
}
