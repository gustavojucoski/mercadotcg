-- Remove o sufixo _ja dos codes de sets japoneses e troca a constraint
-- de UNIQUE(code) por UNIQUE(code, language).
-- O identificador derivado para display/slug continua sendo code||'_'||language
-- para sets não-EN, calculado em runtime nas queries (não armazenado).

-- 1. Troca a constraint ANTES do UPDATE para evitar conflito durante o rename
ALTER TABLE card_sets DROP CONSTRAINT card_sets_code_key;
ALTER TABLE card_sets ADD CONSTRAINT card_sets_code_lang_key UNIQUE (code, language);

-- 2. Remove _ja de todos os codes (agora seguro — UNIQUE é por (code, language))
UPDATE card_sets
SET code = regexp_replace(code, '_ja$', '')
WHERE code LIKE '%_ja';
