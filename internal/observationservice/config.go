package observationservice

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ebe542/go-screen-time/internal/managedapps"
	"github.com/ebe542/go-screen-time/internal/policy"
)

type profileConfig struct {
	Children []childConfig `json:"children"`
}

type childConfig struct {
	SID    string        `json:"sid"`
	Budget *budgetConfig `json:"daily_budget"`
	Break  *breakConfig  `json:"break"`
	Apps   []appConfig   `json:"apps"`
}

type budgetConfig struct {
	WeekdayMinutes *int64 `json:"weekday_minutes"`
	WeekendMinutes *int64 `json:"weekend_minutes"`
}

type breakConfig struct {
	AfterMinutes    int64 `json:"after_minutes"`
	DurationMinutes int64 `json:"duration_minutes"`
}

type appConfig struct {
	ID              string           `json:"id"`
	ExecutablePaths []string         `json:"executable_paths"`
	Policy          *appPolicyConfig `json:"policy"`
}

type appPolicyConfig struct {
	WeekdayWindows  []windowConfig `json:"weekday_windows"`
	WeekendWindows  []windowConfig `json:"weekend_windows"`
	AppLimitMinutes *int64         `json:"app_limit_minutes"`
}

type windowConfig struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// LoadRegistry reads one JSON configuration and builds the child allowlists.
func LoadRegistry(reader io.Reader) (*managedapps.Registry, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()

	var config profileConfig
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("decode child profiles: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("expected a single JSON configuration document")
	}
	if config.Children == nil {
		return nil, errors.New("configuration requires a children array")
	}

	profiles := make([]managedapps.ChildProfile, 0, len(config.Children))
	for _, child := range config.Children {
		profile, err := child.buildProfile()
		if err != nil {
			return nil, fmt.Errorf("configure child %q: %w", child.SID, err)
		}
		profiles = append(profiles, profile)
	}

	return managedapps.NewRegistry(profiles)
}

func (c childConfig) buildProfile() (managedapps.ChildProfile, error) {
	profile := managedapps.ChildProfile{SID: c.SID}

	var budget policy.DailyBudget
	if c.Budget != nil {
		if c.Budget.WeekdayMinutes == nil || c.Budget.WeekendMinutes == nil {
			return managedapps.ChildProfile{}, errors.New("daily_budget requires weekday_minutes and weekend_minutes")
		}

		weekday, err := minutesDuration(*c.Budget.WeekdayMinutes)
		if err != nil {
			return managedapps.ChildProfile{}, fmt.Errorf("weekday budget: %w", err)
		}
		weekend, err := minutesDuration(*c.Budget.WeekendMinutes)
		if err != nil {
			return managedapps.ChildProfile{}, fmt.Errorf("weekend budget: %w", err)
		}
		budget = policy.DailyBudget{WeekdayLimit: weekday, WeekendLimit: weekend}
	}

	if c.Break != nil {
		after, err := minutesDuration(c.Break.AfterMinutes)
		if err != nil || after == 0 {
			return managedapps.ChildProfile{}, errors.New("break.after_minutes must be a positive duration")
		}
		duration, err := minutesDuration(c.Break.DurationMinutes)
		if err != nil || duration == 0 {
			return managedapps.ChildProfile{}, errors.New("break.duration_minutes must be a positive duration")
		}
		profile.BreakRule = policy.BreakRule{After: after, Duration: duration}
	}

	for _, configured := range c.Apps {
		if len(configured.ExecutablePaths) == 0 {
			return managedapps.ChildProfile{}, fmt.Errorf("app %q requires an executable path", configured.ID)
		}
		app := managedapps.App{
			ID:              configured.ID,
			ExecutablePaths: configured.ExecutablePaths,
		}
		if configured.Policy != nil {
			if c.Budget == nil {
				return managedapps.ChildProfile{}, fmt.Errorf("time-limited app %q requires a daily_budget", app.ID)
			}
			access, err := configured.Policy.buildPolicy(budget)
			if err != nil {
				return managedapps.ChildProfile{}, fmt.Errorf("configure app %q: %w", app.ID, err)
			}
			app.Policy = access
		}
		profile.Apps = append(profile.Apps, app)
	}

	return profile, nil
}

func (c appPolicyConfig) buildPolicy(budget policy.DailyBudget) (*policy.AppAccessPolicy, error) {
	weekday, err := buildWindows(c.WeekdayWindows)
	if err != nil {
		return nil, fmt.Errorf("weekday_windows: %w", err)
	}
	weekend, err := buildWindows(c.WeekendWindows)
	if err != nil {
		return nil, fmt.Errorf("weekend_windows: %w", err)
	}

	access := &policy.AppAccessPolicy{
		SharedBudget: budget,
		Schedule: policy.UsageSchedule{
			WeekdayWindows: weekday,
			WeekendWindows: weekend,
		},
	}
	if c.AppLimitMinutes != nil {
		limit, err := minutesDuration(*c.AppLimitMinutes)
		if err != nil {
			return nil, fmt.Errorf("app_limit_minutes: %w", err)
		}
		access.AppLimit = &policy.AppDailyLimit{Limit: limit}
	}

	return access, nil
}

func buildWindows(configured []windowConfig) ([]policy.UsageWindow, error) {
	windows := make([]policy.UsageWindow, 0, len(configured))
	for _, window := range configured {
		start, err := parseClockTime(window.Start)
		if err != nil {
			return nil, fmt.Errorf("window start: %w", err)
		}
		end, err := parseClockTime(window.End)
		if err != nil {
			return nil, fmt.Errorf("window end: %w", err)
		}
		allowed, err := policy.NewUsageWindow(start, end)
		if err != nil {
			return nil, err
		}
		windows = append(windows, allowed)
	}
	return windows, nil
}

func parseClockTime(value string) (policy.TimeOfDay, error) {
	if value == "24:00" {
		return policy.NewTimeOfDay(24, 0)
	}
	parsed, err := time.Parse("15:04", value)
	if len(value) != 5 || err != nil {
		return policy.TimeOfDay{}, fmt.Errorf("invalid clock time %q, expected HH:MM", value)
	}
	return policy.NewTimeOfDay(parsed.Hour(), parsed.Minute())
}

func minutesDuration(minutes int64) (time.Duration, error) {
	const maxMinutes = (1<<63 - 1) / int64(time.Minute)
	if minutes < 0 || minutes > maxMinutes {
		return 0, fmt.Errorf("minutes out of range: %d", minutes)
	}
	return time.Duration(minutes) * time.Minute, nil
}
