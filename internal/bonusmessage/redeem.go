package bonusmessage

import (
	"time"

	"github.com/ProtonMail/gopenpgp/v3/crypto"

	"github.com/ebe542/go-screen-time/internal/usage"
)

type redemptionStore interface {
	RedeemBonusMessageAttempt(childID string, at time.Time, grant *usage.VerifiedBonusGrant) error
}

// RedeemBonusMessage verifies the pasted message and submits the result
// to the store for rate limiting and one-time redemption.
func RedeemBonusMessage(
	store redemptionStore,
	armored string,
	decryptionKey *crypto.Key,
	trustedParents []*crypto.Key,
	pcID, childID string,
	at time.Time,
) error {
	grant, err := VerifyBonusMessage(
		armored, decryptionKey, trustedParents, pcID, childID, at,
	)
	if err != nil {
		return store.RedeemBonusMessageAttempt(childID, at, nil)
	}
	return store.RedeemBonusMessageAttempt(childID, at, &grant)
}
