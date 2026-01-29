# Distributed Rate Limiter - Technical Context

## Project Goal

Build production-quality distributed rate limiter in Go demonstrating platform engineering thinking: distributed state management, failure modes, operational concerns, and design tradeoffs.

## Algorithm: Token Bucket

**Why token bucket:**
- Allows legitimate bursts (real user behavior)
- Industry standard (AWS, Stripe, GitHub)
- Two simple parameters: capacity (burst allowance) and refill rate (sustained rate)
- Easier to distribute than sliding window

**How it works:**
- Bucket holds tokens (capacity = max tokens)
- Refills at steady rate (e.g., 10 tokens/second)
- Each request costs tokens (typically 1)
- Allow if tokens available, reject if empty

**Example:** capacity=100, refill=10/sec
- T=0: 100 tokens, 100 requests arrive → all allowed, bucket empty
- T=0.1s: 1 token refilled, next request → rejected
- T=10s: bucket refilled to 100

## Implementation Phases

### Phase 1: Local (In-Memory) - Weeks 1-2

Single instance, no shared state.

**Interface:**
```go
type TokenBucket interface {
    Allow(tokens int) bool
    Wait(tokens int) error  // blocks until available
}
```

**Key decisions:**
- Time handling: Inject time source interface for testability
- Granularity: requests/second
- Storage: In-memory (no persistence in v1)
- Refill: Calculate tokens based on time elapsed since last check

