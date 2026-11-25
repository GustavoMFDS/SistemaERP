# Fase 2 — Esterium Retail (P1)

As propostas complementam PDV, estoque, compras, devoluções, descontos e conciliação existentes. Confirmar lacunas em AUD-01.

## Estoque e catálogo
**RET-01 — Inventário físico:** abrir sessão por tenant e escopo de SKUs, registrar snapshot/referência temporal, contagem cega ou conferência, recontagem, divergências, aprovação por permissão e ajuste transacional auditado. Definir política para vendas durante contagem (reconciliar movimentos, não sobrescrever). Aceite: divergência e concorrência reproduzíveis, nenhuma dupla baixa, reprocessamento idempotente.
**RET-02 — Etiquetas e catálogo:** modelos de etiquetas, EAN/SKU, preview e impressora; edição em massa com prévia, validação e auditoria; categorias/marcas, foto principal e ordenação, exportações paginadas/streaming com RBAC. Aceite: variantes mantêm identidade e saldo; custo oculto não vaza.
**RET-03 — Perdas, depósitos, lotes:** motivo e evidência de avaria/perda; depósitos dentro do mesmo CNPJ, transferências auditadas, lote/validade/FEFO se o segmento precisar. Nunca transferir entre CNPJs como simples movimento interno. Aceite: soma dos saldos, movimentações e inventário reconciliados; dados legados migram sem perda.
**RET-04 — Reposição determinística:** cobertura de dias, mínimo/máximo, lead time, compras em aberto e estoque de segurança; sugestão revisável, nunca pedido automático. Aceite: histórico e entradas rastreáveis, fallback para dados escassos.

## Compras
**RET-05 — Importação NF-e de fornecedor:** carregar XML com parser seguro, validar tipo/estrutura/chave e destinatário, deduplicar, mapear itens/GTIN/SKU e unidades, conferir custo/frete/descontos e quantidade, prévia e aprovação. XML recebido não é autorização da própria loja. Entrada somente após confirmação; sem duplicar conta ou estoque. Validar regras fiscais com contador.
**RET-06 — Gestão de compras:** histórico de custo e comparativo por unidade equivalente, lead time, pedidos sugeridos, aprovações, recebimentos parciais, devolução a fornecedor com efeitos de estoque/financeiro/fiscal aplicáveis. Aceite: devolução não excede recebido, auditoria e idempotência.

## PDV e trocas
**RET-07 — Troca integrada:** localizar venda, escolher itens/estado, calcular saldo pelo valor original e política, escolher reposição e nova venda, cobrar/devolver diferença, comprovante e vínculos. Usar estados explícitos se integração externa falhar, sem duplicar refund/venda.
**RET-08 — Ergonomia e periféricos:** scanner e atalhos existentes preservados, melhoria de busca, foco, operações em poucos cliques, acessibilidade; impressão/ESC-POS/gaveta/balança somente se necessário e após teste físico. Comprovante não fiscal claramente identificado.
**RET-09 — Pagamentos:** Pix dinâmico e TEF via adaptadores de provedor escolhido. Separar intenção, pendente, confirmado, recusado, desconhecido, estornado; verificar callbacks e consultar estado antes de repetir; valor e moeda conferidos; nunca aceitar screenshot como confirmação. Recebíveis, taxas e liquidação conciliados. Aceite: timeout e callback duplicado sem dupla cobrança, falha recuperável, segredo protegido.
**RET-10 — Gestão comercial:** kits/combos com baixa de componentes, regras de promoções e precedência, cupons, preço por margem desejada, aprovação de descontos, metas e comissões calculadas após devoluções conforme regra aprovada. Aceite: arredondamento e precedência determinísticos, permissão e auditoria.

Dependências: FND-01/02, contratos existentes e plano fiscal/financeiro validado para efeitos relevantes. Propostas de ordem: RET-01, RET-05, RET-04, RET-07, RET-09; ajustar à rotina da loja.
