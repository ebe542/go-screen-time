package main

import (
	"errors"
	"strings"
	"time"

	"github.com/ebe542/go-screen-time/internal/managedapps"
	"github.com/ebe542/go-screen-time/internal/policy"
	"github.com/ebe542/go-screen-time/internal/storage/sqlite"
	"github.com/ebe542/go-screen-time/internal/tracking"
)

const diagnosticTimedAppID = "diagnostic-timed-app"

type usageDiagnostic struct {
	store            *sqlite.Ledger
	timedPath        string
	unrestrictedPath string
	childID          string
	tracker          *tracking.Tracker
}

func newUsageDiagnostic(
	dbPath, timedPath, unrestrictedPath string,
	location *time.Location,
) (*usageDiagnostic, error) {
	if strings.TrimSpace(dbPath) == "" ||
		strings.TrimSpace(timedPath) == "" ||
		strings.TrimSpace(unrestrictedPath) == "" {
		return nil, errors.New("recording requires -record-db, -timed-exe, and -unrestricted-exe")
	}

	store, err := sqlite.Open(dbPath, location)
	if err != nil {
		return nil, err
	}
	return &usageDiagnostic{
		store:            store,
		timedPath:        timedPath,
		unrestrictedPath: unrestrictedPath,
	}, nil
}

func (d *usageDiagnostic) Close() error {
	return d.store.Close()
}

func (d *usageDiagnostic) Observe(observation tracking.Observation) (time.Duration, error) {
	if d.tracker == nil {
		registry, err := managedapps.NewRegistry([]managedapps.ChildProfile{{
			SID: observation.SID,
			Apps: []managedapps.App{
				{
					ID:              diagnosticTimedAppID,
					ExecutablePaths: []string{d.timedPath},
					Policy: &policy.AppAccessPolicy{
						SharedBudget: policy.DailyBudget{
							WeekdayLimit: 24 * time.Hour,
							WeekendLimit: 24 * time.Hour,
						},
					},
				},
				{
					ID:              "diagnostic-unrestricted-app",
					ExecutablePaths: []string{d.unrestrictedPath},
				},
			},
		}})
		if err != nil {
			return 0, err
		}
		d.childID = observation.SID
		d.tracker = tracking.NewTracker(registry, d.store, 15*time.Second)
	}

	if observation.SID != d.childID {
		return 0, errors.New("observation belongs to a different Windows account")
	}
	if err := d.tracker.Observe(observation); err != nil {
		return 0, err
	}

	totals, err := d.store.BudgetTotals(d.childID, diagnosticTimedAppID, observation.At)
	if err != nil {
		return 0, err
	}
	return totals.AppRegular, nil
}
