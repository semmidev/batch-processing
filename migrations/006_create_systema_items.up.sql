-- System A's own staging table.
-- Items are inserted here first (by the seeder / trigger endpoint),
-- then drained to the middleware by the sender loop.
-- Failed items are automatically re-queued with exponential back-off.

CREATE TABLE systema_items (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_key      VARCHAR(100)  NOT NULL,          -- local grouping key (correlation-id prefix)
    external_id    VARCHAR(150)  NOT NULL UNIQUE,   -- globally unique item identifier
    payload        TEXT          NOT NULL,           -- JSON payload to forward
    status         VARCHAR(20)   NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending','sending','sent','failed','exhausted')),
    retry_count    SMALLINT      NOT NULL DEFAULT 0,
    max_retries    SMALLINT      NOT NULL DEFAULT 5,
    last_error     TEXT          NULL,
    next_retry_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at     TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Sender loop polls this index to find work
CREATE INDEX IX_systema_items_pending
    ON systema_items(next_retry_at, status)
    WHERE status IN ('pending', 'failed');

-- Useful for per-batch reporting
CREATE INDEX IX_systema_items_batch_key
    ON systema_items(batch_key);
