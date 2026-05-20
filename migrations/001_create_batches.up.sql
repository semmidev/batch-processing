CREATE TABLE batches (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    correlation_id VARCHAR(100) NOT NULL,
    source_system VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL 
        CHECK (status IN ('pending','processing','done','partial','failed', 'cancelled')),
    total_items INT NOT NULL DEFAULT 0,
    processed_items INT NOT NULL DEFAULT 0,
    failed_items INT NOT NULL DEFAULT 0,
    webhook_url VARCHAR(500) NULL,
    webhook_secret VARCHAR(255) NULL,
    idempotency_key VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE NULL
);

CREATE INDEX IX_batches_correlation_id ON batches(correlation_id);
CREATE INDEX IX_batches_status ON batches(status) WHERE status IN ('pending','processing');
