CREATE TABLE batch_items (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    batch_id UUID NOT NULL REFERENCES batches(id),
    external_id VARCHAR(100) NOT NULL,
    payload TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','processing','done','failed','cancelled')),
    retry_count SMALLINT NOT NULL DEFAULT 0,
    max_retries SMALLINT NOT NULL DEFAULT 3,
    result_payload TEXT NULL,
    error_message VARCHAR(1000) NULL,
    locked_by VARCHAR(100) NULL,
    locked_at TIMESTAMP WITH TIME ZONE NULL,
    next_retry_at TIMESTAMP WITH TIME ZONE NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IX_batch_items_batch_id ON batch_items(batch_id);
CREATE INDEX IX_batch_items_worker_poll 
    ON batch_items(status, next_retry_at, locked_at) 
    WHERE status IN ('pending', 'failed');
