# Fase 4 — Esterium Analytics (P2)

Expandir Home/Reports e ranking existentes para visão de gestão, sem substituir consultas transacionais. Dados sintéticos em demonstrações.

## Métricas e contratos (propostas a validar)
| Indicador | Definição inicial | Cuidado |
|---|---|---|
| Faturamento | Soma de vendas finalizadas menos cancelamentos conforme política | Separar bruto e líquido de devoluções, período explícito |
| Ticket médio | Receita de vendas elegíveis / quantidade de vendas elegíveis | Denominador zero = indisponível |
| Margem bruta | (Receita líquida gerencial - CMV) / receita líquida | Custo histórico e impostos definidos; não lucro líquido |
| Capital em estoque | Saldo vendável × custo de estoque conforme método aprovado | Custo ausente sinalizado |
| Giro | CMV do período / estoque médio a custo | Definir amostragem e período |
| Curva ABC | Participação acumulada por receita ou margem; limites configuráveis | Critério e empates visíveis |
| Ruptura | SKU ativo indisponível ou risco por cobertura | Separar ruptura de baixo estoque |
| Estoque parado | SKUs sem vendas por janela configurável, com saldo | Excluir recém-cadastrados conforme regra |
| Ponto de equilíbrio | Custos fixos / margem de contribuição percentual | Somente com despesas e variáveis suficientes |
| Vendedor | Vendas e margem atribuíveis, líquidas conforme regra | Sem ranking injusto sem contexto |
| Fornecedor | Custo unitário comparável, prazo e devoluções | Normalizar unidade/frete |
| Cesta de compras | Frequência, suporte, confiança/lift de itens juntos | Amostra mínima e privacidade |
| Sazonalidade | Comparação de períodos equivalentes | Sinalizar promoções/feriados e dados insuficientes |

## Entregas
ANA-01 dicionário de métricas e fixtures reconciliadas; ANA-02 painel proprietário (faturamento, margem, ticket, capital, série temporal, Central de Atenção); ANA-03 ABC/giro/estoque parado/ruptura; ANA-04 rentabilidade, sazonalidade, vendedores, cesta e fornecedores; ANA-05 ponto de equilíbrio, projeções e relatórios personalizáveis/exportáveis.

## Experiência
Filtros por período e dimensão permitida, comparação com período equivalente, detalhamento até registros autorizados, indicação de atualização, estados de erro e ausência de dados. Nunca fabricar valores. Exemplos conceituais: faturamento R$84.520, margem 32,4%, ticket R$127,48, estoque R$142.350, 12 SKUs em risco, 28 parados, R$8.420 a receber; são números fictícios, não métricas da loja.

## Implementação e aceite
Começar com queries indexadas/visões PostgreSQL, introduzir read models e jobs só quando carga justificar. Isolamento tenant, autorização de custo, filtros consistentes, reconciliação com fontes, teste de datas/estornos, exportação e carga. Medir p95 com volume representativo; meta posterior. Indicadores que não têm dados suficientes exibem indisponível.
