# Validação local do PDV — 2026-10-09

Clone isolado no dispositivo **Gustavo**, caminho local:
`D:\SistemaEmGo-Validacao-20261009`.

- Branch: `ops/pilot-readiness-20260923` (PR #15).
- Commit das correções validadas: `efec954`.
- SO: Windows. Go: 1.26.2; Node: 24.11.0; npm: 11.6.1.
- `npm ci --no-audit --no-fund`: **passou**, 278 pacotes.
- `npm run lint`: **passou**, 0 erros / 10 avisos de hooks.
- `npm run build`: **passou** após corrigir ternário JSX em StockMovementsPage.
- `go test ./...`: **passou** após remover dependência Linux dos testes simulados de xmllint; agora é compilado um processo auxiliar Go nativo para testar argumentos, STDIN e erro de validação em Windows ou Linux.
- `npx playwright test e2e/catalog-import-parser.spec.ts e2e/opening-stock-parser.spec.ts --project=chromium --reporter=line`: **9/9 passaram**.
- `go vet ./...`: **passou** (GO_VET_PASS, código de saída 0).
- `gofmt -l .` no checkout Windows mostrou praticamente todos os arquivos por conversão de finais de linha CRLF (Git autoconversão); não deve ser tratado como falha de estilo generalizada sem comparar em ambiente Linux. O arquivo Go alterado foi formatado com gofmt.
- Banco/integracao: **bloqueado por ambiente**. Docker CLI disponível, mas `docker info` falhou: pipe `dockerDesktopLinuxEngine` não encontrado, mesmo após solicitar inicialização da interface. Não houve migração 0033/0034, teste de transação, isolamento real CNPJ A/B nem E2E com backend vivo nesta execução.
- CI no GitHub também não prova execução dos seis jobs: falha de disponibilização de runners registrada no catálogo.

## Correções enviadas ao PR
1. JSX da página StockMovementsPage compilável; ternário explicitamente finalizado com `: null`.
2. Escapes desnecessários removidos dos testes de CSV de produtos e estoque inicial.
3. Testes do validador XML fiscal independentes de `/bin/sh`; execução auxiliar nativa fornece evidências de argumentos, STDIN e stderr.

## Gates de continuidade (P0)
- [x] Frontend: lint sem erros e build.
- [x] Backend: testes Go unitários.
- [x] Playwright: parsers CSV (9/9).
- [x] Backend: go vet, aprovado localmente.
- [ ] Infraestrutura: habilitar Docker Desktop Linux Engine ou PostgreSQL/Redis locais isolados.
- [ ] Migrations limpas 0001–0034 + upgrade/rollback/reapply com vínculos ativos/inativos.
- [ ] Testes de integração PostgreSQL/Redis e isolamento A/B.
- [ ] E2E navegador com API real e execução prod-like.
- [ ] Validação do hardware, piloto com usuário real, segurança e SEFAZ homologada antes de produção.

**Não fazer merge nem declarar prontidão de produção com apenas os resultados acima.**
