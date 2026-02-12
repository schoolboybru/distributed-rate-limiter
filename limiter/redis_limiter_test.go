package limiter

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type MockMetrics struct {
	mu        sync.Mutex
	allows    []string
	denies    []string
	errors    []string
	latencies []time.Duration
}

func (m *MockMetrics) OnAllow(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.allows = append(m.allows, key)
}

func (m *MockMetrics) OnDeny(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.denies = append(m.denies, key)
}

func (m *MockMetrics) OnError(key string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors = append(m.errors, key)
}

func (m *MockMetrics) OnLatency(key string, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latencies = append(m.latencies, d)
}

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

func TestMetrics_OnAllowCalled(t *testing.T) {
	client := setupTestRedis(t)
	key := "test:metrics:allow"
	defer cleanupKey(t, client, "ratelimit:"+key)

	metrics := &MockMetrics{}
	limiter := NewRedisLimiter(client, 5, 1, "ratelimit:", WithMetrics(metrics))

	limiter.Allow(key, 3)

	if !slices.Contains(metrics.allows, key) {
		t.Error("expected metrics.allows to contain the key")
	}

	if len(metrics.latencies) != 1 {
		t.Errorf("expected metrics.latencies to have 1 entry, go %d", len(metrics.latencies))
	}
}

func TestMetrics_OnDenyCalled(t *testing.T) {
	client := setupTestRedis(t)
	key := "test:metrics:deny"
	defer cleanupKey(t, client, "ratelimit:"+key)

	metrics := &MockMetrics{}
	limiter := NewRedisLimiter(client, 5, 1, "ratelimit:", WithMetrics(metrics))

	limiter.Allow(key, 5)
	limiter.Allow(key, 1)
	if !slices.Contains(metrics.denies, key) {
		t.Error("expected metrics.denies to contain the key")
	}
}
func TestFailOpen_AllowsWhenRedisDown(t *testing.T) {
	// Create client pointing to non-existent Redis
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:9999", // wrong port
	})
	limiter := NewRedisLimiter(client, 5, 1, "ratelimit:", WithFailureMode(FailOpen))

	if !limiter.Allow("ErrorKey", 5) {
		t.Error("expected allow to be true for non-existent redis client with FailOpen")
	}
}
func TestFailClosed_DeniesWhenRedisDown(t *testing.T) {
	// Create client pointing to non-existent Redis
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:9999", // wrong port
	})
	limiter := NewRedisLimiter(client, 5, 1, "ratelimit:", WithFailureMode(FailClosed))

	if limiter.Allow("ErrorKey", 5) {
		t.Error("expected allow to be false for non-existent redis client with FailClosed")
	}
}
func TestFailDegrade_UsesLocalLimiter(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:9999",
	})
	limiter := NewRedisLimiter(client, 5, 0, "ratelimit:", WithFailureMode(FailDegrade))

	limiter.Allow("Degrade", 1)
	limiter.Allow("Degrade", 1)
	limiter.Allow("Degrade", 1)
	limiter.Allow("Degrade", 1)
	limiter.Allow("Degrade", 1)

	if limiter.Allow("Degrade", 1) {
		t.Error("expected allow to be false for FailDegrade and using local limiter")
	}
}
func TestCircuitBreaker_IntegrationFailsFast(t *testing.T) {
	// Create client pointing to non-existent Redis
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:9999", // wrong port
	})
	metrics := &MockMetrics{}
	limiter := NewRedisLimiter(client, 5, 1, "ratelimit:",
		WithCircuitBreaker(3, 30*time.Second),
		WithMetrics(metrics),
	)

	limiter.Allow("Fail", 1)
	limiter.Allow("Fail", 1)
	limiter.Allow("Fail", 1)

	limiter.Allow("Fail", 1)

	if len(metrics.errors) < 4 {
		t.Errorf("expected at least 4 errors, got %d", len(metrics.errors))
	}
}
