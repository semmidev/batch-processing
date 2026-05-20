package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/semmidev/batch-processing/internal/domain"
	"github.com/semmidev/batch-processing/internal/port/output"
)

type deadLetterRepo struct {
	db *sqlx.DB
}

// NewDeadLetterRepository creates a new deadLetterRepo implementing output.DeadLetterRepository.
func NewDeadLetterRepository(db *sqlx.DB) output.DeadLetterRepository {
	return &deadLetterRepo{db: db}
}

func (r *deadLetterRepo) Insert(ctx context.Context, item *domain.DeadLetterQueue) error {
	query := `INSERT INTO dead_letter_queue (source_table, source_id, payload, failure_reason, retry_count, created_at)
              VALUES (:source_table, :source_id, :payload, :failure_reason, :retry_count, :created_at)`
	_, err := r.db.NamedExecContext(ctx, query, item)
	return err
}
