# CI por escopo — proposta conservadora

Branch experimental empilhada em 1391c73d, sem alterar a PR #231 ou a branch principal. Não usar os resultados locais como aprovação de release.

| Escopo de PR | Jobs gerais executados | Windows Setup |
| --- | --- | --- |
| docs: apenas Markdown sob docs/esterium-2 | scope + ci-scope-gate | filtros existentes; não dispara por docs isolados |
| windows: apenas workflow Windows, spec/config Playwright instalado e/ou docs acima | scope, backend, frontend, security, local-windows-service-tests, local-windows-bundle, ci-scope-gate | homologação completa continua obrigatória |
| full: qualquer outro caminho, diff vazio/indisponível, alterações de CI/classificador, código, SQL, dependências ou mistura com caminhos desconhecidos | todos os jobs aplicáveis, mais scope e gate | filtros existentes preservados |
| push principal / dispatch | full; EDB adicional em dispatch, não em push (comportamento push preexistente) | homologação completa por dispatch separado; push conserva prévia |

O gate exige success para todo job selecionado e skipped para os demais. Skipped não é prova de execução. Diff Git completo entre merge-base e head, incluindo os dois caminhos em renames. Nenhum status obrigatório do repositório é alterado neste protótipo. Antes de adoção: confirmar regras efetivas, revisar lista de caminhos e dependências, executar PRs de teste das três rotas e validar cancelamento/falha do gate; não substituir homologação assinada por checks direcionados. Manter workflow Windows com verificação pós-publicação e exigir os dois workflows completos no mesmo head para pacote de distribuição. PRs empilhadas com alterações de código acumuladas corretamente classificam como full.

Estimativa por número de jobs, não duração medida: docs 2 em vez de 20; Windows-only 7 em vez de 20 no CI geral (Windows Setup completo permanece); full 22 em vez de 20 em PR, custo de dois jobs leves de controle. O tempo de instalação EDB/ZIP de homologação não é eliminado. Sem alteração de regras, merge, aprovação de release ou aplicação na VM.

Validações locais: 16 casos de classificação, 10 cenários sintéticos do gate extraído do YAML, diff real 4d32aab4...1391c73d (windows), diff inválido (full), parser YAML e cobertura de 19 definições de jobs (20 execuções com matriz E2E), git diff --check. Execução real do novo workflow ainda não realizada.
