package output

import (
	"context"

	"github.com/semmidev/batch-processing/internal/domain"
)

// IdempotencyRepository defines the output port (driven boundary) for accessing and checking idempotency keys.
type IdempotencyRepository interface {
	Get(ctx context.Context, key string) (*domain.IdempotencyKey, error)
	Create(ctx context.Context, idempotencyKey *domain.IdempotencyKey) error
	UpdateResponse(ctx context.Context, key string, responseCache string) error
}
