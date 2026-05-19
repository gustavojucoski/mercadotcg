-- Reverte: restaura _ja nos codes e a constraint original.

ALTER TABLE card_sets DROP CONSTRAINT card_sets_code_lang_key;
ALTER TABLE card_sets ADD CONSTRAINT card_sets_code_key UNIQUE (code);

UPDATE card_sets
SET code = code || '_ja'
WHERE language = 'ja';
