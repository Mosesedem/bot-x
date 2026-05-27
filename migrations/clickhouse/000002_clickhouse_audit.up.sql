CREATE TABLE IF NOT EXISTS audit_events (
    event_id        UUID,
    entity_type     LowCardinality(String),
    entity_id       String,
    action          LowCardinality(String),
    actor_id        String,
    gateway         LowCardinality(String),
    payload         String,
    ip_address      String,
    created_at      DateTime64(3)
) ENGINE = MergeTree()
ORDER BY (created_at, entity_type, entity_id);
