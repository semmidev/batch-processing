package worker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/semmidev/batch-processing/internal/config"
	"github.com/semmidev/batch-processing/internal/domain"
	"github.com/semmidev/batch-processing/internal/observability"
	"github.com/semmidev/batch-processing/internal/port/output"
	"go.uber.org/zap"
)

type WebhookDispatcher struct {
	cfg        *config.Config
	outboxRepo output.OutboxRepository
	batchRepo  output.BatchRepository
	client     *http.Client
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

func NewWebhookDispatcher(cfg *config.Config, outboxRepo output.OutboxRepository, batchRepo output.BatchRepository) *WebhookDispatcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &WebhookDispatcher{
		cfg:        cfg,
		outboxRepo: outboxRepo,
		batchRepo:  batchRepo,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

func (wd *WebhookDispatcher) Start() {
	observability.Log.Info("starting webhook dispatcher")
	wd.wg.Add(1)
	go wd.poll()
}

func (wd *WebhookDispatcher) Stop() {
	observability.Log.Info("stopping webhook dispatcher...")
	wd.cancel()
	wd.wg.Wait()
	observability.Log.Info("webhook dispatcher stopped")
}

func (wd *WebhookDispatcher) poll() {
	defer wd.wg.Done()
	ticker := time.NewTicker(time.Duration(wd.cfg.Worker.PollIntervalSecs) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-wd.ctx.Done():
			return
		case <-ticker.C:
			wd.processPendingEvents()
		}
	}
}

func (wd *WebhookDispatcher) processPendingEvents() {
	events, err := wd.outboxRepo.GetPendingEvents(wd.ctx, wd.cfg.Worker.ItemsPerPoll)
	if err != nil {
		observability.Log.Error("failed to fetch outbox events", zap.Error(err))
		return
	}

	for _, event := range events {
		wd.dispatch(event)
	}
}

func (wd *WebhookDispatcher) dispatch(event domain.OutboxEvent) {
	start := time.Now()

	// Parse event payload to get batch ID
	var payloadMap map[string]interface{}
	if err := json.Unmarshal([]byte(event.Payload), &payloadMap); err != nil {
		observability.Log.Error("failed to unmarshal event payload", zap.Error(err))
		wd.outboxRepo.MarkEventAsFailed(wd.ctx, event.ID, event.RetryCount+1, time.Now().Add(5*time.Minute))
		return
	}

	batchIDStr, ok := payloadMap["batch_id"].(string)
	if !ok {
		observability.Log.Error("batch_id missing from event payload")
		wd.outboxRepo.MarkEventAsFailed(wd.ctx, event.ID, event.RetryCount+1, time.Now().Add(5*time.Minute))
		return
	}

	batchID, err := uuid.Parse(batchIDStr)
	if err != nil {
		observability.Log.Error("invalid batch_id in event payload", zap.Error(err))
		wd.outboxRepo.MarkEventAsFailed(wd.ctx, event.ID, event.RetryCount+1, time.Now().Add(5*time.Minute))
		return
	}

	// Fetch full batch status
	batch, err := wd.batchRepo.GetBatch(wd.ctx, batchID)
	if err != nil || batch == nil {
		observability.Log.Error("failed to fetch batch for webhook", zap.Error(err))
		wd.outboxRepo.MarkEventAsFailed(wd.ctx, event.ID, event.RetryCount+1, time.Now().Add(1*time.Minute))
		return
	}

	if !batch.WebhookURL.Valid || batch.WebhookURL.String == "" {
		// No webhook configured, just mark as processed
		wd.outboxRepo.MarkEventAsProcessed(wd.ctx, event.ID)
		return
	}

	total, success, failed, failedItems, _ := wd.batchRepo.GetBatchItemsStatus(wd.ctx, batchID)

	var webhookFailedItems []domain.WebhookFailedItem
	for _, fi := range failedItems {
		webhookFailedItems = append(webhookFailedItems, domain.WebhookFailedItem{
			ExternalID: fi.ExternalID,
			Error:      fi.ErrorMessage.String,
		})
	}

	webhookPayload := domain.WebhookPayload{
		Event:         event.EventType,
		BatchID:       batch.ID.String(),
		CorrelationID: batch.CorrelationID,
		Status:        string(batch.Status),
		Summary: domain.WebhookSummary{
			Total:   total,
			Success: success,
			Failed:  failed,
		},
		FailedItems: webhookFailedItems,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	payloadBytes, _ := json.Marshal(webhookPayload)

	req, err := http.NewRequestWithContext(wd.ctx, http.MethodPost, batch.WebhookURL.String, bytes.NewReader(payloadBytes))
	if err != nil {
		observability.Log.Error("failed to create webhook request", zap.Error(err))
		wd.outboxRepo.MarkEventAsFailed(wd.ctx, event.ID, event.RetryCount+1, time.Now().Add(1*time.Minute))
		return
	}

	req.Header.Set("Content-Type", "application/json")

	// Add signature if secret exists
	if batch.WebhookSecret.Valid && batch.WebhookSecret.String != "" {
		mac := hmac.New(sha256.New, []byte(batch.WebhookSecret.String))
		mac.Write(payloadBytes)
		signature := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Signature", fmt.Sprintf("sha256=%s", signature))
	}

	res, err := wd.client.Do(req)
	duration := time.Since(start)

	var status string
	if err != nil || res.StatusCode >= 400 {
		status = "failed"
		observability.Log.Warn("webhook delivery failed", zap.Error(err), zap.String("url", batch.WebhookURL.String))

		if event.RetryCount < wd.cfg.Webhook.MaxRetries {
			nextRetry := time.Now().Add(time.Duration(1<<event.RetryCount) * time.Minute)
			wd.outboxRepo.MarkEventAsFailed(wd.ctx, event.ID, event.RetryCount+1, nextRetry)
		} else {
			// Max retries reached, could move to dead letter queue, but for now just mark processed/failed permanently
			wd.outboxRepo.MarkEventAsProcessed(wd.ctx, event.ID) // Or a specific permanent failure status
		}
	} else {
		status = "success"
		wd.outboxRepo.MarkEventAsProcessed(wd.ctx, event.ID)
	}

	if res != nil && res.Body != nil {
		res.Body.Close()
	}

	observability.WebhookDeliveryDuration.Observe(duration.Seconds())
	observability.WebhooksSent.WithLabelValues(status).Inc()
}
