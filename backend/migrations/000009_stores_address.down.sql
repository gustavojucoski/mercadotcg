ALTER TABLE stores
    DROP COLUMN IF EXISTS trade_name,
    DROP COLUMN IF EXISTS phone,
    DROP COLUMN IF EXISTS address_zip,
    DROP COLUMN IF EXISTS address_street,
    DROP COLUMN IF EXISTS address_number,
    DROP COLUMN IF EXISTS address_complement,
    DROP COLUMN IF EXISTS address_neighborhood,
    DROP COLUMN IF EXISTS address_city,
    DROP COLUMN IF EXISTS address_state,
    DROP COLUMN IF EXISTS address_country;
