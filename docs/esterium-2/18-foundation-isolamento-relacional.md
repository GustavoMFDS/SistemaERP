# FND-01C / AUD-01 — Isolamento relacional no núcleo do varejo

24/11/2026. Base #220 HEAD 0b9fbafd02fc924940b93139465ed3a20543fb68. Nova [PR #221](https://github.com/GustavoMFDS/SistemaERP/pull/221), HEAD f54a160a27587b48e2da687adc0e57790a9209c4, Draft. Comparação das migrations 0001, 0003, 0015, 0017–0020 e 0038: as FKs originais usam id global, a migration 0003 acrescenta tenant_id NOT NULL, módulos mais novos usam FKs compostas; existem índices compostos em products, sales e cash_sessions. A 0017 atual é barcode por tenant; não substituir histórico. Inspeção estática, não consulta de produção.

## Incremento 0053 — 11 FKs
| Origem | Referência | Exclusão preservada |
|---|---|---|
| products.category_id | categories | SET NULL só category_id |
| inventory_balances.product_id | products | CASCADE |
| inventory_movements.product_id | products | RESTRICT |
| cash_sessions.cash_register_id | cash_registers | RESTRICT |
| sales.cash_session_id | cash_sessions | RESTRICT |
| sales.customer_id | customers | SET NULL só customer_id |
| sale_items.sale_id | sales | CASCADE |
| sale_items.product_id | products | RESTRICT |
| payments.sale_id | sales | CASCADE |
| cash_movements.cash_session_id | cash_sessions | RESTRICT |
| cash_session_reconciliations.cash_session_id | cash_sessions | CASCADE |

Cada chave valida (tenant_id, id) no PostgreSQL. Três índices únicos novos para categories/customers/cash_registers; demais índices reaproveitados. DDL transacional e imediatamente validada, sem UPDATE/DELETE para sanar histórico. Rollback remove apenas novos objetos, não dados. Não equivale a RLS ou autorização completa.

## Evidência e gate
Seis arquivos, 314 adições, zero remoções. Consulta somente leitura scripts/audit-core-tenant-relationships.sql (contagens de divergência, sem identificadores); executar em cópia verificada antes de qualquer atualização real. Bloquear se existir divergência, não reatribuir dados. Migration pode exigir janela de manutenção por locks; migrador pode marcar dirty em falha, não forçar versão. CI dedicado cria banco exclusivo v52, seed sintético, insere referência cruzada proposital e exige recusa atômica, remove só fixture incorreta, sobe v53, testa vínculos corretos e 11 recusas por constraint específica, SET NULL preservando empresa, downgrade e reaplicação. Teste apenas proposto, ainda em execução no CI; não declarar migração validada até evidência.

## Snapshots
- #218 22 success, 2 em andamento, 4 skipped; #219 22 success, 2 em andamento, 4 skipped.
- #220 18 success, 6 em andamento, 4 skipped; E2E de replay ainda não confirmado.
- #221 22 em andamento, 2 queued, 4 skipped na primeira consulta.
- Nenhum failure nos snapshots; não são resultados finais. Main e dados reais intactos.

Próximos: validar CI e inventário pg_constraint após 0053; revisar ledger, invoices, XML, idempotency_keys, tenants de entidades centrais e membership de atores separadamente (evitar impor semântica que impeça revogação/auditoria). Testes de retenção e concorrência, isolamento API, e homologação VM seguem pendentes. Não iniciar instalação em loja.
