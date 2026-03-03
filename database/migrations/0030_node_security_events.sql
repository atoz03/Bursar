CREATE TABLE IF NOT EXISTS node_security_events (
    event_id BIGSERIAL PRIMARY KEY,
    report_id TEXT NOT NULL DEFAULT '',
    node_id VARCHAR(50) NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    severity VARCHAR(20) NOT NULL DEFAULT 'info',
    reason TEXT NOT NULL DEFAULT '',
    related_usernames TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_node_security_events_node_time
    ON node_security_events(node_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_node_security_events_type_time
    ON node_security_events(event_type, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_node_security_events_users
    ON node_security_events USING GIN (related_usernames);
