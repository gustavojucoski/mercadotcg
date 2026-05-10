-- Extensões necessárias do PostgreSQL/Supabase
-- pgcrypto: gen_random_uuid()
-- citext:    busca case-insensitive por nome de carta
-- pg_trgm:   busca aproximada (typo tolerance) em nomes
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "citext";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";
