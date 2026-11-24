# Design System e experiência (transversal)

Aprimorar o trabalho histórico de refinamento (issue #43, [UX PDV](../ux-pdv-operacao-guidada.md), [UX financeiro/fiscal](../ux-financeiro-fiscal-20261009.md)). Não redesenhar tudo antes de mapear as telas e problemas reais.

## UX-01 — Inventário e fundações
Catalogar páginas e estados atuais, componentes compartilhados, tokens de cor, tipografia, espaçamento, foco, bordas, elevação e densidade. Definir paleta profissional, contraste e temas claro/escuro. Tailwind 4 depende da reconciliação #215/#216; não duplicar migração. Documentar tokens e exemplos de código.

## UX-02 — Componentes
Botões com hierarquia, inputs monetários BRL, tabelas com filtros/paginação/ordenação/ações em massa, formulário e validação contextual, diálogo confirmatório, toast, status, alertas, gráficos acessíveis, loading/skeleton, vazios e erro recuperável. Estados desabilitados explicam motivo; sem alertas verdes quando consulta falhou. Não depender só de cor. Keyboard, foco, labels e contraste WCAG AA como meta de projeto, validar na implementação.

## UX-03 — Jornadas
| Área | Direção |
|---|---|
| Início | Indicadores acionáveis, Central de Atenção, atalhos por perfil |
| PDV | Busca/scanner prioritários, carrinho legível, confirmação e atalhos, poucos cliques |
| Catálogo e estoque | Tabela densa porém legível, filtros persistentes, fotos/variantes, ações em massa |
| Financeiro/relatórios | Separar realizado, previsto e divergente, drill-down e exportação |
| Fiscal | Estados precisos, próxima ação, distinguir obrigação/preview/autorização |
| Equipe e setup | Orientação passo a passo e permissões compreensíveis |
| Mobile | Resumo proprietário e fluxo de leitura/contagem no estoque |

Referências visuais: dashboard administrativo claro, PDV com carrinho lateral, catálogo com tabelas eficientes e painel mobile compacto são direções conceituais, não design aprovado ou imagens do ERP.

## UX-04 — Validação
Prototipar por jornada, aprovar com usuários da loja antes de implementação abrangente. Testar teclado, scanner, zoom, leitores de tela quando possível, breakpoints reais, erro/offline, confirmação de mutação e recuperação. Capturar evidências antes/depois sem PII. Medir tempo, erros e passos; metas só após baseline. Design System deve reduzir duplicação e não adicionar dependência pesada sem justificativa.
