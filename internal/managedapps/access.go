package managedapps

import (
	"time"

	"github.com/ebe542/go-screen-time/internal/policy"
)

// AccessDecision describes the policy result for an observed executable.
type AccessDecision string

const (
	AccessAllowed        AccessDecision = "allowed"
	AccessBlocked        AccessDecision = "blocked"
	AccessNotAllowlisted AccessDecision = "not_allowlisted"
	AccessUnmanaged      AccessDecision = "unmanaged"
)

// AccessStore evaluates a managed app against its usage and bonus state.
type AccessStore interface {
	AllowsAppWithGrants(
		childID, appID string,
		at time.Time,
		access policy.AppAccessPolicy,
		breakStartedAt time.Time,
	) (bool, error)
}

// EvaluateAccess applies the child's allowlist and any assigned time policy.
// A storage error is returned with the zero value of AccessDecision.
func (r *Registry) EvaluateAccess(
	store AccessStore,
	sid, executable string,
	at, breakStartedAt time.Time,
) (AccessDecision, error) {
	if _, hasProfile := r.bySID[sid]; !hasProfile {
		return AccessUnmanaged, nil
	}

	assignment, found := r.Match(sid, executable)
	if !found {
		return AccessNotAllowlisted, nil
	}
	if assignment.Policy == nil {
		return AccessAllowed, nil
	}

	allowed, err := store.AllowsAppWithGrants(
		assignment.ChildID,
		assignment.AppID,
		at,
		*assignment.Policy,
		breakStartedAt,
	)
	if err != nil {
		return "", err
	}
	if allowed {
		return AccessAllowed, nil
	}
	return AccessBlocked, nil
}
