# FND-01A/B — Triagem inicial das PRs #216 e #211

Data: 24/11/2026. Inspeção somente leitura via GitHub; nenhuma execução de scripts, alteração de PR existente ou merge.

## #216 — candidata de consolidação

HEAD 369b7bc0646a7755419f0288960732511b125189; base main 69d5e205400d3f4b02c35dca9b164bdeaa9cf49c. Draft aberta. Consulta de check-runs do HEAD exato retornou 29 checks concluídos: 25 success, 4 skipped, zero falhas nessa resposta. Skipped: unsigned-real-readonly-plan, garnet-oss-real-tls-aof-smoke, postgres-oss-tls-schema-smoke, postgresql-oss-binaries-smoke. Não interpretar skips como aprovações; conferir obrigatoriedade e workflow/trigger. Entre os sucessos: backend, frontend, security, integration, E2E disposable run 1/2, e2e-prodlike, laboratório de primeira instalação zip/edb e gates Windows. Evidência: [checks do HEAD](https://github.com/GustavoMFDS/SistemaERP/commit/369b7bc0646a7755419f0288960732511b125189/checks).

Primeira página de arquivos alterados retornou 100 entradas (não é inventário completo): bootstrap admin, scripts Windows, workflows, testes fiscais, configuração, caixa, estoque, frontend, PWA, marca, Tailwind, recibo e documentação. Revisar paginação restante, diffs, escopo das PRs #212–215, possíveis dados sensíveis em evidências e documentação, e aceite externo antes de recomendar integração.

## #211 — não declarar substituída

HEAD 44a01aad140aae27a83a84d33a34059ee8e39c5d. Cinco arquivos modificados; o modo explícito read-only -VerifyApprovedRelease consta no script unificado, documentação e testes desse HEAD. Consulta dos mesmos arquivos no HEAD #216 mostra blobs diferentes em todos os cinco; o identificador -VerifyApprovedRelease está ausente nos trechos encontrados no script, documentação e testes da #216. Há verificações de assinatura em outros trechos, portanto ausência do identificador não prova ausência de proteção equivalente.

| Arquivo | Blob #216 | Blob #211 |
|---|---|---|
| scripts/install-esterium-local-complete.ps1 | e596e53d | 126a6f43 |
| docs/esterium-local-one-command-first-install.md | b823f004 | ea5bb9b9 |
| docs/esterium-local-pilot-checklist.md | 7fa862e6 | 54f0b87a |
| scripts/test-esterium-first-run-probe-ci.ps1 | 16230e39 | 36ab1fff |
| scripts/test-esterium-signed-release-positive-ci.ps1 | 5d925070 | a96519d3 |

Classificação provisória: cobertura não demonstrada. Comparar semântica do gate de assinatura completo antes de escrita, manifesto/certificado de canal independente, modo PLAN rigoroso, guardas Apply e testes negativos. Se faltar comportamento, portar incremento mínimo sobre base candidata e testar; se equivalente, documentar a prova antes de fechar. Não confiar em assinatura sintética como comercial.

## Próxima ação
FND-01A/B permanece em andamento: completar diffs paginados, comparar #212–215, verificar checks exigidos e avaliar #211. FND-01C (históricas) e AUD-01 detalhada continuam pendentes. Nenhuma release liberada.
