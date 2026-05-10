DROP INDEX IF EXISTS idx_stores_document;

ALTER TABLE stores
    DROP COLUMN IF EXISTS document_verified_by,
    DROP COLUMN IF EXISTS document_verified_at,
    DROP COLUMN IF EXISTS legal_name,
    DROP COLUMN IF EXISTS document_status,
    DROP COLUMN IF EXISTS document_number,
    DROP COLUMN IF EXISTS document_type;

DROP TYPE IF EXISTS document_status;
DROP TYPE IF EXISTS document_type;
