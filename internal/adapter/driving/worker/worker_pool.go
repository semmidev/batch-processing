package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"time"

	"github.com/semmidev/batch-processing/internal/config"
	"github.com/semmidev/batch-processing/internal/domain"
	"github.com/semmidev/batch-processing/internal/observability"
	"github.com/semmidev/batch-processing/internal/port/output"
	"github.com/semmidev/batch-processing/internal/uuid"
	"go.uber.org/zap"
)

type WorkerPool struct {
	cfg        *config.Config
	batchRepo  output.BatchRepository
	outboxRepo output.OutboxRepository
	dlqRepo    output.DeadLetterRepository
	systemC    output.SystemCClient
	workerID   string
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewWorkerPool(cfg *config.Config, batchRepo output.BatchRepository, outboxRepo output.OutboxRepository, dlqRepo output.DeadLetterRepository, systemC output.SystemCClient) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerPool{
		cfg:        cfg,
		batchRepo:  batchRepo,
		outboxRepo: outboxRepo,
		dlqRepo:    dlqRepo,
		systemC:    systemC,
		workerID:   uuid.New().String(),
		ctx:        ctx,
		cancel:     cancel,
	}
}

func (wp *WorkerPool) Start() {
	observability.Log.Info("starting worker pool", zap.Int("workers", wp.cfg.Worker.Count))

	for i := 0; i < wp.cfg.Worker.Count; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}

	wp.wg.Add(1)
	go wp.runStaleLockCleaner()
}

func (wp *WorkerPool) Stop() {
	observability.Log.Info("stopping worker pool...")
	wp.cancel()
	wp.wg.Wait()
	observability.Log.Info("worker pool stopped")
}

func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()
	observability.WorkerActive.WithLabelValues("batch_worker").Inc()
	defer observability.WorkerActive.WithLabelValues("batch_worker").Dec()

	ticker := time.NewTicker(time.Duration(wp.cfg.Worker.PollIntervalSecs) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-wp.ctx.Done():
			return
		case <-ticker.C:
			wp.processPendingItems()
		}
	}
}

func (wp *WorkerPool) runStaleLockCleaner() {
	defer wp.wg.Done()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-wp.ctx.Done():
			return
		case <-ticker.C:
			err := wp.batchRepo.ResetStaleLocks(wp.ctx, 10*time.Minute)
			if err != nil {
				observability.Log.Error("failed to reset stale locks", zap.Error(err))
			} else {
				observability.Log.Debug("stale locks check completed")
			}
		}
	}
}

func (wp *WorkerPool) processPendingItems() {
	items, err := wp.batchRepo.GetPendingItems(wp.ctx, wp.workerID, wp.cfg.Worker.ItemsPerPoll)
	if err != nil {
		observability.Log.Error("failed to get pending items", zap.Error(err))
		return
	}

	if len(items) == 0 {
		return // Nothing to do
	}

	observability.ItemsPending.Sub(float64(len(items)))
	observability.ItemsProcessing.Add(float64(len(items)))

	for _, item := range items {
		wp.processItem(item)
	}

	observability.ItemsProcessing.Sub(float64(len(items)))
}

func (wp *WorkerPool) processItem(item domain.BatchItem) {
	start := time.Now()

	resBytes, err := wp.systemC.SendItem(wp.ctx, item.ExternalID, item.Payload)
	duration := time.Since(start)

	status := domain.ItemStatusDone
	var errMsg sql.NullString
	var resMsg sql.NullString
	var nextRetryAt sql.NullTime

	if err != nil {
		status = domain.ItemStatusFailed
		errMsg = sql.NullString{String: err.Error(), Valid: true}
		if item.RetryCount < item.MaxRetries {
			status = domain.ItemStatusPending                                                                              // Retry
			nextRetryAt = sql.NullTime{Time: time.Now().Add(time.Duration(1<<item.RetryCount) * time.Second), Valid: true} // Exponential backoff
		} else {
			// Max retries exhausted, send to DLQ
			status = domain.ItemStatusFailed
			dlqItem := &domain.DeadLetterQueue{
				SourceTable:   "batch_items",
				SourceID:      item.ID.String(),
				Payload:       item.Payload,
				FailureReason: err.Error(),
				RetryCount:    item.RetryCount + 1,
				CreatedAt:     time.Now().UTC(),
			}
			if dlqErr := wp.dlqRepo.Insert(wp.ctx, dlqItem); dlqErr != nil {
				observability.Log.Error("failed to insert into dead letter queue", zap.Error(dlqErr), zap.String("item_id", item.ID.String()))
			}
		}
		observability.Log.Error("failed to process item", zap.Error(err), zap.String("item_id", item.ID.String()))
	} else {
		resMsg = sql.NullString{String: string(resBytes), Valid: true}
	}

	// Update Item
	err = wp.batchRepo.UpdateItemStatus(wp.ctx, item.ID, status, resMsg, errMsg, item.RetryCount+1, nextRetryAt)
	if err != nil {
		observability.Log.Error("failed to update item status", zap.Error(err), zap.String("item_id", item.ID.String()))
		return
	}

	observability.ItemProcessingDuration.WithLabelValues(string(status)).Observe(duration.Seconds())
	observability.ItemsProcessed.WithLabelValues(string(status)).Inc()

	if status == domain.ItemStatusDone || status == domain.ItemStatusFailed {
		wp.checkBatchCompletion(item.BatchID)
	}
}

func (wp *WorkerPool) checkBatchCompletion(batchID uuid.UUID) {
	total, success, failed, _, err := wp.batchRepo.GetBatchItemsStatus(wp.ctx, batchID)
	if err != nil {
		observability.Log.Error("failed to check batch items status", zap.Error(err), zap.String("batch_id", batchID.String()))
		return
	}

	if success+failed == total {
		// Batch is complete
		status := domain.BatchStatusDone
		if failed > 0 {
			if success > 0 {
				status = domain.BatchStatusPartial
			} else {
				status = domain.BatchStatusFailed
			}
		}

		err = wp.batchRepo.CompleteBatch(wp.ctx, batchID, status, time.Now())
		if err != nil {
			observability.Log.Error("failed to complete batch", zap.Error(err), zap.String("batch_id", batchID.String()))
			return
		}

		// Enqueue webhook event
		eventPayload, _ := json.Marshal(map[string]interface{}{
			"batch_id": batchID.String(),
			"status":   status,
		})

		outboxEvent := &domain.OutboxEvent{
			AggregateID: batchID.String(),
			EventType:   "batch_completed",
			Payload:     string(eventPayload),
			Status:      "pending",
			CreatedAt:   time.Now(),
		}

		if err := wp.outboxRepo.CreateEvent(wp.ctx, outboxEvent); err != nil {
			observability.Log.Error("failed to create outbox event for webhook", zap.Error(err))
		}
	}
}
