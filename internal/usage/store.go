package usage

import (
	"time"

	"github.com/ebe542/go-screen-time/internal/policy"
)

// VerifiedBonusGrant contains the data from an authenticated bonus message.
// The caller must verify the signer, recipient, and expiry before redemption.
type VerifiedBonusGrant struct {
	ID       string
	ChildID  string
	AppID    string
	Duration time.Duration
}

// Store provides durable usage accounting and bonus management.
type Store interface {
	GrantBonus(childID, appID string, at time.Time, duration time.Duration) error
	IssueBonusCode(childID, appID string, issuedAt, expiresAt time.Time, duration time.Duration) (string, error)
	// RedeemBonusCode must consume the code and create its grant atomically.
	RedeemBonusCode(childID, code string, at time.Time) error
	// RedeemVerifiedBonus records the grant ID and credits the time atomically.
	RedeemVerifiedBonus(grant VerifiedBonusGrant, at time.Time) error
	BonusesFor(childID, appID string, at time.Time) (AppBonuses, error)
	BudgetTotals(childID, appID string, at time.Time) (BudgetUsage, error)
	RecordAppUsageWithGrants(childID, appID string, start, end time.Time, access policy.AppAccessPolicy) error
	AllowsAppWithGrants(childID, appID string, at time.Time, access policy.AppAccessPolicy, breakStartedAt time.Time) (bool, error)
	// RedeemBonusMessageAttempt applies the child's attempt limit and credits
	// a verified grant atomically. A nil grant records a failed verification.
	RedeemBonusMessageAttempt(childID string, at time.Time, grant *VerifiedBonusGrant) error
}
