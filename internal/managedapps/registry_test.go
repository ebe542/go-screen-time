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
func TestRegistryUsesChildBreakRuleForEachTimedApp(t *testing.T) {
	rule := policy.BreakRule{After: 45 * time.Minute, Duration: 10 * time.Minute}
	firstOldRule := policy.BreakRule{After: 30 * time.Minute, Duration: 5 * time.Minute}
	secondOldRule := policy.BreakRule{After: time.Hour, Duration: 15 * time.Minute}

	firstPolicy := &policy.AppAccessPolicy{BreakRule: firstOldRule}
	secondPolicy := &policy.AppAccessPolicy{BreakRule: secondOldRule}
	const writerPath = `C:\School\writer.exe`

	registry, err := NewRegistry([]ChildProfile{{
		SID:       childASID,
		BreakRule: rule,
		Apps: []App{
			{ID: "game", ExecutablePaths: []string{gamePath}, Policy: firstPolicy},
			{ID: "launcher", ExecutablePaths: []string{launcherPath}, Policy: secondPolicy},
			{ID: "writer", ExecutablePaths: []string{writerPath}},
		},
	}})
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	for _, executable := range []string{gamePath, launcherPath} {
		got, found := registry.Match(childASID, executable)
		if !found || got.Policy == nil {
			t.Fatalf("timed app %q did not match", executable)
		}
		if got.Policy.BreakRule != rule {
			t.Errorf("break rule for %q = %+v, want %+v", executable, got.Policy.BreakRule, rule)
		}
	}

	writer, found := registry.Match(childASID, writerPath)
	if !found || writer.Policy != nil {
		t.Errorf("writer = (%+v, %t), want an unrestricted entry", writer, found)
	}
	if firstPolicy.BreakRule != firstOldRule || secondPolicy.BreakRule != secondOldRule {
		t.Error("building the registry modified the supplied app policies")
	}
}
