# FND-01C / AUD-01 — Integridade transacional e replay offline

24/11/2026. Fonte histórica #8 e candidata #219 83e7f4b9. Este documento é evidência inicial estática e de CI, não homologação.

## Correção P0 de replay offline
O backend app.go chama startIdempotencyMaintenance no startup, com retenção de 30 dias, lotes de 2.000, até cinco por tabela, em idempotency_keys, procurement_idempotency_keys, return_idempotency_keys e finance_idempotency_keys, e ticker de uma hora. A migration 0016 cria índice por created_at. O teste histórico de startup da #8 não está no caminho antigo na candidata. Reintroduzir prova com fixtures exclusivamente descartáveis, cleanup antes de fechar pool e tenant próprio; testar as quatro tabelas conforme schema atual.

Antes de #220, retryQueueItem e rebindQueueItemToCashSession renovavam createdAt sem limite absoluto; o backend pode não conhecer a chave antiga após retenção. [#220](https://github.com/GustavoMFDS/SistemaERP/pull/220), commit 0b9fbafd02fc924940b93139465ed3a20543fb68, acrescenta intentCreatedAt, bloqueio de 28 dias, migração conservadora de registros com tentativa anterior sem data original, bloqueio de datas inválidas/futuras, guarda antes de cada envio, mensagens e botões desativados no PDV e cenário E2E com uma venda recente simulada e quatro casos bloqueados. Nenhum registro é descartado automaticamente. Uma decisão técnica conservadora: registros legados com idade ambígua exigem conferência mesmo que talvez ainda sejam recentes. Não protege contra manipulação deliberada do relógio/localStorage. O teste novo ainda depende de CI. Não houve execução de venda ou banco real.

## Matriz inicial da #8
| Contrato | Evidência na candidata | Pendente |
|---|---|---|
| Retenção server-side | app.go, migration 0016; quatro tabelas | teste de fronteira e lotes, isolamento de fixtures |
| Replay offline | #220 proposta | E2E em HEAD exato, concorrência entre abas e mudança de sessão/escopo |
| Caixa concorrente | cash_concurrency_test.go presente | comparar invariantes e código com #8; não inferir equivalência por nome |
| Integridade tenant | 0003 adiciona tenant_id NOT NULL, 0017 atual só barcode por tenant; 0018/0019 adicionam índices compostos e FKs para compras/devoluções | conferir catálogo PostgreSQL e FKs compostas de tabelas centrais, políticas de exclusão/SET NULL e escopo de atores |
| Auditoria e privacidade | módulos e testes parciais existentes | mapear round7/9 e permissões; ações por tenant, auditoria transacional e exportação |
| Fiscal e estoque | suites e migrations evoluídas | mapear invariantes round9 e validação de documentos, histórico imutável |
| Financeiro | módulo atual e migrations de contas | datas/fusos, integridade de ledger e reconciliação |

Busca estática em clone isolado do HEAD dbbbf4d8 (sem mudança de migrations até candidata #220): não encontrou nomes históricos products_tenant_category_fk, sales_tenant_customer_fk, inventory_balances_tenant_product_fk, sales_tenant_cash_session_fk, sale_items_tenant_sale_fk, sales_created_by_membership_fk, audit_logs_actor_membership_fk, idempotency_keys_tenant_sale_fk. Existem FKs compostas em módulos posteriores (compras, devoluções, fiscal, imagens). Ausência de nomes não prova ausência de restrição equivalente. A migration 0017 histórica da #8 não deve ser sobreposta à 0017_tenant_scoped_product_barcode; qualquer nova proteção exige migration >=0053 com pré-validação e rollback específicos, sem corrigir dados automaticamente. Priorizar um teste de contrato sobre schema descartável e inventário pg_constraint antes de mudar schema.

## Revalidação
#218 HEAD 38aea85: 22 success, 2 em andamento e 4 skips na consulta; nenhum failure. #219 HEAD 83e7f4b9: 18 success, 6 em andamento e 4 skips; nenhum failure. [Job Garnet](https://github.com/GustavoMFDS/SistemaERP/actions/runs/45485171903/job/138768509974) passou, logs confirmam PASS native Garnet ERP security contract write e verify e recuperação AOF após interrupção forçada. Não equivale a reboot físico. #220 aberta Draft em base #219; CI iniciado. Manter VM desligada e snapshots para validação do Setup real; não remover ainda. Sem merge nem fechamento de PR.
