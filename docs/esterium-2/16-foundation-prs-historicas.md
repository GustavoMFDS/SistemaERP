# FND-01C — Triagem das 18 PRs históricas

24/11/2026. Main 69d5e205, candidata #218 38aea85bc5793e538b54762ffb935e7e7a14d48e (base #216). Comparação por blob ou remoção equivalente de todos os caminhos retornados pela API (cada PR tem menos de 100 arquivos). Não constitui equivalência funcional dos divergentes. Números de arquivos podem se sobrepor.

| PR | Arquivos | Iguais main | Iguais candidata | Ausentes candidata (não removidos) | Próxima ação |
|---|---:|---:|---:|---:|---|
| #136 | 6 | 1 | 0 | 0 | Reconciliar assertions de navegação/PDV e fixture |
| #135 | 16 | 13 | 12 | 0 | Revisar team/service.go, toolchain atual e gofmt |
| #123 | 3 | 1 | 0 | 0 | Go/lock/CI evoluídos; confirmar scanner e gates |
| #122 | 10 | 8 | 8 | 0 | Revisar modules.go e team/service.go |
| #75 | 6 | 1 | 1 | 0 | Preflight read-only e testes de recusa |
| #74 | 6 | 2 | 1 | 0 | Smoke React/API mesma origem e busca |
| #73 | 11 | 0 | 0 | 0 | Segredos, ACL, manifestos e SCM |
| #66 | 4 | 1 | 1 | 0 | TLS LAN/loopback e rejeição HTTP |
| #64 | 2 | 0 | 0 | 1 | Recuperação de segurança via #219 |
| #62 | 3 | 0 | 0 | 2 | Contrato Redis/Garnet via #219 |
| #57 | 3 | 2 | 1 | 0 | Workflow #214/#216 evoluído, base fiscal descartável |
| #56 | 2 | 1 | 1 | 0 | Fornecedor idêntico; CI count=2 confirmado |
| #55 | 23 | 1 | 1 | 0 | Reconciliar E2E e histórico importações |
| #53 | 16 | 13 | 12 | 0 | Revisar team/service.go e toolchain |
| #31 | 2 | 0 | 0 | 0 | Backup e restauração, imagens/variantes |
| #30 | 3 | 0 | 0 | 0 | Reruns substituídos em parte por #214; revisar fixtures |
| #27 | 7 | 0 | 1 | 2 | Marca definitiva Esterium e PWA: revisar equivalência |
| #8 | 86 | 0 | 0 | 17 | P0: revisão de contratos transacionais, tenant, fiscal e privacidade; não transplantar migração 0017 antiga |

## Gaps prioritários
#62/#64 tinham testes específicos do backend real de token e rate limiter. A candidata tem startup probe atômico (consumo único e concorrência, identidade e rate limit) e script nativo TLS/ACL/AOF que recupera SET e Lua INCR após queda forçada. Isso não demonstra a recuperação de tokens consumidos/revogados e throttling usando o store real. [PR #219](https://github.com/GustavoMFDS/SistemaERP/pull/219) (83e7f4b9) acrescenta contrato em duas fases no smoke standalone Windows, com cert pinado, credenciais sintéticas, TTL, falha fechada, sem alterar trust store, produto ou instalação real. Não equivale ainda à execução: CI e revisão pendentes. Não fechar #62/#64 até comprovar equivalência suficiente.
 
#8 é especialmente sensível: os migrations 0015 e 0016 existem na candidata, mas 0017 agora é tenant_scoped_product_barcode e não a antiga tenant_relational_integrity. 17 caminhos ausentes, incluindo testes de tenant, privacy, timezone, fiscal, idempotência e E2E rounds 7–9. Não renumerar ou sobrepor migration existente, nem assumir que recursos ausentes por caminho estão ausentes funcionalmente. Inventariar invariantes por teste e mapear para schema/código atual antes de portar testes mínimos.

## CI da #218
Snapshot consultado: {"skipped":4,"success":18,"in_progress":6}. [CI 45484163407](https://github.com/GustavoMFDS/SistemaERP/actions/runs/45484163407), [Windows Setup 45484163498](https://github.com/GustavoMFDS/SistemaERP/actions/runs/45484163498). Security aprovado: govulncheck em Go 1.26.11 informa zero vulnerabilidades alcançáveis, seis em pacotes importados e uma em módulo requerido sem chamadas vulneráveis detectadas; npm audit zero. SPA origin e prodlike aprovados, confirmar demais jobs e gate final. Sem merge ou fechamento nesta sessão.
