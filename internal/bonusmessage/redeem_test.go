package bonusmessage

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/ProtonMail/gopenpgp/v3/crypto"
	"github.com/ProtonMail/gopenpgp/v3/profile"

	"github.com/ebe542/go-screen-time/internal/storage/sqlite"
)

func TestRedeemBonusMessageCreditsOnceAfterReencryptionAndRestart(t *testing.T) {
	fixture := newRedemptionFixture(t)
	path := filepath.Join(t.TempDir(), "usage.db")
	first := fixture.message(t, "child-a", "app-a", fixture.at.Add(time.Hour))
	second := fixture.message(t, "child-a", "app-a", fixture.at.Add(time.Hour))
	if first == second {
		t.Fatal("re-encrypting the grant produced identical test messages")
	}

	store := openRedemptionStore(t, path)
	if err := fixture.redeem(store, "child-a", first, fixture.at); err != nil {
		_ = store.Close()
		t.Fatalf("redeem first message: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close before restart: %v", err)
	}

	store = openRedemptionStore(t, path)
	defer store.Close()

	if err := fixture.redeem(store, "child-a", second, fixture.at.Add(time.Second)); err == nil {
		t.Error("re-encrypted copy was accepted after restart")
	}

	bonuses, err := store.BonusesFor("child-a", "app-a", fixture.at.Add(time.Second))
	if err != nil {
		t.Fatalf("read credited bonus: %v", err)
	}
	if bonuses.App.Granted != 17*time.Minute || bonuses.Shared.Granted != 0 {
		t.Errorf("credited bonuses = %+v, want 17m for app A only", bonuses)
	}
}

func TestRedeemBonusMessageRejectsInvalidMessagesWithoutCredit(t *testing.T) {
	fixture := newRedemptionFixture(t)
	validExpiry := fixture.at.Add(time.Hour)
	valid := fixture.message(t, "child-a", "", validExpiry)

	tests := []struct {
		name    string
		message string
	}{
		{"tampered", tamperArmoredMessage(t, valid)},
		{"expired", fixture.message(t, "child-a", "", fixture.at)},
		{"different child", fixture.message(t, "child-b", "", validExpiry)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := openRedemptionStore(t, filepath.Join(t.TempDir(), "usage.db"))
			defer store.Close()

			if err := fixture.redeem(store, "child-a", tt.message, fixture.at); err == nil {
				t.Error("invalid message was accepted")
			}
			assertSharedRedemptionBonus(t, store, "child-a", fixture.at, 0)
			assertSharedRedemptionBonus(t, store, "child-b", fixture.at, 0)
		})
	}
}

func TestRedeemBonusMessageLimitsFailedAttemptsPerChildAcrossRestart(t *testing.T) {
	fixture := newRedemptionFixture(t)
	path := filepath.Join(t.TempDir(), "usage.db")
	expiry := fixture.at.Add(time.Hour)
	messageA := fixture.message(t, "child-a", "", expiry)
	messageB := fixture.message(t, "child-b", "", expiry)

	store := openRedemptionStore(t, path)
	for range 5 {
		if err := fixture.redeem(store, "child-a", "invalid message", fixture.at); err == nil {
			_ = store.Close()
			t.Fatal("invalid message was accepted")
		}
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close before restart: %v", err)
	}

	store = openRedemptionStore(t, path)
	defer store.Close()

	duringBlock := fixture.at.Add(30 * time.Second)
	if err := fixture.redeem(store, "child-a", messageA, duringBlock); err == nil {
		t.Error("child A redeemed a message during the blocked minute")
	}
	assertSharedRedemptionBonus(t, store, "child-a", duringBlock, 0)

	if err := fixture.redeem(store, "child-b", messageB, duringBlock); err != nil {
		t.Fatalf("child B was blocked by child A's failed attempts: %v", err)
	}
	assertSharedRedemptionBonus(t, store, "child-b", duringBlock, 17*time.Minute)

	afterBlock := fixture.at.Add(time.Minute)
	if err := fixture.redeem(store, "child-a", messageA, afterBlock); err != nil {
		t.Fatalf("child A could not redeem after the block expired: %v", err)
	}
	assertSharedRedemptionBonus(t, store, "child-a", afterBlock, 17*time.Minute)
}

type redemptionFixture struct {
	pgp       *crypto.PGPHandle
	pcPrivate *crypto.Key
	pcPublic  *crypto.Key
	parent    *crypto.Key
	parentPub *crypto.Key
	at        time.Time
}

func newRedemptionFixture(t *testing.T) redemptionFixture {
	t.Helper()

	pgp := crypto.PGPWithProfile(profile.RFC9580())
	pcPrivate, pcPublic := testKeyPair(t, pgp, "pc")
	parent, parentPub := testKeyPair(t, pgp, "parent")
	return redemptionFixture{
		pgp:       pgp,
		pcPrivate: pcPrivate,
		pcPublic:  pcPublic,
		parent:    parent,
		parentPub: parentPub,
		at:        time.Now().UTC().Truncate(time.Second).Add(time.Minute),
	}
}

func (f redemptionFixture) message(t *testing.T, childID, appID string, expiry time.Time) string {
	t.Helper()

	grantID := "2eaa766d-c0c6-42f1-9e6e-0da9de609f48"
	if childID == "child-b" {
		grantID = "ce63fdcb-684e-481f-81d9-bbc64a7f41e6"
	}
	data, err := json.Marshal(map[string]any{
		"format":     "go-screen-time/bonus/v1",
		"grant_id":   grantID,
		"pc_id":      "pc-a",
		"child_id":   childID,
		"app_id":     appID,
		"minutes":    17,
		"expires_at": expiry,
	})
	if err != nil {
		t.Fatalf("encode test grant: %v", err)
	}
	return testArmoredMessage(t, f.pgp, f.pcPublic, f.parent, data)
}

func (f redemptionFixture) redeem(store *sqlite.Ledger, childID, armored string, at time.Time) error {
	return RedeemBonusMessage(
		store, armored, f.pcPrivate, []*crypto.Key{f.parentPub}, "pc-a", childID, at,
	)
}

func openRedemptionStore(t *testing.T, path string) *sqlite.Ledger {
	t.Helper()

	store, err := sqlite.Open(path, time.UTC)
	if err != nil {
		t.Fatalf("open SQLite ledger: %v", err)
	}
	return store
}

func assertSharedRedemptionBonus(
	t *testing.T,
	store *sqlite.Ledger,
	childID string,
	at time.Time,
	want time.Duration,
) {
	t.Helper()

	bonuses, err := store.BonusesFor(childID, "app-a", at)
	if err != nil {
		t.Fatalf("read bonus for %s: %v", childID, err)
	}
	if bonuses.Shared.Granted != want {
		t.Errorf("%s shared bonus = %s, want %s", childID, bonuses.Shared.Granted, want)
	}
}
