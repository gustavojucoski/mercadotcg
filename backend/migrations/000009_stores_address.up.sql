ALTER TABLE stores
    ADD COLUMN trade_name            VARCHAR(128),
    ADD COLUMN phone                 VARCHAR(32),
    ADD COLUMN address_zip           VARCHAR(8),
    ADD COLUMN address_street        TEXT,
    ADD COLUMN address_number        VARCHAR(16),
    ADD COLUMN address_complement    VARCHAR(128),
    ADD COLUMN address_neighborhood  VARCHAR(128),
    ADD COLUMN address_city          VARCHAR(128),
    ADD COLUMN address_state         CHAR(2),
    ADD COLUMN address_country       CHAR(2) NOT NULL DEFAULT 'BR';
