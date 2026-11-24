# Esterium ERP 2.0 — Master Development Plan

**Início:** 24/11/2026 | **Estado:** proposta de evolução iniciada a pedido do proprietário do projeto; requisitos detalhados, estimativas e decisões de produto sujeitos a aceite. Não é declaração de release ou homologação.

## Objetivo
Transformar o Esterium em um ERP de varejo confiável para as lojas da família, com operação local segura, redução de trabalho manual, gestão financeira e comercial, análise de dados e inteligência assistida. Comercialização para terceiros é uma opção futura, não um compromisso.

## Ordem de leitura
| Documento | Propósito |
|---|---|
| [01 — Visão e escopo](01-visao-produto.md) | Usuários, objetivos, fronteiras e métricas |
| [02 — Baseline e auditoria](02-auditoria-baseline.md) | Evidências, PRs, pendências e plano de auditoria |
| [03 — Arquitetura](03-arquitetura.md) | Camadas, dados, eventos, contratos e segurança |
| [04 — Foundation](04-foundation.md) | Estabilização, instalação, backup, fiscal e piloto |
| [05 — Retail](05-retail.md) | Estoque, compras, PDV, pagamentos e gestão comercial |
| [06 — Management](06-management.md) | Financeiro, CRM, crediário e gestão mobile |
| [07 — Analytics](07-analytics.md) | BI, definições de métricas e painéis |
| [08 — Intelligence](08-intelligence.md) | Previsão, anomalias, preços e assistente |
| [09 — Design System e UX](09-design-ux.md) | Experiência, acessibilidade e critérios visuais |
| [10 — Ecosystem](10-ecosystem.md) | Catálogo, canais digitais, integrações e distribuição |
| [11 — Roadmap e backlog](11-roadmap-backlog.md) | Sequência, dependências, tarefas e aceite |
| [12 — Governança e qualidade](12-governanca-qualidade.md) | Processo de PR, testes, evidências e documentação |
| [13 — Decisões e questões abertas](13-decisoes.md) | Decisões existentes, propostas e questões de negócio |
| [14 — Triagem de PRs](14-foundation-pr-triagem-inicial.md) | Evidências de reconciliação inicial |
| [15 — Revalidação de CI](15-foundation-ci-revalidacao.md) | Bloqueios, correções e comparação funcional |

## Fontes e autoridade
Base técnica observada: main @ 69d5e205400d3f4b02c35dca9b164bdeaa9cf49c. Documentos anteriores permanecem históricos; este plano não substitui contratos implementados, runbooks ou requisitos fiscais oficiais. Em caso de conflito entre proposta e comportamento existente, registrar divergência antes de alterar código.

Referências: [README](../../README.md), [arquitetura](../architecture.md), [catálogo histórico](../catalogo-geral-esterium-2026-10-16.md), [roadmap varejo](../retail-roadmap.md), [refinamento](../refinamento-final-esterium-2026-10-16.md), [checklist #151](https://github.com/GustavoMFDS/SistemaERP/issues/151), [PR #216](https://github.com/GustavoMFDS/SistemaERP/pull/216).

## Como começar
Executar FND-01 (reconciliação de PRs e evidências) e AUD-01 (inventário verificável), depois selecionar pequenos incrementos com critérios de aceite. Nenhum merge, instalação, emissão fiscal, alteração de dados reais ou acesso a credenciais é autorizado por este plano. Os exemplos são sintéticos.
