# Validação Docker, PostgreSQL, Redis e PDV — 2026-10-09

**Projeto:** SistemaEmGo — branch `ops/pilot-readiness-20260923`, PR #15.
**Execução:** clone separado `D:\SistemaEmGo-Validacao-20261009`, máquina Gustavo (Windows).
**Ambiente:** exclusivamente local/demonstração; não executar emissão fiscal real.
**Estado:** algumas verificações passaram; **E2E completo continua reprovado**. Não autoriza merge/produção.

## Infraestrutura de testes

- Docker Desktop Linux Engine: operacional, PostgreSQL 16 e Redis 7.4 saudáveis.
- Projeto Docker Compose isolado: `sistemaemgo-validacao-20261009`.
- Portas dedicadas: PostgreSQL `127.0.0.1:55433`; Redis `127.0.0.1:56379`.
- Banco `sistemaemgo`: teste de integração Go (não reutilizar como banco do PDV visual).
- Banco `sistemaemgo_ui`: testes Playwright e interface de demonstração.
- Banco `sistemaemgo_migration_audit`: ensaios de upgrade/rollback, completamente isolados.
- API local: `http://127.0.0.1:18080`; web: `http://127.0.0.1:5173`; health HTTP 200. Rate limit de login desativado **somente na API de teste**, para executar Playwright repetidamente. Nunca repetir isso em produção.
- Seed é de demonstração, com contas de senha conhecida. Nunca expor as portas públicas nem usar esses dados em um ambiente real.

## Resultados confirmados

| Procedimento | Resultado |
|---|---|
| Docker + PostgreSQL + Redis | Passou, ambos healthy |
| Migrations limpas 0001–0034 | Passou em bancos novos, schema 34 `dirty=false` |
| Seed da versão 34 | Passou |
| `go test -tags=integration ./tests/integration -count=1` (primeira execução) | Exit 0 no banco de integração; não é prova de execuções subsequentes sobre o mesmo banco |
| Teste integração de convite, aceitação, isolamento de CNPJ e revogação | Passou |
| Teste novo de seed/RBAC (admin, gerente, caixa) | Passou |
| `go test ./...`, `go vet ./...` | Passaram |
| Frontend `npm run lint`, `npm run build` | Passaram; lint tem avisos de hooks |
| Playwright de funcionários | 3 passaram |
| Playwright de clientes e ranking após correção do seed | 6 passaram |
| Playwright de estoque (relatório/movimentos) | 5 passaram na execução selecionada |
| Playwright do núcleo PDV (barra, autorização, cancelamento, idempotência) | 6 passaram |
| Playwright de venda offline/reconexão e PDV operacional em banco UI separado | 2 passaram |
| Playwright smoke Chromium após correção de seletor | 1 passou |
| Rollback da 33 sem membro suspenso e reapply 33/34 | Passou; versão 34 `dirty=false` |
| Rollback 33 com membro suspenso | **Recusa segura confirmada**, bloqueio preservado |
| Recuperação do banco descartável do teste de rollback negativo | Passou; v34 `dirty=false`, vínculo permanece inativo |

## Defeitos efetivos corrigidos

1. **Migration 0033 down inválida:** `DO $` / `END $` corrigidos para delimitadores PostgreSQL `DO $$` / `END $$`. A instalação subia, mas o downgrade falhava por sintaxe. Reexecutado com sucesso em banco descartável.
2. **RBAC de gerente em seed novo:** a migration 0034 criava `customer:read/write` antes do seed criar o papel gerente. O seed foi corrigido para atribuir ambas as permissões a gerente, sem liberar para caixa. Novo teste de integração valida efetivamente as permissões de admin, gerente e caixa.
3. **Teste visual ambíguo:** busca de heading `Produtos` passou a ser exata; smoke Chromium repetido e aprovado.

## Problemas ainda abertos (não encobrir)

- Suíte Chromium completa: **14 passaram, 12 falharam, 74 não foram executados**; interrupção configurada ao atingir 12 falhas. Há seletores ambíguos, testes de refresh que retornaram valor inesperado e dependência entre testes que tentam abrir o mesmo caixa.
- A execução de integração inicial deixou registros financeiros/fiscais imutáveis e sessões no CNPJ demo: casos concorrentes usam o mesmo tenant e não podem limpar todas as referências sem violar regras de append-only. A solução correta é executar cada lote sensível em um banco **descartável** independente e não relaxar o schema ou forçar deleções de trilha auditável.
- A repetição do teste `TestNFCeReservation_IsAtomicAndIdempotentPerSale` no banco já usado falhou com número reservado 2, quando o teste pressupõe um banco novo e espera 1. **Não representa falha comprovada de emissão**, mas mostra que o teste não é isolado/repetível.
- Os testes de PDV offline/operações **passaram em banco separado**, após falharem num banco contaminado com caixas abertos por integrações anteriores.
- A API demo e as telas funcionam localmente, mas ainda **não existe homologação SEFAZ real**, periféricos físicos, integração real de Pix/TEF, nem aceite de operador.
- O GitHub Actions hospedado permanece sem evidência de runners executando o HEAD final. Não dar merge.

## Checklist de continuidade P0

- [x] Instalação limpa de schema até 34, validação de seed e RBAC.
- [x] Rollback/reapply 33/34 em banco descartável, sem vínculo suspenso.
- [x] Verificação fail-closed de rollback com vínculo suspenso e recuperação do banco descartável.
- [x] Fluxos selecionados do PDV Chromium, inclusive offline e carrinho.
- [ ] Corrigir independência da suíte Playwright completa e deixar **100/100 passando** no mesmo SHA.
- [ ] Redesenhar fixtures PostgreSQL dos testes imutáveis para bancos/tenants isolados, sem suprimir auditoria.
- [ ] Teste integral de integração + Playwright em ambiente CI reproduzível com banco separado por job.
- [ ] Executar e registrar testes em runners CI no commit final; refazer E2E prod-like.
- [ ] Testar caixa, impressora térmica, contingência, restauração e piloto em hardware real.
- [ ] Homologar fiscal por CNPJ/UF antes de transmissão real.

Este relatório documenta uma **validação parcialmente aprovada e bloqueios reproduzidos**; não é certificado de homologação.
