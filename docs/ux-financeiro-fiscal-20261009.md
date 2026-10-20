# Melhoria de UX e finanças — 2026-10-09

**Branch:** `ops/pilot-readiness-20260923`, PR #15. Cada CNPJ mantém dados e papéis **independentes**.

## Correções e melhorias

1. **Fiscal:** corrigido erro fatal quando a API fornece `blocking_reasons: null` e quando `XMLListResponse.items` é nulo. Agora a lista de pendências vira um array vazio e a página continua visível; teste Chromium capturou o erro antes da correção e passou depois.
2. **Configurar loja:** título orientado ao dono, progresso em cinco etapas, cards e próxima ação, aviso técnico da NFC-e recolhível. A configuração fiscal não ativa emissão real.
3. **Início:** nova área de boas-vindas, atalho principal para o caixa, indicadores semanais reais, poucos acessos principais, estoque de atenção em cards; ações restantes em seção expandível, sem remover rotas.
4. **Estoque:** reposição visível em destaque, estoque inicial e ajuste manual em seções recolhíveis; tentativa de importação pendente abre seção automaticamente.
5. **Produtos:** busca automática com atraso de 300 ms, tabelas com dados fiscais opcionais e formulário/importação recolhíveis. Os itens e preços continuam sob controle da API.
6. **PDV:** busca remota por nome/SKU/código a partir de duas letras, mesclada ao catálogo local sem perder metadados do carrinho; seleção rápida visual dos primeiros oito resultados. Busca local/cache continua disponível offline. A atualização manual de produtos foi movida para **Outras ações**, e o encerramento ganhou um bloco explicando a conferência física e os pagamentos informados.
7. **Financeiro:** nova visualização integrada de contas e evolução, com recursos básicos de operação descritos abaixo; conciliações avançadas e ranking de produtos seguem acessíveis.

## Backend: contas a pagar e a receber (fase funcional inicial)

**Migration 0035_finance_accounts** complementa as tabelas existentes sem apagar compras/recebimentos anteriores:
- Criação manual de contas com descrição, data e valor monetário em centavos (tipo Money).
- Listagem por CNPJ de até 500 contas, status e vencimentos.
- Baixa integral transacional com status, data, operador, meio informado e referência idempotente (UUID).
- Lançamento correspondente no `ledger_entries`: `expense` negativo para contas a pagar; `revenue` positivo para contas a receber, **somente ao registrar a baixa**.
- Eventos de auditoria persistidos **na mesma transação** da criação ou da baixa.
- Permissões: `finance:read` para consulta; `finance:reconcile` para criação/baixa, além de autenticação e filtro por tenant.
- Em caso de repetição idempotente de uma baixa confirmada para a mesma conta/método, retornar replay sem novo lançamento.
- Série diária de 7, 14, 30 ou 90 dias com base em lançamentos reais do ledger.
- A reversão da migration 35 falha de modo seguro se houver proveniência de contas manuais ou baixas que seriam perdidas.

**Rotas:** `GET/POST /api/v1/finance/accounts`, `POST /api/v1/finance/accounts/{kind}/{id}/settle` e `GET /api/v1/finance/trends?days=14`.

**Limites IMPORTANTES:** isto **não é** contabilidade completa, saldo bancário, liquidação automática do Pix/TEF, baixa parcial, juros/multas, parcelamento de crediário ou integração fiscal. O meio de pagamento é informado pelo operador, não uma confirmação de banco. O gráfico mostra apenas lançamentos do livro; totais de contas podem ser truncados acima de 500.

## Evidências nesta fase

- `go test ./...` e `go vet ./...`: passaram após backend de contas.
- `npm run build`: passou após as mudanças financeiras e de navegação. `npm run lint`: 0 erros, 12 avisos de Hooks/estilo ainda abertos.
- A regressão final em banco de demonstração reutilizado passou nos dois testes de layout PDV, mas `pdv-offline.spec.ts` e `pdv-operations.spec.ts` atingiram timeout ao procurar `Abrir`: já existia sessão de caixa aberta nesse banco. **Não contar esses testes como aprovados nesta rodada**; reexecutar em banco descartável por cenário.
- Migration 0035 aplicada ao PostgreSQL local `sistemaemgo_ux`.
- Chromium: cenários financeiros de conta a pagar (criar, dar baixa, gráfico) **e negação de acesso ao operador de caixa**, aprovados **2/2**.
- Chromium: renderização Fiscal sem erro JS após resposta real com campos nulos, aprovado **1/1**.
- Chromium: navegação Início/Produtos/Estoque/Configurar loja e busca remota no PDV, aprovados **2/2** após corrigir um seletor de teste que procurava `columnheader` onde o componente expunha `cell`.
- A suíte Playwright completa e testes concorrentes entre CNPJs ainda não foram concluídos; não declarar prontidão de produção.

## Prioridade seguinte

- [ ] Corrigir e reexecutar toda a suíte Playwright em base independente por caso.
- [ ] Testar isolamento entre duas empresas para as novas contas, idempotência concorrente e falhas transacionais.
- [ ] Melhorar a listagem de contas acima de 500 registros com paginação real e totais agregados no banco.
- [ ] Implementar baixa parcial/estorno auditado, parcelas, juros/multas e conciliação de extratos.
- [ ] Integrar contas a receber com cadastro de clientes e venda a prazo, com contrato e critérios de risco.
- [ ] Tratar fornecedores e baixa de compras com conferência de notas/duplicatas.
- [ ] Homologar fluxos fiscais SEFAZ fora do ambiente de demonstração, sem transmissão automática.
- [ ] Validar acessibilidade/UX com funcionários em dispositivos e caixa reais.

Sem aprovação explícita de integração, não fazer merge nem ativar produção.
