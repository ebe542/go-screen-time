package managedapps

import (
	"testing"
	"time"

	"github.com/ebe542/go-screen-time/internal/policy"
)

const (
	childASID  = "S-1-5-21-100-200-300-1001"
	childBSID  = "S-1-5-21-100-200-300-1002"
	unknownSID = "S-1-5-21-100-200-300-1003"

	gamePath     = `C:\Games\Example\game.exe`
	launcherPath = `C:\Games\Example\launcher.exe`
)

func TestRegistryMatchesAppsWithinChildProfile(t *testing.T) {
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
		{
			SID: childBSID,
			Apps: []App{
				{
					ID:              "example-game",
					ExecutablePaths: []string{gamePath},
					Policy: &policy.AppAccessPolicy{
						AppLimit: &policy.AppDailyLimit{Limit: time.Hour},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	tests := []struct {
		name  string
		sid   string
		path  string
		limit time.Duration
	}{
		{"game for first child", childASID, gamePath, 30 * time.Minute},
		{"same executable for second child", childBSID, gamePath, time.Hour},
		{"launcher shares the game rule", childASID, launcherPath, 30 * time.Minute},
		{"same path with different case", childASID, `c:\games\EXAMPLE\GAME.EXE`, 30 * time.Minute},
		{"launcher belongs only to first profile", childBSID, launcherPath, 0},
		{"same filename in a different directory", childASID, `C:\Other\game.exe`, 0},
		{"account without a child profile", unknownSID, gamePath, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := registry.Match(tt.sid, tt.path)
			if tt.limit == 0 {
				if found {
					t.Errorf("unexpected match: %+v", got)
				}
				return
			}
			if !found {
				t.Fatal("expected a matching app")
			}
			if got.ChildID != tt.sid || got.AppID != "example-game" {
				t.Errorf("matched child and app = (%q, %q)", got.ChildID, got.AppID)
			}
			if got.Policy == nil || got.Policy.AppLimit == nil || got.Policy.AppLimit.Limit != tt.limit {
				t.Errorf("matched policy = %+v, want app limit %s", got.Policy, tt.limit)
			}
		})
	}
}

func TestNewRegistryRejectsAmbiguousExecutablePaths(t *testing.T) {
	_, err := NewRegistry([]ChildProfile{
		{
			SID: childASID,
			Apps: []App{
				{ID: "app-a", ExecutablePaths: []string{gamePath}},
				{ID: "app-b", ExecutablePaths: []string{`c:\games\example\GAME.EXE`}},
			},
		},
	})
	if err == nil {
		t.Fatal("expected an error for a path assigned to two apps in one profile")
	}
}

func TestNewRegistryRejectsNonAbsoluteExecutablePaths(t *testing.T) {
	for _, path := range []string{`game.exe`, `Games\game.exe`, `C:game.exe`} {
		t.Run(path, func(t *testing.T) {
			_, err := NewRegistry([]ChildProfile{
				{
					SID: childASID,
					Apps: []App{
						{ID: "app-a", ExecutablePaths: []string{path}},
					},
				},
			})
			if err == nil {
				t.Errorf("expected an error for non-absolute path %q", path)
			}
		})
	}
}