**Test coverage:**
- Initial state (full bucket)
- Token depletion
- Refill over time
- Burst handling (can't exceed capacity)
- Edge cases (empty bucket, time boundaries)

### Phase 2: Distributed (Redis) - Weeks 3-4

Multiple instances share state via Redis.

**Why Redis:**
- Strong consistency guarantees
- Atomic operations via Lua scripts
- Well-understood failure modes
- Production-proven

**Implementation:**
- Lua script for atomic check-and-decrement
- Handle connection failures with retries
- Circuit breaker for Redis unavailability
- Metrics: hits, passes, Redis latency

**Key decisions:**
- Clock skew: Use Redis TIME command for server time, or accept drift and document
- Failure mode: Fail open (allow all), fail closed (deny all), or degrade (local limits at 1/N)
- Race conditions: Lua scripts ensure atomicity
- Consistency vs availability: Choose consistency (strict limits) over availability

### Phase 3: Coordination-Free - Weeks 5-6

Local rate limiting with no shared state.

**Approach:**
- Each instance enforces 1/N of global limit (N = number of instances)
- No coordination between instances
- Eventual consistency (temporary over-limit possible)

**Tradeoffs:**
- Pro: Fault-tolerant, no Redis dependency, lower latency
- Con: Approximate limits (can exceed during instance changes)
- Use case: High-scale services where approximate limits acceptable

**Comparison:**
- Redis: Strict limits, single point of failure, higher latency
- Coordination-free: Approximate limits, no single point of failure, lower latency

### Phase 4: Production Ready - Weeks 7-8

**Observability:**
- Structured logging (zerolog or zap)
- Distributed tracing spans (OpenTelemetry?)
- Metrics export (Prometheus format)

**Performance:**
- Benchmarks (requests/sec, latency percentiles)
- Load testing under various conditions
- Document performance characteristics

**Integration:**
- Example: Rate-limited HTTP service
- Demonstrate real-world usage

**Documentation:**
- Architecture diagrams
- All implementations explained
- Decision rationale documented

## Key Technical Decisions

### Time Handling
**Options:**
1. Use `time.Now()` directly - simple but hard to test
2. Inject time source interface - testable but more verbose
3. Use build tags for test time - clean API but complex builds

**Choice:** Inject time source for explicit dependencies and testability.

### Token Refill Strategy
**Options:**
1. Discrete: Refill N tokens every interval
2. Continuous: Calculate available tokens based on time elapsed

**Choice:** Continuous refill - more accurate, smoother behavior.

```go
elapsed := now.Sub(lastCheck)
tokensToAdd := elapsed.Seconds() * refillRate
currentTokens = min(currentTokens + tokensToAdd, capacity)
```

### Redis Failure Handling
**Options:**
1. Fail open: Allow all requests (prefer availability)
2. Fail closed: Deny all requests (prefer security)
3. Degrade: Local limits at 1/N (balanced approach)

**Choice:** Configurable, default to degrade mode - document the tradeoff.

### Atomicity in Redis
**Options:**
1. WATCH/MULTI/EXEC: Optimistic locking with retries
2. Lua scripts: Server-side atomic operations

**Choice:** Lua scripts - single round-trip, no retry logic needed, simpler.

```lua
-- Pseudo-code for Lua script
local tokens = redis.call('GET', key)
if tokens >= requested then
    redis.call('DECRBY', key, requested)
    return 1  -- allowed
else
    return 0  -- denied
end
```

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                   Client Requests                    │
└──────────────────────┬──────────────────────────────┘
                       │
         ┌─────────────┴─────────────┐
         │                           │
    ┌────▼────┐                 ┌────▼────┐
    │Instance1│                 │Instance2│
    │ (Local) │                 │ (Local) │
    └────┬────┘                 └────┬────┘
         │                           │
         └─────────────┬─────────────┘
                       │
                  ┌────▼────┐
                  │  Redis  │  (Shared state)
                  └─────────┘

Local: Each instance independent
Distributed: Instances coordinate via Redis
```

## Project Structure

```
distributed-rate-limiter/
├── README.md
├── docs/
│   ├── DECISIONS.md      # Log all technical decisions
│   └── architecture.png  # Visual diagrams
├── limiter/
│   ├── token_bucket.go       # Local implementation
│   ├── token_bucket_test.go
│   ├── redis_limiter.go      # Redis-backed
│   ├── redis_limiter_test.go
│   ├── local_limiter.go      # Coordination-free
│   └── interface.go          # Common interface
├── server/
│   └── main.go              # Example HTTP service
├── scripts/
│   └── load_test.sh         # Performance testing
└── go.mod
```

## Testing Strategy

**Unit tests:**
- Token bucket algorithm correctness
- Edge cases (empty, full, partial tokens)
- Time-based calculations

**Integration tests:**
- Redis integration (requires test Redis instance)
- Multiple instances competing for same limit
- Connection failure scenarios

**Chaos tests:**
- Kill Redis mid-request
- Network partitions
- Clock skew between instances
- Rapid instance scaling (N changes)

**Performance tests:**
- Throughput benchmarks
- Latency percentiles (p50, p95, p99)
- Load testing under sustained traffic

## Critical Technical Challenges

1. **Clock skew in distributed system**
   - Problem: Instances may have different system times
   - Solution: Use Redis TIME, or accept drift and document impact

2. **Race conditions with concurrent requests**
   - Problem: Multiple requests checking/decrementing simultaneously
   - Solution: Atomic operations (Lua scripts in Redis)

3. **Partial failures**
   - Problem: Redis available to some instances, not others
   - Solution: Circuit breaker per-instance, local fallback

4. **Token refill precision**
   - Problem: Fractional tokens, rounding errors over time
   - Solution: Use float64 for calculations, document precision limits

5. **Memory management**
   - Problem: State for every rate-limited key in memory
   - Solution: TTL-based eviction, LRU cache for local implementation

## Production Considerations (Document, Don't Implement)

What would be needed for real production use:

- [ ] Dynamic configuration (adjust limits without restart)
- [ ] Multi-dimensional rate limiting (by IP + user + endpoint)
- [ ] Rate limit by custom keys (not just fixed keys)
- [ ] Persistent storage backup (Redis persistence, snapshots)
- [ ] Monitoring dashboards (Grafana integration)
- [ ] Alerting on anomalies (sudden rate limit hits)
- [ ] A/B testing different rate limit strategies
- [ ] Geographic distribution (multi-region Redis)

## Success Criteria

**Code quality:**
- Idiomatic Go, clean interfaces
- >80% test coverage
- Error handling with context
- No race conditions (verified with `-race` flag)

**Documentation:**
- README explains all implementations
- DECISIONS.md documents all major choices
- Architecture diagrams show data flow
- Code comments for non-obvious logic

**Operational thinking:**
- Failure modes analyzed and documented
- Performance characteristics benchmarked
- Observability built-in (logging, metrics)
- Real-world usage example included

**Interview readiness:**
- Can articulate design tradeoffs
- Understand why each choice was made
- Know what would change at scale
- Have concrete performance numbers

## Implementation Notes

**Start simple:**
- Get local version working first
- Add Redis only after local is solid
- Don't prematurely optimize

**Document decisions immediately:**
- Update DECISIONS.md as you go
- You'll forget reasoning in a week
- Decision log becomes interview prep gold

**Test failure modes:**
- Don't just test happy path
- Kill dependencies, introduce latency
- Simulate production chaos

**Measure everything:**
- Benchmark early and often
- Profile for bottlenecks
- Document performance characteristics

## Timeline

- Week 1-2: Local implementation + tests
- Week 3-4: Redis-backed distributed version
- Week 5-6: Coordination-free alternative + comparison
- Week 7-8: Production polish + documentation

Ship v1.0 by end of week 8.
