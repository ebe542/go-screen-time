package observationservice

import (
	"strings"
	"testing"
	"time"

	"github.com/ebe542/go-screen-time/internal/policy"
)

const (
	configTestSID    = "S-1-5-21-100-200-300-1001"
	configTestGame   = `C:\Games\Example\game.exe`
	configTestSecond = `C:\Games\Second\game.exe`
	configTestWriter = `C:\School\writer.exe`
)

const validProfileJSON = `{
  "children": [
    {
      "sid": "S-1-5-21-100-200-300-1001",
      "daily_budget": {"weekday_minutes": 120, "weekend_minutes": 180},
      "break": {"after_minutes": 45, "duration_minutes": 10},
      "apps": [
        {
          "id": "example-game",
          "executable_paths": ["C:\\Games\\Example\\game.exe"],
          "policy": {
            "weekday_windows": [{"start": "14:00", "end": "19:00"}],
            "weekend_windows": [{"start": "10:00", "end": "19:00"}]
          }
        },
        {
          "id": "second-game",
          "executable_paths": ["C:\\Games\\Second\\game.exe"],
          "policy": {"app_limit_minutes": 0}
        },
        {
          "id": "writer",
          "executable_paths": ["C:\\School\\writer.exe"]
        }
      ]
    }
  ]
}`

func TestLoadRegistryAppliesChildAndAppRules(t *testing.T) {
	registry, err := LoadRegistry(strings.NewReader(validProfileJSON))
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}

	wantBudget := policy.DailyBudget{
		WeekdayLimit: 2 * time.Hour,
		WeekendLimit: 3 * time.Hour,
	}
	wantBreak := policy.BreakRule{
		After:    45 * time.Minute,
		Duration: 10 * time.Minute,
	}

	game, ok := registry.Match(configTestSID, configTestGame)
	if !ok || game.Policy == nil {
		t.Fatal("time-limited game not found")
	}
	if game.Policy.AppLimit != nil {
		t.Error("omitted app limit did not remain optional")
	}

	second, ok := registry.Match(configTestSID, configTestSecond)
	if !ok || second.Policy == nil {
		t.Fatal("second time-limited game not found")
	}
	if second.Policy.AppLimit == nil || second.Policy.AppLimit.Limit != 0 {
		t.Error("explicit zero app limit was lost")
	}

	for _, access := range []*policy.AppAccessPolicy{game.Policy, second.Policy} {
		if access.SharedBudget != wantBudget || access.BreakRule != wantBreak {
			t.Errorf("timed policy has budget %+v and break %+v", access.SharedBudget, access.BreakRule)
		}
	}
	if got := registry.BreakRules()[configTestSID]; got != wantBreak {
		t.Errorf("child break rule = %+v, want %+v", got, wantBreak)
	}

	writer, ok := registry.Match(configTestSID, configTestWriter)
	if !ok || writer.Policy != nil {
		t.Error("writer should have an unrestricted allowlist entry")
	}

	fridayNoon := time.Date(2026, time.November, 27, 12, 0, 0, 0, time.UTC)
	if game.Policy.Schedule.Allows(fridayNoon) {
		t.Error("game was allowed before its weekday window")
	}
	if !game.Policy.Schedule.Allows(fridayNoon.Add(2 * time.Hour)) {
		t.Error("game was blocked at the start of its weekday window")
	}
	if !game.Policy.Schedule.Allows(fridayNoon.AddDate(0, 0, 1)) {
		t.Error("game was blocked during its weekend window")
	}
}

func TestLoadRegistryRejectsInvalidProfiles(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			"invalid clock time",
			strings.Replace(validProfileJSON, `"14:00"`, `"14:60"`, 1),
		},
		{
			"negative budget",
			strings.Replace(validProfileJSON, `"weekday_minutes": 120`, `"weekday_minutes": -1`, 1),
		},
		{
			"negative app limit",
			strings.Replace(validProfileJSON, `"app_limit_minutes": 0`, `"app_limit_minutes": -1`, 1),
		},
		{
			"time-limited app without shared budget",
			strings.Replace(validProfileJSON,
				`"daily_budget": {"weekday_minutes": 120, "weekend_minutes": 180},`, "", 1),
		},
		{
			"invalid break duration",
			strings.Replace(validProfileJSON, `"duration_minutes": 10`, `"duration_minutes": 0`, 1),
		},
		{
			"path assigned to multiple apps",
			strings.Replace(validProfileJSON,
				`C:\\Games\\Second\\game.exe`, `C:\\Games\\Example\\game.exe`, 1),
		},
		{
			"unknown field",
			strings.Replace(validProfileJSON, `"weekday_minutes"`, `"workday_minutes"`, 1),
		},
		{"second JSON document", validProfileJSON + `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry, err := LoadRegistry(strings.NewReader(tt.input))
			if err == nil || registry != nil {
				t.Errorf("LoadRegistry = (%v, %v), want error and no registry", registry, err)
			}
		})
	}
}
