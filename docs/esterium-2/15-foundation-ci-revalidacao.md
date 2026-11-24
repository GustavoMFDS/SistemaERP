# Foundation — Revalidação CI e reconciliação funcional

24/11/2026. PR #218, base #216 HEAD 369b7bc0646a7755419f0288960732511b125189; primeiro HEAD dbbbf4d8abfcae6a55f69b5d8ef440f2c9c65ee4.

## FND-01 — CI e bloqueios

A consulta dos checks do primeiro HEAD da #218 registrou 15 sucessos, cinco falhas, quatro skips, um cancelamento após atualização da branch e três jobs em andamento no último snapshot. A consulta não é um placar definitivo do run. Os jobs local-windows-service-tests, local-windows-installer-plan, backend, frontend e integration passaram; os laboratórios de instalação completa ainda não estavam concluídos.

- [CI 45482867965](https://github.com/GustavoMFDS/SistemaERP/actions/runs/45482867965): quatro jobs de navegador (local-built-spa-origin-smoke, E2E disposable 1 e 2, e2e-prodlike) falharam ao instalar Chromium: Playwright não suporta ubuntu26.04-x64. Nenhum teste funcional de navegador foi aprovado por esses jobs. Lockfile fixa Playwright 1.55.1. Commit [2051451](https://github.com/GustavoMFDS/SistemaERP/commit/2051451f8ac6550ab6152a2684f736a80226f958) fixa somente os três jobs/matriz afetados em ubuntu-24.04, sem eliminar testes. Nova execução pendente.
- [Job security](https://github.com/GustavoMFDS/SistemaERP/actions/runs/45482867965/job/138760355937): govulncheck reportou GO-2026-6975 (otel v1.45.0), GO-2026-6974 (otel/sdk e trace v1.45.0), corrigidos em v1.47.0; GO-2026-6859 (net/http Go 1.26.10), corrigido em Go 1.26.11. São três achados alcançáveis reportados pelo scanner. Atualizar dependências com go get/go mod tidy, checksums reais, toolchain em CI e documentação. Não excluir scanner nem usar allowlist.
- [Windows Setup 45482867927](https://github.com/GustavoMFDS/SistemaERP/actions/runs/45482867927): os testes de instalação integrada ainda não tinham conclusão no snapshot. Gate real de distribuição assinada e piloto físico continuam externos.

## FND-01B — comparação funcional adicional

Base #216 369b7bc. Estas são evidências estáticas de cobertura, não decisão de fechar PR.

| PR | Evidência observada | Estado |
|---|---|---|
| #212 | Setup C#, modos EDB/ZIP, empacotador offline, workflows e documentação presentes. HEAD de origem relata instalação em runner e 6 casos/4 skips; botão Apply GUI não automatizado. 29 arquivos divergentes precisam de análise semântica, principalmente coordenador/serviços e código Setup. | Parcialmente reconciliada |
| #213 | App.tsx carrega as 17 páginas por lazy/Suspense; Zod 4.6.7 no lockfile; script PWA valida PNG, CRC, dimensões, RGBA, manifesto e rótulo acessível. | Escopo central identificado; revalidar assets, build, desempenho e diffs |
| #214 | Dois arquivos de teste fiscal idênticos; workflow executa integração count=2, teste de fornecedor count=2 e fiscal em base separada descartável com flag TEST_FISCAL_DISPOSABLE_DB=1 e count=2. Job integration do primeiro HEAD #218 passou. | Forte evidência de cobertura; falta conferir conjunto completo de filtros e HEAD final |
| #215 | Lockfile Tailwind e plugin Vite 4.3.3, Vite usa tailwindcss(), remoção da config antiga aparece na lista de mudanças da #216. Fixture tem limpeza de caixa com guardas. | Migração central identificada; E2E visual e política de navegadores ainda precisam ser validados |

A #216 possui 138 arquivos alterados (duas páginas de resultados conferidas); não interpretar 138 como funcionalidades. Revisar arquivos de instalador divergentes, manter #211 até #218 passar e checklist reconciliado, não fechar #212–215 sem prova do HEAD final. A decisão DEC-13 sem Docker continua requisito de instalação e não é afetada pelo CI Linux.
