# Roadmap e backlog mestre

Planejamento qualitativo inicial, não promessa de prazo. P0 = bloqueio de piloto/risco crítico; P1 = operação/gestão; P2 = expansão analítica; P3 = opcional. Estimativas B/M/A = baixa/média/alta complexidade, sujeitas à auditoria e capacidade.

| Fase | IDs | Impacto | Complexidade | Gate |
|---|---|---|---|---|
| Auditoria | AUD-01 | Muito alto | A | Inventário e matriz de evidências |
| Foundation | FND-01–10 | Muito alto | A | Versão candidata, aceite técnico, físico e fiscal aplicável |
| Retail | RET-01–10 | Muito alto | M/A | Fluxos transacionais reconciliados |
| Management | MGT-01–09 | Alto | M/A | Financeiro confiável, UX por perfil |
| Analytics | ANA-01–05 | Alto | M | Métricas reconciliadas e autorizadas |
| Intelligence | INT-01–06 | Alto potencial | A | Dados adequados, backtest, aprovação humana |
| Ecosystem | ECO-01–06 | Variável | A | Demanda real e integrações seguras |
| Design/UX | UX-01–04 | Alto | M | Transversal, testes por jornada |

## Priorização inicial
| Trabalho | Valor | Complexidade | Pré-requisito |
|---|---|---|---|
| Estabilização/instalação | Muito alto | A | AUD-01, FND-01 |
| Fiscal | Muito alto | A | FND-07, externos |
| Backup/recuperação | Muito alto | M | FND-04 |
| Inventário físico | Muito alto | M | Regras de estoque verificadas |
| XML compras | Alto | M | RET-05, fiscal/contábil |
| Financeiro avançado | Muito alto | A | MGT-01, contrato de custos |
| Pix/TEF | Alto | A | RET-09, provedor |
| BI | Alto | M | ANA-01, dados |
| Design System | Alto | M | UX-01, Tailwind reconciliado |
| Mobile | Alto | M | PWA e segurança |
| CRM | Médio/alto | M | Vínculo, consentimento |
| Previsão/assistente | Alto potencial | A | INT-01, dados, autorização |
| E-commerce | Variável | A | Núcleo, demanda |

## Dependências e estratégia
AUD-01 + FND-01 -> baseline validada -> incrementos de Retail/Management -> métricas Analytics -> Intelligence. UX transversal. Trabalho documental e design podem avançar em paralelo, código que depende de PR pendente aguarda base reconciliada. Fiscal e testes físicos são trilhas com dependências externas; não atrasar auditoria, mas não operar comercialmente sem gates aplicáveis.

## Primeiro ciclo
| ID | Estado inicial | Próxima ação |
|---|---|---|
| FND-01A | A iniciar | Inspecionar #216 vs main, HEAD, checks, arquivos e commits |
| FND-01B | A iniciar | Comparar #211 e #212–215; classificar cobertura e conflitos |
| FND-01C | A iniciar | Reconciliar PRs históricas; uma decisão por PR com evidência |
| AUD-01A | Parcial | Snapshot inicial registrado; inventariar rotas, schema, módulos, telas, testes |
| AUD-01B | A iniciar | Revisão transacional e tenant/RBAC; testes negativos |
| AUD-01C | A iniciar | Rodar suítes em ambiente descartável; evidências por SHA |
| AUD-01D | A iniciar | Auditoria UI/performance/infra e gaps operacionais |
| FND-02 | A iniciar | Corrigir README e catálogos após auditoria |
| FND-03/04/05/06/07 | Bloqueios externos a confirmar | Checklist #151 e PR #216; obter condições e responsáveis |

## Gestão de tarefas
IDs estáveis definidos nos documentos de fase. Decompor épicos em issues pequenas (ID, contexto, escopo/fora do escopo, dependências, contratos, critérios de aceite, testes, evidências, rollback). Não abrir todas as issues de expansão sem validar prioridade e evitar duplicidade. Estados: proposta, pronta, em implementação, em revisão, bloqueada, concluída com evidência; nunca confundir PR aberta com entrega.

## Atualização de execução — 24/11/2026

| ID | Estado atual | Evidência / próxima ação |
|---|---|---|
| FND-01A | Em revisão | #216 candidata consolidada ed0095f; CI integral e Windows Setup disparados |
| FND-01B | Em revisão | #218 incorporada; #227 fecha lacuna de PLAN GUI de #211; comparar demais divergências de #212–215 |
| FND-01C | Em revisão | #219–226 ancestrais da candidata, checks individuais aprovados; revisão histórica restante |
| AUD-01B | Parcial validado | #220–226: replay, tenant, retenção, data comercial, privacidade e auditoria; não equivale a auditoria completa |
| AUD-01C | Em revisão | 210 sucessos cumulativos; validar runs integrais por ID, não apenas SHA compartilhado |
| FND-02 | Em andamento | 21 documentos existentes auditados, zero links relativos para caminhos ausentes; atualização #21 |
| FND-03 | Em revisão | ZIP/EDB automatizados; PLAN GUI #227 pendente; assinatura comercial e VM física pendentes |
| FND-04–10 | Não concluídos | Backup físico, LAN, periféricos, fiscal, performance, recuperação e piloto exigem gates próprios |

Detalhes: [21 — consolidação da candidata](21-foundation-consolidacao-candidata.md). Nenhuma liberação comercial.
