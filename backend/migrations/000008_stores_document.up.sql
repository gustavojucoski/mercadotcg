CREATE TYPE document_type   AS ENUM ('cpf', 'cnpj');
CREATE TYPE document_status AS ENUM ('pending', 'auto_verified', 'manually_verified');

ALTER TABLE stores
    ADD COLUMN document_type       document_type,
    ADD COLUMN document_number     VARCHAR(14),
    ADD COLUMN document_status     document_status NOT NULL DEFAULT 'pending',
    ADD COLUMN legal_name          VARCHAR(255),
    ADD COLUMN document_verified_at TIMESTAMPTZ,
    ADD COLUMN document_verified_by UUID REFERENCES users(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX idx_stores_document
    ON stores(document_type, document_number)
    WHERE document_number IS NOT NULL;
