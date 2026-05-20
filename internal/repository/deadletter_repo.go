package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/semmidev/batch-processing/internal/domain"
)

type DeadLetterRepository interface {
	Insert(ctx context.Context, item *domain.DeadLetterQueue) error
}

type deadLetterRepo struct {
	db *sqlx.DB
}

func NewDeadLetterRepository(db *sqlx.DB) DeadLetterRepository {
	return &deadLetterRepo{db: db}
}

func (r *deadLetterRepo) Insert(ctx context.Context, item *domain.DeadLetterQueue) error {
	query := `INSERT INTO dead_letter_queue (source_table, source_id, payload, failure_reason, retry_count, created_at)
              VALUES (:source_table, :source_id, :payload, :failure_reason, :retry_count, :created_at)`
	_, err := r.db.NamedExecContext(ctx, query, item)
	return err
}
