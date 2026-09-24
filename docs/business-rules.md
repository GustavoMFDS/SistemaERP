# Etapa 5 — Regras de negócio (MVP)

## Estoque
- A venda baixa estoque automaticamente.
- Bloqueia venda acima do saldo, a menos que `ALLOW_NEGATIVE_STOCK=true`.
- Baixo estoque: `qty_on_hand <= min_stock` gera alerta/relatório.
- Toda entrada/saída gera `inventory_movements` com `type`, usuário e observação.
- Ajuste manual exige `reason` e só aceita `adjustment`, `loss` ou `damage`.
- Movimentos `purchase` e `return` são reservados aos fluxos transacionais de compras e devoluções; não podem ser forjados pelo endpoint de ajuste manual.

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
- Cada pagamento aceita uma única conciliação inicial. Replay da mesma `Idempotency-Key` retorna o resultado original; uma nova tentativa de conciliação inicial retorna conflito.
- Correções posteriores usam o fluxo explícito de ajuste: o registro inicial permanece imutável, cada ajuste exige justificativa, guarda valores/taxas anteriores e novos, é idempotente e grava auditoria transacional.
- Reembolso de devolução pode ser parcial e multimétodo, mas a soma nunca ultrapassa `refund_due`.
- Replay de uma liquidação com a mesma `Idempotency-Key` devolve o mesmo ID, status e `remaining_amount` registrados na resposta original, mesmo que outras liquidações ocorram depois.
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


### Fechamento líquido de meios digitais

- Dinheiro físico declarado nunca pode ser negativo.
- Meios digitais podem ter saldo líquido negativo na sessão quando reembolsos vinculados à sessão excedem as vendas daquele método.
- Nessa situação, o fechamento deve registrar o valor líquido negativo declarado e comparar diretamente com o esperado negativo, em vez de forçar zero e criar uma divergência artificial.


## Compras e fornecedores

- Fornecedores e compras são sempre tenant-scoped.
- Leitura, escrita e recebimento usam permissões próprias: `procurement:read`, `procurement:write` e `procurement:receive`.
- Cashier não recebe permissões de procurement por padrão; admin e manager recebem.
- Nova compra exige fornecedor ativo e produtos ativos do mesmo tenant.
- Criar a compra não altera estoque. Estoque e custo são atualizados somente no recebimento.
- Recebimento parcial é permitido; a quantidade acumulada nunca pode exceder a quantidade pedida.
- Compra com qualquer quantidade já recebida não pode ser cancelada.
- Se houver vencimento financeiro, a compra cria contas a pagar tenant-safe; cancelamento da compra cancela o título ainda aberto.
- Criação, recebimento e cancelamento de compra gravam auditoria crítica na mesma transação das alterações de negócio.


## Visibilidade de custo e menor privilégio

- `cost_price`, `cost_unit` e `profit_estimated` só são expostos por HTTP quando o usuário possui `finance:read`.
- Cashier pode consultar produtos, estoque e vendas necessários ao PDV sem receber custo ou lucro.
- Um usuário com `product:write` sem `finance:read` não pode sobrescrever custo oculto: criação força custo zero e atualização preserva o custo atual.
- A UI oculta ações de escrita de produto/estoque quando as permissões correspondentes não estão presentes; a API continua sendo a barreira autoritativa.
- Leitura do módulo de devoluções exige `sale:return`, não apenas `sale:read`.


## Separação de custos, compras e ajustes manuais

- Usuários sem `finance:read` podem consultar produtos e vendas necessárias ao PDV, mas recebem `cost_price`, `cost_unit` e `profit_estimated` redigidos como zero.
- A redação é aplicada no backend; esconder campos na interface não é a barreira de segurança.
- Usuários com `product:write` mas sem `finance:read` não podem alterar custo indiretamente: criação força custo zero e atualização preserva o custo existente.
- Compras usam permissões próprias `procurement:read`, `procurement:write` e `procurement:receive`, concedidas por padrão apenas a admin/manager.
- Cashier não pode listar fornecedores/compras, devoluções nem endpoints financeiros por acesso direto.
- O endpoint manual `inventory/adjust` aceita apenas `adjustment`, `loss` e `damage`.
- Movimentos `purchase` e `return` são reservados aos fluxos transacionais de recebimento de compra e devolução, preservando rastreabilidade.
- Produto inativo não pode ser incluído em uma nova compra.
- Criação, recebimento e cancelamento de compra, assim como ajuste manual de estoque, gravam auditoria na mesma transação da alteração operacional.
- Cada pagamento digital pode ter no máximo uma conciliação inicial. Correções posteriores são registradas em `payment_reconciliation_adjustments`, sem apagar o histórico original.


### Preço efetivo no PDV

- `promo_price` positivo, quando configurado, é o preço efetivo usado pelo PDV e pelo backend.
- Promoção nunca pode superar `price_cash`; create/update rejeitam essa configuração.
- Carrinho suspenso é rascunho e, quando retomado online, recebe o preço atual do catálogo.
- Catálogo offline vencido não pode ser usado para iniciar novas vendas.


### Retenção de idempotência

- Chaves idempotentes de venda, compras/fornecedores, devoluções e financeiro são imutáveis enquanto retidas.
- As quatro famílias de chaves idempotentes usam a mesma janela operacional de retenção de 30 dias.
- A manutenção roda em lotes limitados e usa índice por `created_at` para evitar scan integral conforme o banco cresce.
- Depois da janela de retenção, o sistema não promete replay histórico de uma chave antiga; clientes offline também têm janela própria mais curta para evitar reenvio inseguro.

### Manutenção de fornecedores

- Fornecedor pode ser editado, ativado ou desativado sem apagar o histórico de compras.
- Fornecedor inativo permanece visível no histórico, mas não aparece para novas compras.
- O backend rejeita criação de compra com fornecedor inativo mesmo em chamada API direta.
