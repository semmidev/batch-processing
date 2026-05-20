package input

import (
	"context"

	"github.com/google/uuid"
	"github.com/semmidev/batch-processing/internal/domain"
)

// BatchUseCase is the input port (driving boundary) for the Batch Use Cases.
type BatchUseCase interface {
	SubmitBatch(ctx context.Context, idempotencyKey string, req domain.SubmitBatchRequest) (uuid.UUID, error)
	GetBatchStatus(ctx context.Context, batchID uuid.UUID) (*domain.BatchStatusResponse, error)
	CancelBatch(ctx context.Context, batchID uuid.UUID) error
}
