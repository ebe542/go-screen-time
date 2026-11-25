package bonusmessage

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ProtonMail/gopenpgp/v3/crypto"
)

func TestIssueBonusMessageFromBothParents(t *testing.T) {
	fixture := newRedemptionFixture(t)
	secondParent, secondParentPublic := testKeyPair(t, fixture.pgp, "second-parent")
	trusted := []*crypto.Key{fixture.parentPub, secondParentPublic}

	request := BonusRequest{
		PCID:      "pc-a",
		ChildID:   "child-a",
		AppID:     "app-a",
		Minutes:   17,
		ExpiresAt: fixture.at.Add(time.Hour),
	}

	store := openRedemptionStore(t, filepath.Join(t.TempDir(), "usage.db"))
	defer store.Close()

	ids := make(map[string]bool)
	for _, signer := range []*crypto.Key{fixture.parent, secondParent} {
		armored, err := IssueBonusMessage(fixture.pcPublic, signer, request, fixture.at)
		if err != nil {
			t.Fatalf("issue message: %v", err)
		}
		if !strings.HasPrefix(armored, "-----BEGIN PGP MESSAGE-----") {
			t.Fatal("issued message is not an armored OpenPGP message")
		}

		grant, err := VerifyBonusMessage(
			armored, fixture.pcPrivate, trusted, request.PCID, request.ChildID, fixture.at,
		)
		if err != nil {
			t.Fatalf("verify issued message: %v", err)
		}
		if grant.ID == "" || ids[grant.ID] {
			t.Fatalf("expected a new grant ID, got %q", grant.ID)
		}
		ids[grant.ID] = true

		if grant.ChildID != request.ChildID ||
			grant.AppID != request.AppID ||
			grant.Duration != 17*time.Minute {
			t.Errorf("verified grant does not match the request: %+v", grant)
		}
		if err := RedeemBonusMessage(
			store, armored, fixture.pcPrivate, trusted, request.PCID, request.ChildID, fixture.at,
		); err != nil {
			t.Fatalf("redeem issued message: %v", err)
		}
	}

	bonuses, err := store.BonusesFor(request.ChildID, request.AppID, fixture.at)
	if err != nil {
		t.Fatalf("read credited bonuses: %v", err)
	}
	if bonuses.App.Granted != 34*time.Minute || bonuses.Shared.Granted != 0 {
		t.Errorf("credited bonuses = %+v, want 34m for app A only", bonuses)
	}
}

func TestIssueBonusMessageCreatesNewIDForRepeatedRequest(t *testing.T) {
	fixture := newRedemptionFixture(t)
	request := BonusRequest{
		PCID:      "pc-a",
		ChildID:   "child-a",
		Minutes:   5,
		ExpiresAt: fixture.at.Add(time.Hour),
	}
	trusted := []*crypto.Key{fixture.parentPub}

	var previousID string
	for range 2 {
		armored, err := IssueBonusMessage(fixture.pcPublic, fixture.parent, request, fixture.at)
		if err != nil {
			t.Fatalf("issue message: %v", err)
		}
		grant, err := VerifyBonusMessage(
			armored, fixture.pcPrivate, trusted, request.PCID, request.ChildID, fixture.at,
		)
		if err != nil {
			t.Fatalf("verify issued message: %v", err)
		}
		if grant.ID == "" || grant.ID == previousID {
			t.Fatalf("expected a new grant ID, got %q", grant.ID)
		}
		if grant.AppID != "" || grant.Duration != 5*time.Minute {
			t.Errorf("verified grant = %+v, want 5m shared bonus", grant)
		}
		previousID = grant.ID
	}
}

func TestIssueBonusMessageRejectsInvalidTerms(t *testing.T) {
	fixture := newRedemptionFixture(t)
	valid := BonusRequest{
		PCID:      "pc-a",
		ChildID:   "child-a",
		Minutes:   5,
		ExpiresAt: fixture.at.Add(time.Hour),
	}

	tests := []struct {
		name   string
		change func(*BonusRequest)
	}{
		{"missing target PC", func(r *BonusRequest) { r.PCID = "" }},
		{"missing child", func(r *BonusRequest) { r.ChildID = "" }},
		{"zero minutes", func(r *BonusRequest) { r.Minutes = 0 }},
		{"negative minutes", func(r *BonusRequest) { r.Minutes = -1 }},
		{"duration overflow", func(r *BonusRequest) { r.Minutes = 1 << 62 }},
		{"missing deadline", func(r *BonusRequest) { r.ExpiresAt = time.Time{} }},
		{"deadline at issuance", func(r *BonusRequest) { r.ExpiresAt = fixture.at }},
		{"deadline before issuance", func(r *BonusRequest) {
			r.ExpiresAt = fixture.at.Add(-time.Second)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := valid
			tt.change(&request)

			armored, err := IssueBonusMessage(fixture.pcPublic, fixture.parent, request, fixture.at)
			if err == nil || armored != "" {
				t.Error("invalid request should return an error and no message")
			}
		})
	}

	armored, err := IssueBonusMessage(nil, fixture.parent, valid, fixture.at)
	if err == nil || armored != "" {
		t.Error("missing recipient key should return an error and no message")
	}
	armored, err = IssueBonusMessage(fixture.pcPublic, nil, valid, fixture.at)
	if err == nil || armored != "" {
		t.Error("missing signing key should return an error and no message")
	}
}
