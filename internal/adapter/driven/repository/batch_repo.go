package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/semmidev/batch-processing/internal/domain"
	"github.com/semmidev/batch-processing/internal/port/output"
)

type batchRepo struct {
	db *sqlx.DB
}

// NewBatchRepository creates a new batchRepo instance implementing output.BatchRepository.
func NewBatchRepository(db *sqlx.DB) output.BatchRepository {
	return &batchRepo{db: db}
}

func (r *batchRepo) CreateBatch(ctx context.Context, batch *domain.Batch) error {
	query := `INSERT INTO batches (id, correlation_id, source_system, status, total_items, webhook_url, webhook_secret, idempotency_key, created_at, updated_at)
              VALUES (:id, :correlation_id, :source_system, :status, :total_items, :webhook_url, :webhook_secret, :idempotency_key, :created_at, :updated_at)`
	_, err := r.db.NamedExecContext(ctx, query, batch)
	return err
}

func (r *batchRepo) UpdateBatchStatus(ctx context.Context, batchID uuid.UUID, status domain.BatchStatus) error {
	query := `UPDATE batches SET status = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, status, batchID)
	return err
}

func (r *batchRepo) IncrementProcessedItems(ctx context.Context, batchID uuid.UUID) error {
	query := `UPDATE batches SET processed_items = processed_items + 1, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, batchID)
	return err
}

func (r *batchRepo) IncrementFailedItems(ctx context.Context, batchID uuid.UUID) error {
	query := `UPDATE batches SET failed_items = failed_items + 1, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, batchID)
	return err
}

func (r *batchRepo) GetBatch(ctx context.Context, batchID uuid.UUID) (*domain.Batch, error) {
	var batch domain.Batch
	query := `SELECT * FROM batches WHERE id = $1`
	err := r.db.GetContext(ctx, &batch, query, batchID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &batch, err
}

func (r *batchRepo) GetBatchByCorrelationID(ctx context.Context, correlationID string) (*domain.Batch, error) {
	var batch domain.Batch
	query := `SELECT * FROM batches WHERE correlation_id = $1 AND status IN ('pending', 'processing')`
	err := r.db.GetContext(ctx, &batch, query, correlationID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &batch, err
}

func (r *batchRepo) BulkInsertItems(ctx context.Context, items []domain.BatchItem) error {
	if len(items) == 0 {
		return nil
	}
	query := `INSERT INTO batch_items (batch_id, external_id, payload, status, max_retries, created_at, updated_at)
              VALUES (:batch_id, :external_id, :payload, :status, :max_retries, :created_at, :updated_at)`
	_, err := r.db.NamedExecContext(ctx, query, items)
	return err
}

func (r *batchRepo) GetPendingItems(ctx context.Context, workerID string, limit int) ([]domain.BatchItem, error) {
	query := `
		UPDATE batch_items
		SET status = 'processing',
			locked_by = $2,
			locked_at = NOW(),
			updated_at = NOW()
		WHERE id IN (
			SELECT id FROM batch_items 
			WHERE status IN ('pending', 'failed')
			  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
			ORDER BY id
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		RETURNING id, batch_id, external_id, payload, status, retry_count, max_retries, result_payload, error_message, locked_by, locked_at, next_retry_at, created_at, updated_at
    `
	var items []domain.BatchItem
	err := r.db.SelectContext(ctx, &items, query, limit, workerID)
	return items, err
}

func (r *batchRepo) UpdateItemStatus(ctx context.Context, itemID uuid.UUID, status domain.ItemStatus, resultPayload, errorMsg sql.NullString, retryCount int, nextRetryAt sql.NullTime) error {
	query := `
		UPDATE batch_items
		SET status = $1,
			result_payload = $2,
			error_message = $3,
			retry_count = $4,
			next_retry_at = $5,
			updated_at = NOW(),
			locked_by = NULL,
			locked_at = NULL
		WHERE id = $6
	`
	_, err := r.db.ExecContext(ctx, query, status, resultPayload, errorMsg, retryCount, nextRetryAt, itemID)
	return err
}

func (r *batchRepo) GetBatchItemsStatus(ctx context.Context, batchID uuid.UUID) (total, success, failed int, failedItems []domain.BatchItem, err error) {
	var batch domain.Batch
	err = r.db.GetContext(ctx, &batch, `SELECT total_items, processed_items, failed_items FROM batches WHERE id = $1`, batchID)
	if err != nil {
		return 0, 0, 0, nil, err
	}

	err = r.db.SelectContext(ctx, &failedItems, `SELECT external_id, error_message FROM batch_items WHERE batch_id = $1 AND status = 'failed'`, batchID)
	if err != nil {
		return 0, 0, 0, nil, err
	}

	return batch.TotalItems, batch.ProcessedItems, batch.FailedItems, failedItems, nil
}

func (r *batchRepo) CompleteBatch(ctx context.Context, batchID uuid.UUID, status domain.BatchStatus, completedAt time.Time) error {
	query := `UPDATE batches SET status = $1, completed_at = $2, updated_at = NOW() WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, status, completedAt, batchID)
	return err
}

func (r *batchRepo) ResetStaleLocks(ctx context.Context, staleThreshold time.Duration) error {
	query := `
		UPDATE batch_items
		SET status = 'pending',
			locked_by = NULL,
			locked_at = NULL,
			retry_count = retry_count + 1,
			next_retry_at = NOW() + INTERVAL '1 minute'
		WHERE status = 'processing'
		  AND locked_at < NOW() - make_interval(secs => $1)
	`
	_, err := r.db.ExecContext(ctx, query, staleThreshold.Seconds())
	return err
}

func (r *batchRepo) CancelBatch(ctx context.Context, batchID uuid.UUID) error {
	// Cancel the batch and all its pending items in a transaction
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Update batch
	_, err = tx.ExecContext(ctx, `UPDATE batches SET status = 'cancelled', updated_at = NOW() WHERE id = $1 AND status IN ('pending', 'processing')`, batchID)
	if err != nil {
		return err
	}

	// Update pending items
	_, err = tx.ExecContext(ctx, `UPDATE batch_items SET status = 'cancelled', updated_at = NOW() WHERE batch_id = $1 AND status = 'pending'`, batchID)
	if err != nil {
		return err
	}

	return tx.Commit()
}
