# Foundation — privacidade, financeiro e trilha atômica

24/11/2026. FND-01C / AUD-01, reconciliação histórica #8. Alterações em cadeia, sem merge na main, sem operação comercial real.

## Gates obtidos
- #218 HEAD 38aea85 e #219 HEAD 83e7f4b: cada um 25 success, 4 skipped, zero failures, zero pendências. Skips opcionais não executados. Homologação física e aprovação de release comercial pendentes.
- #220 HEAD 02f540c: [E2E 1](https://github.com/GustavoMFDS/SistemaERP/actions/runs/45487496537/job/138776777418) e [E2E 2](https://github.com/GustavoMFDS/SistemaERP/actions/runs/45487496537/job/138776777352) 168 pass cada. Corrigido fixture anterior, contrato de replay agora executado. Não implica proteção contra manipulação deliberada de relógio.
- #222 HEAD 4ddfaca: 20 success, zero failure/pending no workflow CI visível; Windows Setup não disparado nesse HEAD na consulta (não contar como aprovado).
- #223 HEAD 8585500: [integration](https://github.com/GustavoMFDS/SistemaERP/actions/runs/45488096670/job/138778944358) PASS TestFinanceBusinessDayIgnoresDatabaseSessionTimezone duas vezes (0.02s cada).
- #224 HEAD c816fad: [integration](https://github.com/GustavoMFDS/SistemaERP/actions/runs/45488681101/job/138780826573) PASS TestPrivacyUserActionsRespectTenantMembership duas vezes (0.04s cada); demais checks em execução.

## #224 — identidade global versus vínculo local
Antes, privacy_repo bloqueava users.active mesmo quando usuário tinha vários CNPJs. Corrigido: bloqueio altera apenas user_tenants.active, preserva outras lojas. Anonimização global exclusiva desativa conta e vínculo, incrementa auth_version; qualquer outro vínculo, ainda que inativo, causa conflito. Admin protegido conforme política do módulo team. Exportação de active usa vínculo e conta; last_login_at global omitido para usuários compartilhados. Locks company/user/membership; autenticação atual verifica vínculo ativo. Não altera fluxos de autorização LGPD, nem oferece garantia contra mudanças diretas fora da aplicação.

## #225 — migration 0054
[PR #225](https://github.com/GustavoMFDS/SistemaERP/pull/225), HEAD 54e761d. 4 FKs compostas e 3 índices: ledger->sales/cash (SET NULL só coluna de referência preservando valor e tenant), idempotency_keys->sale (RESTRICT enquanto chave retida), user_tenant_roles->user_tenants (CASCADE apenas quando remover vínculo fisicamente; suspensão active=false preserva grants). Esta semântica impede reaparecimento de grants após religar vínculo; não é política de remoção de atores históricos. FKs fiscais 0023 já existentes; não duplicar. Consulta read-only de contagens scripts/audit-finance-membership-relationships.sql antes de upgrade. Sem alterações automáticas de dados. CI propõe banco dedicado v53->v54, rejeição de chave órfã sintética, rollback/reapply, preservação ledger e teste de relink. Requer evidência de integração e revisão de efeitos de deleção. DDL validada pode bloquear escritas; backup e manutenção necessários.

## #226 — auditoria transacional
[PR #226](https://github.com/GustavoMFDS/SistemaERP/pull/226), HEAD ed0095fd. 7 arquivos, 161 adições, 48 remoções. Anonimização cliente/usuário, bloqueio e update de status registram sucesso dentro do mesmo tx; erro do INSERT reverte a mutação. Handler não duplica esses eventos. Tenant, ação, recurso e resultado derivam do contexto autenticado e solicitação. Status compare-and-set pelo estado lido, conflito se mudança concorrente. Teste integra falha de inet no evento e verifica rollback de bloqueio/status, contagem exata, rejeição de transição obsoleta. gofmt sem diferenças semânticas em clone temporário; go test ./internal/modules/privacy/... ./internal/httpapi/handlers passou. Teste tagged compilou, SKIP local sem TEST_DATABASE_URL; não confundir com integração real. CI pendente.
Limites: criação/consentimentos/exportações continuam fluxo anterior; autorização de titular, escopo de request_type/status para ações, exportação completa e revisão de permissões ainda pendentes. Não garante LGPD integral.

## Próximos passos
Confirmar gates #225/#226 e resultados E2E/labs dos HEADs restantes, reduzir fila de branches por consolidação revisada. Revisar #211 checklist de release, #212 divergências de Setup, #213/#215 evidência visual e PRs históricas restantes. Validar VM Windows limpa, GUI, reboot, atualização, recuperação e desinstalação com dados sintéticos e backup. Não apagar VM nem publicar release comercial.
