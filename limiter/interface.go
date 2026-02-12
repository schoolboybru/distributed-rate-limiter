package limiter

import (
	"context"
	"errors"
	"time"
)

var ErrExceedsCapacity = errors.New("requested tokens exceeds bucket capacity")

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

type Limiter interface {
	Allow(key string, tokens int) bool
	Wait(ctx context.Context, key string, tokens int) error
}

type Metrics interface {
	OnAllow(key string)
	OnDeny(key string)
	OnError(key string, err error)
	OnLatency(key string, d time.Duration)
}

type NoopMetrics struct{}

func (NoopMetrics) OnAllow(key string)                    {}
func (NoopMetrics) OnDeny(key string)                     {}
func (NoopMetrics) OnError(key string, err error)         {}
func (NoopMetrics) OnLatency(key string, d time.Duration) {}
