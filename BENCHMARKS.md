# Benchmarks

Recorded benchmark runs for the distributed rate limiter.

---

## Environment

- Date: 2026-03-24
- Machine/CPU: Apple M4
- OS: Darwin 24.6.0 (darwin/arm64)
- Go version (`go version`): go1.25.6 darwin/arm64
- Git commit: `344ce8e`
- Redis version (if applicable): Redis server v7.4.7

---

## Commands

### Local limiter benchmarks (executed)

```bash
go test ./limiter -run '^$' -bench 'TokenBucket|KeyedLimiter|StaticPartition' -benchmem -benchtime=5s -count=1
```

### Redis limiter benchmarks (executed)

```bash
go test ./limiter -run '^$' -bench 'RedisLimiter' -benchmem -benchtime=5s -count=1
```

### Full benchmark suite (executed)

```bash
go test ./limiter -run '^$' -bench . -benchmem -benchtime=5s -count=1
```

### CPU scaling (executed)

```bash
go test ./limiter -run '^$' -bench . -benchmem -cpu=1,2,4,8 -benchtime=3s -count=1
```

### Save output for comparison (not executed in this run)

```bash
go test ./limiter -run '^$' -bench . -benchmem -count=10 > before.txt
# (make changes)
go test ./limiter -run '^$' -bench . -benchmem -count=10 > after.txt
benchstat before.txt after.txt
```

---

## Microbenchmark Results (`testing.B`)

| Benchmark | ns/op | B/op | allocs/op | Notes |
|---|---:|---:|---:|---|
| BenchmarkTokenBucket_Allow | 45.79 | 0 | 0 | Serial local baseline |
| BenchmarkTokenBucket_Allow_Parallel | 118.2 | 0 | 0 | Mutex contention under parallel load |
| BenchmarkKeyedLimiter_Allow_SingleKey | 51.29 | 0 | 0 | Single hot key |
| BenchmarkKeyedLimiter_Allow_ManyKeys | 51.61 | 0 | 0 | 1024 rotating keys |
| BenchmarkKeyedLimiter_Allow_Parallel_SingleKey | 136.2 | 0 | 0 | Hot-key contention path |
| BenchmarkStaticPartitionLimiter_Allow | 52.06 | 0 | 0 | Wrapper overhead near KeyedLimiter |
| BenchmarkStaticPartitionLimiter_Allow_Parallel | 133.3 | 0 | 0 | Similar contention profile to KeyedLimiter |
| BenchmarkRedisLimiter_Allow_SingleKey | 69486 | 504 | 17 | Includes Redis round-trip + Lua eval |
| BenchmarkRedisLimiter_Allow_ManyKeys | 69215 | 518 | 17 | Slightly higher bytes due key handling |
| BenchmarkRedisLimiter_Allow_Parallel_SingleKey | 25196 | 506 | 17 | Better amortization under concurrency |

### CPU scaling highlights (`-cpu=1,2,4,8`, 3s)

| Benchmark | cpu=1 ns/op | cpu=2 ns/op | cpu=4 ns/op | cpu=8 ns/op |
|---|---:|---:|---:|---:|
| TokenBucket_Allow_Parallel | 47.50 | 85.05 | 107.8 | 115.8 |
| KeyedLimiter_Allow_Parallel_SingleKey | 52.66 | 116.2 | 113.7 | 130.5 |
| StaticPartitionLimiter_Allow_Parallel | 52.58 | 114.7 | 114.0 | 131.2 |
| RedisLimiter_Allow_Parallel_SingleKey | 68582 | 38509 | 29011 | 25963 |

---

## HTTP Load Test (Percentiles)

Use your server + a load tool (e.g., vegeta) to capture p50/p95/p99 and throughput.

### Commands used (vegeta)

```bash
echo "GET http://localhost:8080/ping" | "$(go env GOPATH)/bin/vegeta" attack -duration=30s -rate=200 | tee /tmp/ping.bin | "$(go env GOPATH)/bin/vegeta" report
"$(go env GOPATH)/bin/vegeta" report -type='hist[0,1ms,2ms,5ms,10ms,20ms,50ms,100ms]' /tmp/ping.bin
```

```bash
echo "GET http://localhost:8080/slow" | "$(go env GOPATH)/bin/vegeta" attack -duration=30s -rate=25 | tee /tmp/slow.bin | "$(go env GOPATH)/bin/vegeta" report
"$(go env GOPATH)/bin/vegeta" report -type='hist[0,5ms,10ms,20ms,50ms,100ms,250ms,500ms,1s,2s,5s]' /tmp/slow.bin
```

### HTTP results table

| Endpoint | Rate (req/s) | Duration | Throughput | p50 | p95 | p99 | 429 % | Notes |
|---|---:|---:|---:|---:|---:|---:|---:|---|
| /ping | 200.03 | 29.995s | 0.57 | 248.204µs | 462.042µs | 796.172µs | 99.72% | Server is intentionally configured with very low limit, so almost all requests get 429 |
| /slow | 25.03 | 31.961s | 0.50 | 2.001s | 2.002s | 2.002s | 97.87% | `Wait` timeout is 2s; most requests time out under this offered load |

---

## Findings / Takeaways

- Local in-memory limiters are very fast (~46-52 ns/op serial, 0 allocs/op).
- Redis limiter is network-bound (~69 µs/op serial) with 17 allocs/op from client/script call path.
- Under higher parallelism, Redis parallel benchmark improves per-op time (better pipelining/concurrency), while local hot-key paths show lock-contention growth.
