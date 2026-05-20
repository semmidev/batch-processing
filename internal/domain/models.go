package domain

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type BatchStatus string

const (
	BatchStatusPending    BatchStatus = "pending"
	BatchStatusProcessing BatchStatus = "processing"
	BatchStatusDone       BatchStatus = "done"
	BatchStatusPartial    BatchStatus = "partial"
	BatchStatusFailed     BatchStatus = "failed"
)

type ItemStatus string

const (
	ItemStatusPending    ItemStatus = "pending"
	ItemStatusProcessing ItemStatus = "processing"
	ItemStatusDone       ItemStatus = "done"
	ItemStatusFailed     ItemStatus = "failed"
)

type Batch struct {
	ID             uuid.UUID      `db:"id"`
	CorrelationID  string         `db:"correlation_id"`
	SourceSystem   string         `db:"source_system"`
	Status         BatchStatus    `db:"status"`
	TotalItems     int            `db:"total_items"`
	ProcessedItems int            `db:"processed_items"`
	FailedItems    int            `db:"failed_items"`
	WebhookURL     sql.NullString `db:"webhook_url"`
	WebhookSecret  sql.NullString `db:"webhook_secret"`
	IdempotencyKey string         `db:"idempotency_key"`
	CreatedAt      time.Time      `db:"created_at"`
	UpdatedAt      time.Time      `db:"updated_at"`
	CompletedAt    sql.NullTime   `db:"completed_at"`
}

type BatchItem struct {
	ID            uuid.UUID      `db:"id"`
	BatchID       uuid.UUID      `db:"batch_id"`
	ExternalID    string         `db:"external_id"`
	Payload       string         `db:"payload"` // JSON
	Status        ItemStatus     `db:"status"`
	RetryCount    int            `db:"retry_count"`
	MaxRetries    int            `db:"max_retries"`
	ResultPayload sql.NullString `db:"result_payload"`
	ErrorMessage  sql.NullString `db:"error_message"`
	LockedBy      sql.NullString `db:"locked_by"`
	LockedAt      sql.NullTime   `db:"locked_at"`
	NextRetryAt   sql.NullTime   `db:"next_retry_at"`
	CreatedAt     time.Time      `db:"created_at"`
	UpdatedAt     time.Time      `db:"updated_at"`
}

type OutboxEvent struct {
	ID          uuid.UUID    `db:"id"`
	AggregateID string       `db:"aggregate_id"`
	EventType   string       `db:"event_type"`
	Payload     string       `db:"payload"`
	Status      string       `db:"status"`
	RetryCount  int          `db:"retry_count"`
	NextRetryAt time.Time    `db:"next_retry_at"`
	CreatedAt   time.Time    `db:"created_at"`
	ProcessedAt sql.NullTime `db:"processed_at"`
}

type DeadLetterQueue struct {
	ID            uuid.UUID `db:"id"`
	SourceTable   string    `db:"source_table"`
	SourceID      string    `db:"source_id"`
	Payload       string    `db:"payload"`
	FailureReason string    `db:"failure_reason"`
	RetryCount    int       `db:"retry_count"`
	CreatedAt     time.Time `db:"created_at"`
}

type IdempotencyKey struct {
	KeyValue      string         `db:"key_value"`
	BatchID       uuid.UUID      `db:"batch_id"`
	ResponseCache sql.NullString `db:"response_cache"`
	CreatedAt     time.Time      `db:"created_at"`
	ExpiresAt     time.Time      `db:"expires_at"`
}

// Request/Response DTOs
type SubmitBatchRequest struct {
	CorrelationID string             `json:"correlation_id" validate:"required"`
	WebhookURL    string             `json:"webhook_url" validate:"omitempty,url"`
	Items         []BatchItemRequest `json:"items" validate:"required,min=1,max=1000"`
}

type BatchItemRequest struct {
	ExternalID string          `json:"external_id" validate:"required"`
	Payload    json.RawMessage `json:"payload" validate:"required"`
}

type SubmitBatchResponse struct {
	BatchID string `json:"batch_id"`
	Message string `json:"message"`
}

type BatchStatusResponse struct {
	BatchID         string           `json:"batch_id"`
	Status          string           `json:"status"`
	TotalItems      int              `json:"total_items"`
	ProcessedItems  int              `json:"processed_items"`
	FailedItems     int              `json:"failed_items"`
	CompletedAt     *time.Time       `json:"completed_at,omitempty"`
	FailedItemsList []FailedItemInfo `json:"failed_items,omitempty"`
}

type FailedItemInfo struct {
	ExternalID string `json:"external_id"`
	Error      string `json:"error"`
}

type WebhookPayload struct {
	Event         string              `json:"event"`
	BatchID       string              `json:"batch_id"`
	CorrelationID string              `json:"correlation_id"`
	Status        string              `json:"status"`
	Summary       WebhookSummary      `json:"summary"`
	FailedItems   []WebhookFailedItem `json:"failed_items,omitempty"`
	Timestamp     string              `json:"timestamp"`
}

type WebhookSummary struct {
	Total   int `json:"total"`
	Success int `json:"success"`
	Failed  int `json:"failed"`
}

type WebhookFailedItem struct {
	ExternalID string `json:"external_id"`
	Error      string `json:"error"`
}
