// Package managedapps maps Windows accounts and executables to access policies.
package managedapps

import (
	"fmt"
	"maps"
	"path"
	"strings"

	"github.com/ebe542/go-screen-time/internal/policy"
)

// ChildProfile lists the managed apps for a Windows account.
type ChildProfile struct {
	SID       string
	Apps      []App
	BreakRule policy.BreakRule
}

// App groups executable paths that share a stable ID and an access policy.
type App struct {
	ID              string
	ExecutablePaths []string
	Policy          *policy.AppAccessPolicy
}

// Assignment identifies the child and app that a policy applies to.
type Assignment struct {
	ChildID string
	AppID   string
	Policy  *policy.AppAccessPolicy
}

// Registry looks up managed apps by Windows account SID and executable path.
type Registry struct {
	bySID      map[string]map[string]Assignment
	breakRules map[string]policy.BreakRule
}

// NewRegistry builds a registry and rejects ambiguous profile assignments.
func NewRegistry(profiles []ChildProfile) (*Registry, error) {
	registry := &Registry{
		bySID:      make(map[string]map[string]Assignment, len(profiles)),
		breakRules: make(map[string]policy.BreakRule, len(profiles)),
	}

	for _, profile := range profiles {
		if strings.TrimSpace(profile.SID) == "" {
			return nil, fmt.Errorf("child profile requires a Windows account SID")
		}
		if _, exists := registry.bySID[profile.SID]; exists {
			return nil, fmt.Errorf("duplicate child profile for SID %q", profile.SID)
		}

		byPath := make(map[string]Assignment)
		appIDs := make(map[string]struct{}, len(profile.Apps))
		for _, app := range profile.Apps {
			if strings.TrimSpace(app.ID) == "" {
				return nil, fmt.Errorf("managed app for SID %q requires an ID", profile.SID)
			}
			if _, exists := appIDs[app.ID]; exists {
				return nil, fmt.Errorf("duplicate app ID %q for SID %q", app.ID, profile.SID)
			}
			appIDs[app.ID] = struct{}{}

			assignment := Assignment{
				ChildID: profile.SID,
				AppID:   app.ID,
			}
			if app.Policy != nil {
				configured := *app.Policy
				configured.BreakRule = profile.BreakRule
				assignment.Policy = &configured
			}
			for _, executable := range app.ExecutablePaths {
				key, ok := executableKey(executable)
				if !ok {
					return nil, fmt.Errorf("app %q requires an absolute drive path, got %q",
						app.ID, executable)
				}
				if previous, exists := byPath[key]; exists && previous.AppID != app.ID {
					return nil, fmt.Errorf("path %q is assigned to apps %q and %q for SID %q",
						executable, previous.AppID, app.ID, profile.SID)
				}
				byPath[key] = assignment
			}
		}
		registry.bySID[profile.SID] = byPath
		registry.breakRules[profile.SID] = profile.BreakRule
	}

	return registry, nil
}

// Match finds the policy for an executable running under the given account.
func (r *Registry) Match(sid, executable string) (Assignment, bool) {
	key, ok := executableKey(executable)
	if !ok {
		return Assignment{}, false
	}
	assignment, found := r.bySID[sid][key]
	return assignment, found
}

func executableKey(executable string) (string, bool) {
	executable = strings.ToUpper(strings.ReplaceAll(executable, `\`, "/"))
	if len(executable) < 4 || executable[0] < 'A' || executable[0] > 'Z' ||
		executable[1] != ':' || executable[2] != '/' {
		return "", false
	}

	cleaned := path.Clean(executable[2:])
	if cleaned == "/" {
		return "", false
	}
	return executable[:2] + cleaned, true
}

// BreakRules returns a copy of the configured child break rules.
func (r *Registry) BreakRules() map[string]policy.BreakRule {
	return maps.Clone(r.breakRules)
}
