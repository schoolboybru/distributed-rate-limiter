package limiter

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func setupTestRedis(t *testing.T) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Skip("Redis not available, skipping integration test")
	}

	return client
}

func cleanupKey(t *testing.T, client *redis.Client, key string) {
	client.Del(context.Background(), key)
}

func TestAllow_InitialBucket(t *testing.T) {
	client := setupTestRedis(t)
	key := "test:initial"
	defer cleanupKey(t, client, "ratelimit:"+key)

	limiter := NewRedisLimiter(client, 5, 1, "ratelimit:")

	for i := range 5 {
		if !limiter.Allow(key, 1) {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	if limiter.Allow(key, 1) {
		t.Error("request 6 should be denied")
	}
}

func TestAllow_Refill(t *testing.T) {
	client := setupTestRedis(t)
	key := "test:refill"
	defer cleanupKey(t, client, "ratelimit:"+key)

	limiter := NewRedisLimiter(client, 5, 1, "ratelimit:")

	limiter.Allow(key, 5)

	time.Sleep(1 * time.Second)

	if !limiter.Allow(key, 1) {
		t.Error("request should allow 1 token after 1 second")
	}
}

func TestAllow_DifferentKeys(t *testing.T) {
	client := setupTestRedis(t)
	key1 := "test:key1"
	key2 := "test:key2"
	defer cleanupKey(t, client, "ratelimit:"+key1)
	defer cleanupKey(t, client, "ratelimit:"+key2)

	limiter := NewRedisLimiter(client, 5, 1, "ratelimit:")

	limiter.Allow(key1, 2)
	if !limiter.Allow(key2, 2) {
		t.Error("request should allow second key")
	}
}

func TestWait_Success(t *testing.T) {
	client := setupTestRedis(t)
	key := "test:wait"
	defer cleanupKey(t, client, "ratelimit:"+key)

	limiter := NewRedisLimiter(client, 5, 1, "ratelimit:")

	err := limiter.Wait(context.Background(), key, 5)

	if err != nil {
		t.Errorf("Wait should return nil, got %v", err)
	}
}

func TestWait_ContextTimeoutRedis(t *testing.T) {
	client := setupTestRedis(t)
	key := "test:timeout"
	defer cleanupKey(t, client, "ratelimit:"+key)

	limiter := NewRedisLimiter(client, 5, 1, "ratelimit:")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	limiter.Allow(key, 5)

	err := limiter.Wait(ctx, key, 5)

	if err != context.DeadlineExceeded {
		t.Errorf("expecting DeadlineExceeded, got %v", err)
	}
}

func TestWait_ExceedsCapacity(t *testing.T) {
	client := setupTestRedis(t)
	key := "test:exceed"
	defer cleanupKey(t, client, "ratelimit:"+key)

	limiter := NewRedisLimiter(client, 5, 1, "ratelimit:")

	err := limiter.Wait(context.Background(), key, 20)

	if err != ErrExceedsCapacity {
		t.Errorf("expecting ErrExceedsCapacity, got %v", err)
	}
}

func TestAllow_ConcurrentRedis(t *testing.T) {
	client := setupTestRedis(t)
	key := "test:concurrent"
	defer cleanupKey(t, client, "ratelimit:"+key)

	limiter := NewRedisLimiter(client, 10, 0, "ratelimit:")

	var allowed int64
	var wg sync.WaitGroup

	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if limiter.Allow(key, 1) {
				atomic.AddInt64(&allowed, 1)
			}
		}()
	}

	wg.Wait()

	if allowed != 10 {
		t.Errorf("expected 10 allowed, got %d", allowed)
	}
}
