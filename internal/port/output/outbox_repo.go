package output

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/semmidev/batch-processing/internal/domain"
)

// OutboxRepository defines the output port (driven boundary) for managing and publishing outbox events.
type OutboxRepository interface {
	CreateEvent(ctx context.Context, event *domain.OutboxEvent) error
	GetPendingEvents(ctx context.Context, limit int) ([]domain.OutboxEvent, error)
	MarkEventAsProcessed(ctx context.Context, id uuid.UUID) error
	MarkEventAsFailed(ctx context.Context, id uuid.UUID, retryCount int, nextRetryAt time.Time) error
}
