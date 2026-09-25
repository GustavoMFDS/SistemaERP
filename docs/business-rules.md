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

## Round 6 — integridade transacional de caixa
- Venda, cancelamento, sangria/suprimento e fechamento serializam na linha da sessão de caixa.
- Cancelamento só é permitido enquanto o caixa original permanece aberto.
- Venda com invoice/XML fiscal existente não pode ser cancelada pelo fluxo comum.
- Sangria (`withdrawal`) e suprimento (`supply`) entram no caixa e no razão financeiro na mesma transação.
- Sangria acima do dinheiro físico disponível é rejeitada.
- Fechamento reconcilia esperado, declarado e diferença por `cash`, `pix`, `debit`, `credit`, `transfer` e `voucher`.
- Dinheiro físico esperado = abertura + vendas em dinheiro + suprimentos - sangrias.
- XML fiscal é gerado somente de venda `finalized` e bloqueia a venda enquanto a invoice é criada.

## Round 7 — isolamento entre CNPJs
- Cada loja/CNPJ continua sendo um tenant independente; referências de categoria, produto, cliente, caixa, venda, pagamento, razão e fiscal devem permanecer no mesmo tenant.
- O mesmo código de barras/EAN pode existir em tenants diferentes; unicidade de barcode é tenant-scoped.
- Atualizar um produto inexistente ou de outro tenant retorna not found e não gera sucesso/audit falso.
- Uma venda não aceita `customer_id` pertencente a outro tenant.
- Bloquear um usuário por LGPD bloqueia somente a membership daquele tenant; não desativa a identidade global usada por outra loja.
- Anonimização global de usuário compartilhado por múltiplos tenants é recusada com conflito até que as memberships sejam reconciliadas.
- Consentimentos LGPD exigem um titular identificado e pertencente ao tenant.

### Usuários com acesso a mais de um CNPJ
- Uma identidade de usuário pode pertencer a mais de uma loja independente, mas uma sessão opera em exatamente um tenant por vez.
- A troca de loja é explícita e emite novas credenciais scoped ao CNPJ escolhido.
- Caixa local, fila offline e cache de produtos não são movidos nem apagados ao trocar de loja; permanecem isolados no namespace do tenant original.

