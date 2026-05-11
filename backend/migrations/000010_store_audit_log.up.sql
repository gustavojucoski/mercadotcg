CREATE TABLE store_audit_log (
    id          UUID        NOT NULL DEFAULT gen_random_uuid(),
    store_id    UUID        NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    changed_by  UUID        NOT NULL REFERENCES users(id),
    change_type VARCHAR(64) NOT NULL DEFAULT 'update',
    changes     JSONB       NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id)
);

CREATE INDEX store_audit_log_store_idx
    ON store_audit_log(store_id, created_at DESC);
