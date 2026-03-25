package limiter

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	benchCapacity   = 1_000_000_000_000
	benchRefillRate = 1_000_000_000_000
	benchKeyCount   = 1024
)

func BenchmarkTokenBucket_Allow(b *testing.B) {
	bucket := NewTokenBucket(benchCapacity, benchRefillRate, RealClock{})

	benchmarkAllowSerial(b, func(i int) bool {
		return bucket.Allow(1)
	})
}

func BenchmarkTokenBucket_Allow_Parallel(b *testing.B) {
	bucket := NewTokenBucket(benchCapacity, benchRefillRate, RealClock{})

	benchmarkAllowParallel(b, func(i int) bool {
		return bucket.Allow(1)
	})
}

func BenchmarkKeyedLimiter_Allow_SingleKey(b *testing.B) {
	limiter := NewKeyedLimiter(benchCapacity, benchRefillRate, RealClock{})
	const key = "bench:single"

	benchmarkAllowSerial(b, func(i int) bool {
		return limiter.Allow(key, 1)
	})
}

func BenchmarkKeyedLimiter_Allow_ManyKeys(b *testing.B) {
	limiter := NewKeyedLimiter(benchCapacity, benchRefillRate, RealClock{})
	keys := makeKeys("bench:keyed:", benchKeyCount)

	benchmarkAllowSerial(b, func(i int) bool {
		return limiter.Allow(keys[i%len(keys)], 1)
	})
}

func BenchmarkKeyedLimiter_Allow_Parallel_SingleKey(b *testing.B) {
	limiter := NewKeyedLimiter(benchCapacity, benchRefillRate, RealClock{})
	const key = "bench:single"

	benchmarkAllowParallel(b, func(i int) bool {
		return limiter.Allow(key, 1)
	})
}

func BenchmarkStaticPartitionLimiter_Allow(b *testing.B) {
	limiter := NewStaticPartitionLimiter(benchCapacity, benchRefillRate, 4)
	const key = "bench:single"

	benchmarkAllowSerial(b, func(i int) bool {
		return limiter.Allow(key, 1)
	})
}

func BenchmarkStaticPartitionLimiter_Allow_Parallel(b *testing.B) {
	limiter := NewStaticPartitionLimiter(benchCapacity, benchRefillRate, 4)
	const key = "bench:single"

	benchmarkAllowParallel(b, func(i int) bool {
		return limiter.Allow(key, 1)
	})
}

func BenchmarkRedisLimiter_Allow_SingleKey(b *testing.B) {
	limiter, keys := newRedisLimiterForBench(b, 1)
	key := keys[0]

	benchmarkAllowSerial(b, func(i int) bool {
		return limiter.Allow(key, 1)
	})
}

func BenchmarkRedisLimiter_Allow_ManyKeys(b *testing.B) {
	limiter, keys := newRedisLimiterForBench(b, benchKeyCount)

	benchmarkAllowSerial(b, func(i int) bool {
		return limiter.Allow(keys[i%len(keys)], 1)
	})
}

func BenchmarkRedisLimiter_Allow_Parallel_SingleKey(b *testing.B) {
	limiter, keys := newRedisLimiterForBench(b, 1)
	key := keys[0]

	benchmarkAllowParallel(b, func(i int) bool {
		return limiter.Allow(key, 1)
	})
}

func benchmarkAllowSerial(b *testing.B, allow func(i int) bool) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = allow(i)
	}
}

func benchmarkAllowParallel(b *testing.B, allow func(i int) bool) {
	var counter uint64

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := int(atomic.AddUint64(&counter, 1) - 1)
			_ = allow(i)
		}
	})
}

func makeKeys(prefix string, n int) []string {
	keys := make([]string, n)
	for i := 0; i < n; i++ {
		keys[i] = fmt.Sprintf("%s%d", prefix, i)
	}
	return keys
}

func newRedisLimiterForBench(b *testing.B, keyCount int) (*RedisLimiter, []string) {
	b.Helper()

	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})

	if err := client.Ping(ctx).Err(); err != nil {
		b.Skip("Redis not available, skipping Redis benchmarks")
	}

	prefix := fmt.Sprintf("bench:%d:", time.Now().UnixNano())
	keys := makeKeys("key:", keyCount)

	limiter := NewRedisLimiter(client, benchCapacity, benchRefillRate, prefix)

	b.Cleanup(func() {
		redisKeys := make([]string, len(keys))
		for i, key := range keys {
			redisKeys[i] = prefix + key
		}

		_ = client.Del(ctx, redisKeys...).Err()
		_ = client.Close()
	})

	return limiter, keys
}
