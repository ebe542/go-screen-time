package policy

import "time"

// AppDailyLimit caps the daily usage of a single managed app.
type AppDailyLimit struct {
	Limit time.Duration
}

// Allows reports whether used is below the limit.
// A nil receiver means the app has no individual limit.
func (l *AppDailyLimit) Allows(used time.Duration) bool {
	return l == nil || used < l.Limit
}
