package api

import (
	"context"
	"log/slog"
	"math/rand"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"
)

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = "req-" + strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + strconv.Itoa(rand.Intn(10000))
		}
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, requestID)))
	})
}

func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error("panic recovered", "error", recovered, "stack", string(debug.Stack()))
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func AccessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			next.ServeHTTP(w, r)
			logger.Info("http request", "method", r.Method, "path", r.URL.Path, "request_id", RequestIDFromContext(r.Context()), "duration_ms", time.Since(started).Milliseconds())
		})
	}
}

func Timeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UserHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := UserContext{
			ID:        r.Header.Get("X-User-ID"),
			Role:      r.Header.Get("X-User-Role"),
			SessionID: r.Header.Get("X-Session-ID"),
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, user)))
	})
}
