CREATE TABLE idempotency_keys (
    key_value VARCHAR(255) PRIMARY KEY,
    batch_id UUID NOT NULL,
    response_cache TEXT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL
);
