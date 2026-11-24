# Foundation — consolidação da candidata e PLAN GUI

24/11/2026. Atualização FND-01B/C e FND-03.

## Evidências finais dos incrementos
| PR | HEAD | Success | Skipped | Failure / pendente |
|---|---|---:|---:|---:|
| 218 | 38aea85 | 25 | 4 | 0 |
| 219 | 83e7f4b | 25 | 4 | 0 |
| 220 | 02f540c | 25 | 4 | 0 |
| 221 | 615f4e6 | 25 | 4 | 0 |
| 222 | 4ddfaca | 20 | 0 | 0 |
| 223 | 8585500 | 25 | 4 | 0 |
| 224 | c816fad | 20 | 0 | 0 |
| 225 | 54e761d | 25 | 4 | 0 |
| 226 | ed0095f | 20 | 0 | 0 |

210 sucessos cumulativos de jobs, não casos de teste únicos. #222/#224/#226 não dispararam workflow Setup por filtros de paths; não contabilizar como execução. [#225 integração](https://github.com/GustavoMFDS/SistemaERP/actions/runs/45488892253/job/138781566971): recusa esperada FK core54_idempotency_sale_fk, 54/u, 54/d, 54/u, 54:false. [#226 integração](https://github.com/GustavoMFDS/SistemaERP/actions/runs/45489170207/job/138782534089): duas execuções PASS do contrato de privacidade com falha de auditoria injetada, rollback de status/bloqueio e recusa de status obsoleto.

## Consolidação
[PR #216](https://github.com/GustavoMFDS/SistemaERP/pull/216) branch codex/integration-212-215-20261101 avançada por fast-forward de 369b7bc0646a7755419f0288960732511b125189 até ed0095fd80a1b9697399643a42c292d194d952ef. Compare API: 14 commits ahead, 0 behind, 42 arquivos no incremento, merge base igual ao HEAD anterior. GitHub informou 170 arquivos no diff total contra main. Main permanece 69d5e205400d3f4b02c35dca9b164bdeaa9cf49c. Sem force push. GitHub marcou #218 merged/closed automaticamente no fast-forward (merge_commit_sha 38aea85). #219–226 ainda abertas, mas HEADs ancestrais da candidata; preservar branches e trilha, aguardar validação integral e revisão das fontes históricas antes de encerramentos. PR #216 continua Draft. Descrição atualizada com contexto e texto histórico identificado.

Novos runs no contexto #216: [CI 45494372036](https://github.com/GustavoMFDS/SistemaERP/actions/runs/45494372036) e [Windows Setup 45494372043](https://github.com/GustavoMFDS/SistemaERP/actions/runs/45494372043). Ambos disparados no SHA ed0095f; resultado pendente. A consulta por SHA pode combinar checks de #226 e #216, por isso verificar runs pelo ID/contexto da PR.

## Reconciliação #211 / #227
#211 tinha gate 1 do checklist mais rigoroso que o HEAD consolidado. #218 restaurou flag VerifyApprovedRelease no coordenador, mas GUI Ver plano apenas usava AllSigned sem flag e não validava assinatura do launcher. [#227](https://github.com/GustavoMFDS/SistemaERP/pull/227), HEAD 29f1948003eaefd7e123f54d10eb2071d5c1af2f: 4 arquivos, 52 adições, 12 remoções. GUI assinado autentica launcher e passa flag strict; preview unsigned permanece estrutural, Apply permanece interativo. Verificar arquivos não é autenticação comercial. Planos GUI podem extrair payload temporário; não alegar zero escritas no sistema de arquivos. Lab real EDB/ZIP usa certificado temporário confiado exclusivamente no runner, recusa publicador errado, compara existência de três caminhos de instalação e estados de serviços observados, exige PASS da release antes do Apply. Não prova clique real na GUI. Checklist gate 1 reprova sem assinatura comercial; guia inclui strict PLAN. #227 não incorporada à #216 nesta etapa. CI pendente.

## Documentação
Clone somente leitura da branch #217 HEAD 07f6910c4ba3799ea861977f3af7ddffdbafe9d8 em TEMP do Windows. Auditoria de caminhos Markdown relativos nos 21 arquivos .md de docs/esterium-2: zero caminhos inexistentes. Não valida anchors ou links HTTP. DEC-13 (setup nativo) confirmado em 13-decisoes.md e 04-foundation.md. Sem instalação no Windows principal.

## Gate seguinte
Aguardar resultados dos runs integrais #216 e #227. Verificar relatórios Tailwind/artefatos, checklist #151 e provas do Setup. Reconciliar PRs históricas por comportamento e preservar migrações 0053/0054. Próximo pacote VM-LAB deve corresponder ao HEAD aprovado, assinatura de teste temporária somente na VM isolada. Homologar GUI, reboot, atualização, reparo, desinstalação conservadora, restore e LAN. Certificação comercial e fiscal continuam bloqueios externos; não alterar dados reais ou main.
