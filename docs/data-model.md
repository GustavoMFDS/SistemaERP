# Etapa 2 — Modelagem de dados (PostgreSQL)

Este documento descreve entidades e índices do MVP.

## Entidades principais
- **users, roles, permissions** (+ tabelas de relacionamento)
- **categories, products**
- **inventory_balances** (saldo por produto)
- **inventory_movements** (histórico auditável)
- **cash_registers, cash_sessions**
- **sales, sale_items, payments**
- **ledger_entries** (lançamentos financeiros)
- **companies** (emitente)
- **invoices, invoice_xml_files**
- **audit_logs**

## Índices sugeridos
- `products(sku)` UNIQUE
- `products(barcode)` UNIQUE WHERE barcode IS NOT NULL
- `products(name)` via `GIN (to_tsvector('portuguese', name))` (busca rápida)
- `inventory_balances(product_id)` PK
- `inventory_movements(product_id, created_at DESC)`
- `sales(status, created_at DESC)`
- `payments(sale_id)`
- `ledger_entries(created_at DESC, entry_type)`
- `audit_logs(created_at DESC, actor_user_id)`

## Consistência / transações
- **Finalizar venda**: transação única gravando `sales + sale_items + payments + ledger_entries + inventory_movements` e atualizando `inventory_balances` com locks.
- **Cancelar venda**: transação única com estorno (movimentação inversa) e lançamento reverso.
- **Ajuste de estoque**: transação única com motivo obrigatório + audit.
