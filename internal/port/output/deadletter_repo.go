package output

import (
	"context"

	"github.com/semmidev/batch-processing/internal/domain"
)

// DeadLetterRepository defines the output port (driven boundary) for inserting failed items into the dead letter queue.
type DeadLetterRepository interface {
	Insert(ctx context.Context, item *domain.DeadLetterQueue) error
}
