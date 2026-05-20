package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/semmidev/batch-processing/internal/domain"
	"github.com/semmidev/batch-processing/internal/port/output"
)

type outboxRepo struct {
	db *sqlx.DB
}

// NewOutboxRepository creates a new outboxRepo implementing output.OutboxRepository.
func NewOutboxRepository(db *sqlx.DB) output.OutboxRepository {
	return &outboxRepo{db: db}
}

func (r *outboxRepo) CreateEvent(ctx context.Context, event *domain.OutboxEvent) error {
	query := `INSERT INTO outbox_events (aggregate_id, event_type, payload, status, retry_count, next_retry_at, created_at)
              VALUES (:aggregate_id, :event_type, :payload, :status, :retry_count, :next_retry_at, :created_at)`
	_, err := r.db.NamedExecContext(ctx, query, event)
	return err
}

func (r *outboxRepo) GetPendingEvents(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	// Simple polling with FOR UPDATE SKIP LOCKED
	query := `
        UPDATE outbox_events
        SET status = 'processing'
        WHERE id IN (
            SELECT id FROM outbox_events
            WHERE status IN ('pending', 'failed')
              AND next_retry_at <= NOW()
            ORDER BY id
            FOR UPDATE SKIP LOCKED
            LIMIT $1
        )
        RETURNING id, aggregate_id, event_type, payload, status, retry_count, next_retry_at, created_at, processed_at
    `
	var events []domain.OutboxEvent
	err := r.db.SelectContext(ctx, &events, query, limit)
	return events, err
}

func (r *outboxRepo) MarkEventAsProcessed(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE outbox_events SET status = 'processed', processed_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *outboxRepo) MarkEventAsFailed(ctx context.Context, id uuid.UUID, retryCount int, nextRetryAt time.Time) error {
	query := `UPDATE outbox_events SET status = 'failed', retry_count = $1, next_retry_at = $2 WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, retryCount, nextRetryAt, id)
	return err
}
