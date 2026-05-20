package application

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/semmidev/batch-processing/internal/domain"
	"github.com/semmidev/batch-processing/internal/observability"
	"github.com/semmidev/batch-processing/internal/port/input"
	"github.com/semmidev/batch-processing/internal/port/output"
	"github.com/semmidev/batch-processing/internal/uuid"
	"go.uber.org/zap"
)

type batchService struct {
	batchRepo       output.BatchRepository
	idempotencyRepo output.IdempotencyRepository
}

// NewBatchService creates a new batchService implementing input.BatchUseCase.
func NewBatchService(batchRepo output.BatchRepository, idempotencyRepo output.IdempotencyRepository) input.BatchUseCase {
	return &batchService{
		batchRepo:       batchRepo,
		idempotencyRepo: idempotencyRepo,
	}
}

func (s *batchService) SubmitBatch(ctx context.Context, idempotencyKey string, req domain.SubmitBatchRequest) (uuid.UUID, error) {
	// Idempotency check
	if idempotencyKey != "" {
		existing, err := s.idempotencyRepo.Get(ctx, idempotencyKey)
		if err != nil {
			return uuid.Nil, fmt.Errorf("error checking idempotency: %w", err)
		}
		if existing != nil {
			return existing.BatchID, nil
		}
	}

	batchID := uuid.New()
	now := time.Now().UTC()

	batch := &domain.Batch{
		ID:             batchID,
		CorrelationID:  req.CorrelationID,
		SourceSystem:   "SystemA", // Could be dynamic from auth context
		Status:         domain.BatchStatusPending,
		TotalItems:     len(req.Items),
		ProcessedItems: 0,
		FailedItems:    0,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if req.WebhookURL != "" {
		batch.WebhookURL = sql.NullString{String: req.WebhookURL, Valid: true}
		batch.WebhookSecret = sql.NullString{String: "some-generated-secret", Valid: true}
	}

	// Create Batch
	err := s.batchRepo.CreateBatch(ctx, batch)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to create batch: %w", err)
	}

	// Bulk Insert Items
	items := make([]domain.BatchItem, 0, len(req.Items))
	for _, it := range req.Items {
		items = append(items, domain.BatchItem{
			BatchID:    batchID,
			ExternalID: it.ExternalID,
			Payload:    string(it.Payload),
			Status:     domain.ItemStatusPending,
			MaxRetries: 3,
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}

	err = s.batchRepo.BulkInsertItems(ctx, items)
	if err != nil {
		observability.Log.Error("failed to insert batch items", zap.Error(err), zap.String("batch_id", batchID.String()))
		return uuid.Nil, fmt.Errorf("failed to create batch items: %w", err)
	}

	// Save idempotency key
	if idempotencyKey != "" {
		idempotencyRec := &domain.IdempotencyKey{
			KeyValue:  idempotencyKey,
			BatchID:   batchID,
			CreatedAt: now,
			ExpiresAt: now.Add(24 * time.Hour), // Expire after 1 day
		}
		if err := s.idempotencyRepo.Create(ctx, idempotencyRec); err != nil {
			observability.Log.Error("failed to save idempotency key", zap.Error(err))
		}
	}

	// Record metrics
	observability.BatchesReceived.WithLabelValues("SystemA").Inc()
	observability.ItemsPending.Add(float64(len(items)))

	return batchID, nil
}

func (s *batchService) GetBatchStatus(ctx context.Context, batchID uuid.UUID) (*domain.BatchStatusResponse, error) {
	total, success, failed, failedItems, err := s.batchRepo.GetBatchItemsStatus(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("failed to get batch items status: %w", err)
	}

	batch, err := s.batchRepo.GetBatch(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("failed to get batch: %w", err)
	}
	if batch == nil {
		return nil, nil // Not found
	}

	res := &domain.BatchStatusResponse{
		BatchID:        batch.ID.String(),
		Status:         string(batch.Status),
		TotalItems:     total,
		ProcessedItems: success,
		FailedItems:    failed,
	}

	if batch.CompletedAt.Valid {
		res.CompletedAt = &batch.CompletedAt.Time
	}

	for _, fi := range failedItems {
		res.FailedItemsList = append(res.FailedItemsList, domain.FailedItemInfo{
			ExternalID: fi.ExternalID,
			Error:      fi.ErrorMessage.String,
		})
	}

	return res, nil
}

func (s *batchService) CancelBatch(ctx context.Context, batchID uuid.UUID) error {
	batch, err := s.batchRepo.GetBatch(ctx, batchID)
	if err != nil {
		return fmt.Errorf("failed to check batch existence: %w", err)
	}
	if batch == nil {
		return fmt.Errorf("batch not found")
	}

	if batch.Status != domain.BatchStatusPending && batch.Status != domain.BatchStatusProcessing {
		return fmt.Errorf("cannot cancel batch in state: %s", batch.Status)
	}

	return s.batchRepo.CancelBatch(ctx, batchID)
}
