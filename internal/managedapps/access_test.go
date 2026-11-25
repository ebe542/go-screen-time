package managedapps

import (
	"errors"
	"testing"
	"time"

	"github.com/ebe542/go-screen-time/internal/policy"
)

func TestRegistryEvaluateAccess(t *testing.T) {
	registry, err := NewRegistry([]ChildProfile{
		{
			SID: childASID,
			Apps: []App{
				{
					ID:              "example-game",
					ExecutablePaths: []string{gamePath, launcherPath},
					Policy: &policy.AppAccessPolicy{
						AppLimit: &policy.AppDailyLimit{Limit: 30 * time.Minute},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	at := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	breakStartedAt := at.Add(-time.Minute)
	storageErr := errors.New("usage store unavailable")

	tests := []struct {
		name      string
		sid       string
		path      string
		allowed   bool
		storeErr  error
		want      AccessDecision
		wantCalls int
	}{
		{
			name: "managed launcher allowed", sid: childASID, path: launcherPath,
			allowed: true, want: AccessAllowed, wantCalls: 1,
		},
		{
			name: "managed game blocked", sid: childASID, path: gamePath,
			want: AccessBlocked, wantCalls: 1,
		},
		{
			name: "unlisted executable", sid: childASID, path: `C:\Other\game.exe`,
			want: AccessNotAllowlisted,
		},
		{
			name: "account without a profile", sid: unknownSID, path: gamePath,
			want: AccessUnmanaged,
		},
		{
			name: "storage failure", sid: childASID, path: gamePath,
			storeErr: storageErr, wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			store := accessStoreFunc(func(
				childID, appID string,
				gotAt time.Time,
				access policy.AppAccessPolicy,
				gotBreakStartedAt time.Time,
			) (bool, error) {
				calls++

				if childID != childASID || appID != "example-game" {
					t.Errorf("store received identifiers (%q, %q)", childID, appID)
				}
				if !gotAt.Equal(at) || !gotBreakStartedAt.Equal(breakStartedAt) {
					t.Errorf("store received times (%v, %v)", gotAt, gotBreakStartedAt)
				}
				if access.AppLimit == nil || access.AppLimit.Limit != 30*time.Minute {
					t.Errorf("store received unexpected app limit: %+v", access.AppLimit)
				}
				return tt.allowed, tt.storeErr
			})

			got, err := registry.EvaluateAccess(store, tt.sid, tt.path, at, breakStartedAt)
			if !errors.Is(err, tt.storeErr) {
				t.Errorf("EvaluateAccess error = %v, want %v", err, tt.storeErr)
			}
			if got != tt.want {
				t.Errorf("EvaluateAccess decision = %q, want %q", got, tt.want)
			}
			if calls != tt.wantCalls {
				t.Errorf("store calls = %d, want %d", calls, tt.wantCalls)
			}
		})
	}
}

func TestRegistryEvaluateAccessDistinguishesAllowlistEntries(t *testing.T) {
	const schoolPath = `C:\School\writer.exe`

	registry, err := NewRegistry([]ChildProfile{
		{
			SID: childASID,
			Apps: []App{
				{ID: "writer", ExecutablePaths: []string{schoolPath}},
				{
					ID:              "example-game",
					ExecutablePaths: []string{gamePath},
					Policy: &policy.AppAccessPolicy{
						AppLimit: &policy.AppDailyLimit{Limit: 0},
					},
				},
			},
		},
		{SID: childBSID},
	})
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	at := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		sid       string
		path      string
		want      AccessDecision
		wantCalls int
	}{
		{"unrestricted entry", childASID, schoolPath, AccessAllowed, 0},
		{"explicit zero limit", childASID, gamePath, AccessBlocked, 1},
		{"unlisted executable", childASID, `C:\Other\game.exe`, AccessNotAllowlisted, 0},
		{"empty child allowlist", childBSID, schoolPath, AccessNotAllowlisted, 0},
		{"account without child profile", unknownSID, schoolPath, AccessUnmanaged, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			store := accessStoreFunc(func(
				childID, appID string,
				at time.Time,
				access policy.AppAccessPolicy,
				breakStartedAt time.Time,
			) (bool, error) {
				calls++
				if childID != childASID || appID != "example-game" {
					t.Errorf("unexpected store lookup for child %q and app %q", childID, appID)
				}
				if access.AppLimit == nil || access.AppLimit.Limit != 0 {
					t.Fatal("expected an explicit zero app limit")
				}
				return access.AppLimit.Allows(0), nil
			})

			got, err := registry.EvaluateAccess(store, tt.sid, tt.path, at, time.Time{})
			if err != nil {
				t.Fatalf("EvaluateAccess: %v", err)
			}
			if got != tt.want {
				t.Errorf("decision = %q, want %q", got, tt.want)
			}
			if calls != tt.wantCalls {
				t.Errorf("store calls = %d, want %d", calls, tt.wantCalls)
			}
		})
	}
}

type accessStoreFunc func(
	childID, appID string,
	at time.Time,
	access policy.AppAccessPolicy,
	breakStartedAt time.Time,
) (bool, error)

func (f accessStoreFunc) AllowsAppWithGrants(
	childID, appID string,
	at time.Time,
	access policy.AppAccessPolicy,
	breakStartedAt time.Time,
) (bool, error) {
	return f(childID, appID, at, access, breakStartedAt)
}
