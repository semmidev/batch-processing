package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/semmidev/batch-processing/internal/config"
	"github.com/semmidev/batch-processing/internal/observability"
)

func NewRouter(cfg *config.Config, handler *Handler) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(LoggingMiddleware(observability.Log))
	r.Use(middleware.Recoverer)

	// Public routes
	r.Get("/health", handler.Health)
	r.Handle("/metrics", promhttp.Handler())

	// Protected API routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(APIKeyAuth(cfg))
		r.Post("/batches", handler.SubmitBatch)
		r.Get("/batches/{id}/status", handler.GetBatchStatus)
		r.Post("/batches/{id}/cancel", handler.CancelBatch)
	})

	return r
}
