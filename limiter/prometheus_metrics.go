package limiter

import (
	"errors"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type PrometheusMetrics struct {
	allowed *prometheus.CounterVec
	denied  *prometheus.CounterVec
	errors  *prometheus.CounterVec
	latency *prometheus.HistogramVec
}

func NewPrometheusMetrics(registry *prometheus.Registry, namespace string) *PrometheusMetrics {
	allowed := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "requests_allowed_total",
		Help:      "Total number of allowed requests",
	}, []string{"key"})

	denied := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "requests_denied_total",
		Help:      "Total number of denied requests",
	}, []string{"key"})

	errors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "requests_errors_total",
		Help:      "Total number of errored requests",
	}, []string{"key", "error_type"})

	latency := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "requests_duration_seconds",
		Help:      "Latency of rate limiter operations in seconds",
	}, []string{"key"})

	registry.MustRegister(allowed, denied, errors, latency)

	return &PrometheusMetrics{
		allowed: allowed,
		denied:  denied,
		errors:  errors,
		latency: latency,
	}
}

func (pm *PrometheusMetrics) OnAllow(key string) {
	pm.allowed.With(prometheus.Labels{"key": key}).Inc()
}

func (pm *PrometheusMetrics) OnDeny(key string) {
	pm.denied.With(prometheus.Labels{"key": key}).Inc()
}

func (pm *PrometheusMetrics) OnError(key string, err error) {
	isCircuitOpenError := errors.Is(err, ErrCircuitOpen)
	errorType := "redis_error"

	if isCircuitOpenError {
		errorType = "circuit_open"
	}

	pm.errors.With(prometheus.Labels{"key": key, "error_type": errorType}).Inc()
}

func (pm *PrometheusMetrics) OnLatency(key string, d time.Duration) {
	pm.latency.With(prometheus.Labels{"key": key}).Observe(d.Seconds())
}
