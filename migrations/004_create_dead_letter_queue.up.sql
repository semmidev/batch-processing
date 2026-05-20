CREATE TABLE dead_letter_queue (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    source_table VARCHAR(50) NOT NULL,
    source_id VARCHAR(100) NOT NULL,
    payload TEXT NOT NULL,
    failure_reason VARCHAR(1000) NOT NULL,
    retry_count SMALLINT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
