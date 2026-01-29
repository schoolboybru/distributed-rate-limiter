package limiter

import "time"

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

type Limiter interface {
	Allow(key string, tokens int) bool
	// Wait(key string, tokens int) error
}
