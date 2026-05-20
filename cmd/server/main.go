package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/semmidev/batch-processing/internal/config"
	"github.com/semmidev/batch-processing/internal/database"
	internalhttp "github.com/semmidev/batch-processing/internal/http"
	"github.com/semmidev/batch-processing/internal/observability"
	"github.com/semmidev/batch-processing/internal/repository"
	"github.com/semmidev/batch-processing/internal/service"
	"go.uber.org/zap"
)

func main() {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		panic(fmt.Sprintf("Failed to load config: %v", err))
	}

	// Init Logger
	if err := observability.InitLogger(cfg.LogLevel); err != nil {
		panic(fmt.Sprintf("Failed to init logger: %v", err))
	}
	defer observability.SyncLogger()

	observability.Log.Info("Starting batch-processing middleware...")

	// Init DB
	db, err := database.Connect(cfg)
	if err != nil {
		observability.Log.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	// Init Repositories
	batchRepo := repository.NewBatchRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)
	idempotencyRepo := repository.NewIdempotencyRepository(db)
	dlqRepo := repository.NewDeadLetterRepository(db)

	// Init Services
	systemCClient := service.NewSystemCClient(cfg)
	batchService := service.NewBatchService(batchRepo, idempotencyRepo)

	workerPool := service.NewWorkerPool(cfg, batchRepo, outboxRepo, dlqRepo, systemCClient)
	workerPool.Start()
	defer workerPool.Stop()

	webhookDispatcher := service.NewWebhookDispatcher(cfg, outboxRepo, batchRepo)
	webhookDispatcher.Start()
	defer webhookDispatcher.Stop()

	// Init HTTP
	handler := internalhttp.NewHandler(batchService)
	router := internalhttp.NewRouter(cfg, handler)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: router,
	}

	// Start server in background
	go func() {
		observability.Log.Info("HTTP server listening", zap.Int("port", cfg.Server.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			observability.Log.Fatal("HTTP server failed", zap.Error(err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	observability.Log.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		observability.Log.Fatal("Server forced to shutdown", zap.Error(err))
	}

	observability.Log.Info("Server exiting")
}
