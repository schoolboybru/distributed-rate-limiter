package main

import (
	"context"
	"net/http"
	"time"

	"github.com/schoolboybru/distributed-rate-limiter/limiter"
)

func main() {
	bucket := limiter.NewTokenBucket(5, 1, limiter.RealClock{})

	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		if !bucket.Allow(1) {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Rate limited! Try again later.\n"))
		}
		w.Write([]byte("pong\n"))
	})

	http.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := bucket.Wait(ctx, 1); err != nil {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Timed out waiting for rate limit.\n"))
		}
		w.Write([]byte("Completed slow operation!\n"))
	})

	println("Server running on :8080")
	http.ListenAndServe(":8080", nil)
}
