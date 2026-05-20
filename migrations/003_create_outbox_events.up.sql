CREATE TABLE outbox_events (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    aggregate_id VARCHAR(100) NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    payload TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','sent','failed','processing')),
    retry_count SMALLINT NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMP WITH TIME ZONE NULL
);

CREATE INDEX IX_outbox_events_pending 
    ON outbox_events(status, next_retry_at) 
    WHERE status = 'pending' OR status = 'failed';
