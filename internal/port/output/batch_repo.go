package output

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/semmidev/batch-processing/internal/domain"
)

// BatchRepository defines the output port (driven boundary) for interacting with persistent batch storage.
type BatchRepository interface {
	CreateBatch(ctx context.Context, batch *domain.Batch) error
	UpdateBatchStatus(ctx context.Context, batchID uuid.UUID, status domain.BatchStatus) error
	IncrementProcessedItems(ctx context.Context, batchID uuid.UUID) error
	IncrementFailedItems(ctx context.Context, batchID uuid.UUID) error
	GetBatch(ctx context.Context, batchID uuid.UUID) (*domain.Batch, error)
	GetBatchByCorrelationID(ctx context.Context, correlationID string) (*domain.Batch, error)
	BulkInsertItems(ctx context.Context, items []domain.BatchItem) error
	GetPendingItems(ctx context.Context, workerID string, limit int) ([]domain.BatchItem, error)
	UpdateItemStatus(ctx context.Context, itemID uuid.UUID, status domain.ItemStatus, resultPayload, errorMsg sql.NullString, retryCount int, nextRetryAt sql.NullTime) error
	GetBatchItemsStatus(ctx context.Context, batchID uuid.UUID) (total, success, failed int, failedItems []domain.BatchItem, err error)
	CompleteBatch(ctx context.Context, batchID uuid.UUID, status domain.BatchStatus, completedAt time.Time) error
	ResetStaleLocks(ctx context.Context, staleThreshold time.Duration) error
	CancelBatch(ctx context.Context, batchID uuid.UUID) error
}
