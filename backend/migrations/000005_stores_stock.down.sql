-- Atenção: ENUMs do Postgres não permitem remover valor sem recriar o tipo.
-- O valor 'ligapokemon' adicionado em price_source permanece após esta down
-- migration. Não é problema: ENUMs do Postgres só são problema quando o valor
-- é removido em uso ativo, e aqui estamos zerando o schema.

DROP TABLE IF EXISTS external_card_refs;
DROP TABLE IF EXISTS stock_movements;
DROP TYPE  IF EXISTS stock_movement_kind;
DROP TABLE IF EXISTS stock_items;
DROP TRIGGER IF EXISTS trg_stores_updated_at ON stores;
DROP TABLE IF EXISTS stores;
