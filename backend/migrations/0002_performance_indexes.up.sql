-- 0002_performance_indexes.up.sql

BEGIN;

-- Etapa 8 (Performance): a base já tem GIN(trgm) para busca por name/sku.
-- Esses índices B-Tree ajudam padrões comuns de ORDER BY / filtros simples.

CREATE INDEX IF NOT EXISTS products_name_idx ON products(name);
CREATE INDEX IF NOT EXISTS products_active_name_idx ON products(active, name);
CREATE INDEX IF NOT EXISTS products_active_min_stock_idx ON products(active, min_stock);

COMMIT;
