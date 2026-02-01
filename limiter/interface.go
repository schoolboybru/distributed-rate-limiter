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
