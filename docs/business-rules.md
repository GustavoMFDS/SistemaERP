# Etapa 5 — Regras de negócio (MVP)

## Estoque
- A venda baixa estoque automaticamente.
- Bloqueia venda acima do saldo, a menos que `ALLOW_NEGATIVE_STOCK=true`.
- Baixo estoque: `qty_on_hand <= min_stock` gera alerta/relatório.
- Toda entrada/saída gera `inventory_movements` com `type`, usuário e observação.
- Ajuste manual exige `reason`.

## Financeiro
- Toda venda finalizada gera `ledger_entries` com bruto, desconto, líquido e lucro estimado.
- Lucro estimado = soma por item `(unit_price - cost_price) * qty - descontos`.
- Sangria/suprimento entram no caixa e no razão financeiro.

## PDV
- Venda possui status: `open`, `finalized`, `cancelled`.
- Cancelamento exige permissão e justificativa.
- Suporta múltiplos pagamentos por venda (pix+dinheiro etc.).
- Operador deve ter uma sessão de caixa aberta para vender.

## Fiscal (NF-e MVP)
- XML é gerado a partir de uma venda finalizada.
- XML fica armazenado e disponível para download.
- Estrutura preparada para assinatura/transmissão futura (camada separada de montagem do XML).


## Devoluções e trocas

- Somente vendas com status `finalized` aceitam devolução.
- A soma das quantidades devolvidas de cada item nunca pode exceder a quantidade originalmente vendida.
- O valor de reembolso é derivado do valor líquido da venda original e não é informado pelo cliente.
- Devoluções idempotentes reutilizam a mesma `Idempotency-Key`; a mesma chave com payload diferente resulta em conflito.
- `restock=true` gera movimento de estoque `return`; `restock=false` não aumenta estoque vendável.
- Troca é registrada com `kind=exchange`; a mercadoria substituta deve ser registrada em uma nova venda normal.
- Registrar devolução não liquida dinheiro/Pix/cartão. O valor fica como reembolso devido para o fluxo financeiro.
- Após qualquer devolução, o cancelamento integral da venda original é bloqueado para impedir dupla recomposição de estoque.


## Conciliação de pagamentos e reembolsos

- A venda continua sendo a fonte do valor cobrado; conciliação não reescreve total da venda.
- Dinheiro é conciliado pelo fechamento da sessão de caixa e não pelo endpoint de adquirentes.
- Pagamentos não monetários podem registrar provedor, referência da transação, autorização e parcelas.
- A conciliação registra recebido bruto, taxa, líquido e diferença contra o pagamento esperado.
- Valor bruto diferente do esperado produz status `divergent`; taxa de adquirente é armazenada separadamente.
- Reconciliar novamente com outra chave é permitido como novo ajuste auditável; replay da mesma chave é idempotente.
- Reembolso de devolução pode ser parcial e multimétodo, mas a soma nunca ultrapassa `refund_due`.
- Reembolso em dinheiro exige sessão aberta e disponibilidade física; gera movimento `withdrawal`.
- Reembolso digital pode ser associado a uma sessão aberta para compor a conciliação líquida por método.
- Cada liquidação gera lançamento negativo `return_refund` no ledger e evento crítico de auditoria.


## Operação rápida do PDV

- Desconto positivo em qualquer item ou no total exige `sale:discount`; a regra é validada no backend antes da transação de venda.
- Admin e manager recebem `sale:discount` por padrão; cashier não recebe.
- Carrinhos suspensos são rascunhos locais, não alteram estoque, caixa, ledger ou auditoria.
- Carrinhos suspensos usam armazenamento escopado por tenant e usuário e não atravessam identidades no mesmo navegador.
- O write-ahead offline continua ocorrendo somente no momento da finalização; suspender um carrinho não cria uma intenção de venda.
- Comprovante impresso pela UI antes da integração fiscal real é sempre identificado como não fiscal.
