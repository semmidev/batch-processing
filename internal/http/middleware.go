package http

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/semmidev/batch-processing/internal/config"
	"go.uber.org/zap"
)

func APIKeyAuth(cfg *config.Config) func(http.Handler) http.Handler {
	validKeys := make(map[string]struct{})
	for _, k := range cfg.Security.ValidAPIKeys {
		k = strings.TrimSpace(k)
		if k != "" {
			validKeys[k] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(validKeys) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get(cfg.Security.APIKeyHeader)
			key = strings.TrimPrefix(key, "Bearer ")

			// Constant time comparison to prevent timing attacks
			valid := false
			for validKey := range validKeys {
				if subtle.ConstantTimeCompare([]byte(key), []byte(validKey)) == 1 {
					valid = true
					break
				}
			}

			if !valid {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func LoggingMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			logger.Info("http request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", ww.Status()),
				zap.Duration("duration", time.Since(start)),
				zap.String("ip", r.RemoteAddr),
			)
		})
	}
}
