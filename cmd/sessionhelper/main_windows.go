// Command sessionhelper reports foreground observations for its Windows session.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/ebe542/go-screen-time/internal/observationpipe"
	"github.com/ebe542/go-screen-time/internal/sessionobserver"
	"github.com/ebe542/go-screen-time/internal/tracking"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	dbPath := flag.String("record-db", "", "SQLite file for the foreground usage test")
	timedPath := flag.String("timed-exe", "", "full path of the time-limited test executable")
	unrestrictedPath := flag.String("unrestricted-exe", "", "full path of the unrestricted test executable")
	pipeName := flag.String("pipe", "", "send observations to a local service pipe")
	flag.Parse()

	if *pipeName != "" && (*dbPath != "" || *timedPath != "" || *unrestrictedPath != "") {
		return errors.New("-pipe cannot be combined with recording options")
	}

	var diagnostic *usageDiagnostic
	if *dbPath != "" || *timedPath != "" || *unrestrictedPath != "" {
		var err error
		diagnostic, err = newUsageDiagnostic(*dbPath, *timedPath, *unrestrictedPath, time.Local)
		if err != nil {
			return err
		}
		defer diagnostic.Close()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var sender *observationpipe.Sender
	if *pipeName != "" {
		dialCtx, cancelDial := context.WithTimeout(ctx, 5*time.Second)
		connected, err := observationpipe.Dial(dialCtx, *pipeName)
		cancelDial()
		if err != nil {
			return err
		}
		sender = connected
		defer sender.Close()
	}

	output := json.NewEncoder(os.Stdout)
	err := sessionobserver.RunWindowsMonitor(ctx, func(
		observation tracking.Observation,
		readErr error,
	) error {
		if readErr != nil {
			log.Printf("foreground observation: %v", readErr)
		}

		if sender != nil {
			sendCtx, cancelSend := context.WithTimeout(ctx, 5*time.Second)
			defer cancelSend()
			return sender.Send(sendCtx, observation)
		}

		line := struct {
			tracking.Observation
			Status     string `json:"status"`
			MediaUsage string `json:"media_usage,omitempty"`
		}{
			Observation: observation,
			Status:      statusName(observation.State),
		}

		if diagnostic != nil {
			used, err := diagnostic.Observe(observation)
			if err != nil {
				return err
			}
			line.MediaUsage = used.String()
		}

		return output.Encode(line)
	})
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
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
