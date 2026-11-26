// Command sessionhelper reports foreground observations for its Windows session.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"os/signal"

	"github.com/ebe542/go-screen-time/internal/sessionobserver"
	"github.com/ebe542/go-screen-time/internal/tracking"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	output := json.NewEncoder(os.Stdout)
	err := sessionobserver.RunWindowsMonitor(ctx, func(
		observation tracking.Observation,
		readErr error,
	) error {
		if readErr != nil {
			log.Printf("foreground observation: %v", readErr)
		}
		return output.Encode(struct {
			tracking.Observation
			Status string `json:"status"`
		}{
			Observation: observation,
			Status:      statusName(observation.State),
		})
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func statusName(state tracking.ObservationState) string {
	switch state {
	case tracking.ObservationForeground:
		return "foreground"
	case tracking.ObservationInactive:
		return "inactive"
	default:
		return "unavailable"
	}
}
