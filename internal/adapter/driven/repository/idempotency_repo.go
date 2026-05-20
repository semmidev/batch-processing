package repository

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
	"github.com/semmidev/batch-processing/internal/domain"
	"github.com/semmidev/batch-processing/internal/port/output"
)

type idempotencyRepo struct {
	db *sqlx.DB
}

// NewIdempotencyRepository creates a new idempotencyRepo implementing output.IdempotencyRepository.
func NewIdempotencyRepository(db *sqlx.DB) output.IdempotencyRepository {
	return &idempotencyRepo{db: db}
}

func (r *idempotencyRepo) Get(ctx context.Context, key string) (*domain.IdempotencyKey, error) {
	var result domain.IdempotencyKey
	query := `SELECT key_value, batch_id, response_cache, created_at, expires_at 
              FROM idempotency_keys WHERE key_value = $1`
	err := r.db.GetContext(ctx, &result, query, key)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &result, err
}

func (r *idempotencyRepo) Create(ctx context.Context, idempotencyKey *domain.IdempotencyKey) error {
	query := `INSERT INTO idempotency_keys (key_value, batch_id, response_cache, created_at, expires_at)
              VALUES (:key_value, :batch_id, :response_cache, :created_at, :expires_at)`
	_, err := r.db.NamedExecContext(ctx, query, idempotencyKey)
	return err
}

func (r *idempotencyRepo) UpdateResponse(ctx context.Context, key string, responseCache string) error {
	query := `UPDATE idempotency_keys SET response_cache = $1 WHERE key_value = $2`
	_, err := r.db.ExecContext(ctx, query, responseCache, key)
	return err
}
