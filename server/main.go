package main

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/schoolboybru/distributed-rate-limiter/limiter"
)

func main() {
	registry := prometheus.NewRegistry()

	metrics := limiter.NewPrometheusMetrics(registry, "ratelimiter")

	partitionLimiter := limiter.NewStaticPartitionLimiter(5, 1, 2, limiter.WithPartitionMetrics(metrics))

	http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		if !partitionLimiter.Allow("default", 1) {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Rate limited! Try again later.\n"))
			return
		}
		w.Write([]byte("pong\n"))
	})

	http.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := partitionLimiter.Wait(ctx, "default", 1); err != nil {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Timed out waiting for rate limit.\n"))
			return
		}
		w.Write([]byte("Completed slow operation!\n"))
	})

	println("Server running on :8080")
	http.ListenAndServe(":8080", nil)
}
