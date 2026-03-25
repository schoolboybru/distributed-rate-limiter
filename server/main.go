package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/schoolboybru/distributed-rate-limiter/limiter"
)

func main() {
	logger := zerolog.New(os.Stdout).With().Timestamp().Str("service", "rate-limiter-server").Logger()

	registry := prometheus.NewRegistry()

	metrics := limiter.NewPrometheusMetrics(registry, "ratelimiter")

	partitionLimiter := limiter.NewStaticPartitionLimiter(5, 1, 2, limiter.WithPartitionMetrics(metrics))

	http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		if !partitionLimiter.Allow("default", 1) {
			logger.Warn().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", http.StatusTooManyRequests).
				Dur("latency", time.Since(start)).
				Msg("request denied by rate limiter")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Rate limited! Try again later.\n"))
			return
		}

		logger.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", http.StatusOK).
			Dur("latency", time.Since(start)).
			Msg("request allowed")
		w.Write([]byte("pong\n"))
	})

	http.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := partitionLimiter.Wait(ctx, "default", 1); err != nil {
			logger.Warn().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", http.StatusTooManyRequests).
				Err(err).
				Dur("latency", time.Since(start)).
				Msg("request timed out waiting for rate limiter")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Timed out waiting for rate limit.\n"))
			return
		}

		logger.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", http.StatusOK).
			Dur("latency", time.Since(start)).
			Msg("request completed after rate limiter wait")
		w.Write([]byte("Completed slow operation!\n"))
	})

	logger.Info().Str("addr", ":8080").Msg("server starting")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		logger.Fatal().Err(err).Msg("server stopped")
	}
}
