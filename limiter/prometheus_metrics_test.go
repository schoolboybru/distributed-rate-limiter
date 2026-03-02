package limiter

import (
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	dto "github.com/prometheus/client_model/go"
)

var _ Metrics = (*PrometheusMetrics)(nil)

func newTestMetrics(t *testing.T) *PrometheusMetrics {
	t.Helper()
	registry := prometheus.NewRegistry()
	return NewPrometheusMetrics(registry, "test")
}

func TestPrometheusMetrics_OnAllow(t *testing.T) {
	pm := newTestMetrics(t)

	pm.OnAllow("user:1")
	pm.OnAllow("user:1")
	pm.OnAllow("user:1")

	if testutil.ToFloat64(pm.allowed.WithLabelValues("user:1")) != 3 {
		t.Errorf("expected allowed to be called 3 times, got %d", pm.allowed.WithLabelValues("user:1"))
	}
}

func TestPrometheusMetrics_OnAllow_MultipleKeys(t *testing.T) {
	pm := newTestMetrics(t)

	pm.OnAllow("user:1")
	pm.OnAllow("user:1")
	pm.OnAllow("user:2")

	if testutil.ToFloat64(pm.allowed.WithLabelValues("user:1")) != 2 {
		t.Errorf("expected allowed to be called 2 times for user1, got %d", pm.allowed.WithLabelValues("user:1"))
	}

	if testutil.ToFloat64(pm.allowed.WithLabelValues("user:2")) != 1 {
		t.Errorf("expected allowed to be called 1 time for user2, got %d", pm.allowed.WithLabelValues("user:2"))
	}
}

func TestPrometheusMetrics_OnDeny(t *testing.T) {
	pm := newTestMetrics(t)

	pm.OnDeny("user:1")
	pm.OnDeny("user:1")
	pm.OnDeny("user:1")

	if testutil.ToFloat64(pm.denied.WithLabelValues("user:1")) != 3 {
		t.Errorf("expected denied to be called 3 times, got %d", pm.denied.WithLabelValues("user:1"))
	}
}

func TestPrometheusMetrics_OnError_RedisError(t *testing.T) {
	pm := newTestMetrics(t)

	pm.OnError("user:1", fmt.Errorf("connection refused"))

	if testutil.ToFloat64(pm.errors.WithLabelValues("user:1", "redis_error")) != 1 {
		t.Errorf("expected error for redis_error to be called 1 times, got %d", pm.errors.WithLabelValues("user:1", "redis_error"))
	}
}

func TestPrometheusMetrics_OnError_CircuitOpen(t *testing.T) {
	pm := newTestMetrics(t)

	pm.OnError("user:1", ErrCircuitOpen)

	if testutil.ToFloat64(pm.errors.WithLabelValues("user:1", "circuit_open")) != 1 {
		t.Errorf("expected error for circuit_open to be called 1 times, got %d", pm.errors.WithLabelValues("user:1", "circuit_open"))
	}
}

func TestPrometheusMetrics_OnError_WrappedCircuitOpen(t *testing.T) {
	pm := newTestMetrics(t)

	pm.OnError("user:1", fmt.Errorf("something: %w", ErrCircuitOpen))

	if testutil.ToFloat64(pm.errors.WithLabelValues("user:1", "circuit_open")) != 1 {
		t.Errorf("expected error for circuit_open to be called 1 times, got %d", pm.errors.WithLabelValues("user:1", "circuit_open"))
	}
}

func TestPrometheusMetrics_OnLatency(t *testing.T) {
	pm := newTestMetrics(t)

	pm.OnLatency("user:1", 150*time.Millisecond)

	observer, _ := pm.latency.GetMetricWithLabelValues("user:1")
	h := observer.(prometheus.Histogram)

	var m dto.Metric
	h.Write(&m)

	if m.GetHistogram().GetSampleCount() != 1 {
		t.Errorf("expected sample count 1, got %d", m.GetHistogram().GetSampleCount())
	}
	if m.GetHistogram().GetSampleSum() != 0.15 {
		t.Errorf("expected sample sum 0.15, got %f", m.GetHistogram().GetSampleSum())
	}
}
