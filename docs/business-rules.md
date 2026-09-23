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
